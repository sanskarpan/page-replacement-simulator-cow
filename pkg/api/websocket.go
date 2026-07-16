package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// newUpgrader returns a websocket.Upgrader whose CheckOrigin is tied to the
// Server's allowed-origins list, preventing cross-site WebSocket hijacking.
func (s *Server) newUpgrader() websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			if s.allowAllOrigins {
				return true
			}
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true // same-origin request (no Origin header)
			}
			host := extractWSHost(origin)
			return s.allowedOrigins[host]
		},
	}
}

// extractWSHost strips the scheme and port from an origin header value.
func extractWSHost(origin string) string {
	origin = strings.TrimPrefix(origin, "http://")
	origin = strings.TrimPrefix(origin, "https://")
	if idx := strings.IndexByte(origin, ':'); idx >= 0 {
		origin = origin[:idx]
	}
	return origin
}

// Client represents a WebSocket client
type Client struct {
	server         *Server
	conn           *websocket.Conn
	send           chan interface{}
	done           chan struct{}
	metricsStarted int32 // atomic flag — ensures only one sendPeriodicMetrics goroutine per client
}

// HandleWebSocket handles WebSocket connections
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	upgrader := s.newUpgrader()
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err)
		return
	}

	client := &Client{
		server: s,
		conn:   conn,
		send:   make(chan interface{}, 256),
		done:   make(chan struct{}),
	}

	s.RegisterClient(client)

	// Start client goroutines
	go client.writePump()
	go client.readPump()

	// Send initial state — guard against the client disconnecting within the delay.
	go func() {
		timer := time.NewTimer(100 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-client.done:
			return
		}
		select {
		case client.send <- map[string]interface{}{
			"type":   "initial_state",
			"status": s.monitor.GetSystemStatus(),
		}:
		case <-client.done:
		}
	}()
}

// readPump reads messages from the WebSocket connection
func (c *Client) readPump() {
	defer func() {
		c.server.UnregisterClient(c)
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Warn("websocket unexpected close", "error", err)
			}
			break
		}

		// Handle incoming messages
		c.handleMessage(message)
	}
}

// writePump writes messages to the WebSocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		close(c.done)
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			// Write the message
			data, err := json.Marshal(message)
			if err != nil {
				slog.Error("websocket message marshal", "error", err)
				return
			}
			w.Write(data)

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage handles incoming WebSocket messages
func (c *Client) handleMessage(message []byte) {
	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		slog.Warn("websocket message unmarshal", "error", err)
		return
	}

	msgType, ok := msg["type"].(string)
	if !ok {
		return
	}

	switch msgType {
	case "subscribe_metrics":
		// One metrics goroutine per client — idempotent via atomic CAS.
		if atomic.CompareAndSwapInt32(&c.metricsStarted, 0, 1) {
			go c.sendPeriodicMetrics()
		}

	case "request_state":
		// Send current state; guard against sending on a closed channel when the
		// client disconnects concurrently (done fires before send is closed).
		msg := map[string]interface{}{
			"type":      "state_update",
			"status":    c.server.monitor.GetSystemStatus(),
			"processes": c.server.monitor.GetProcessDetails(),
			"frames":    c.server.monitor.GetFrameDetails(),
		}
		select {
		case c.send <- msg:
		case <-c.done:
		}
	}
}

// sendPeriodicMetrics sends metrics updates periodically.
// The inner select on done prevents a panic when close(c.send) races with the
// ticker firing: writePump closes done after closing send, so checking done
// before sending guards the window where send is closed but done is not yet.
func (c *Client) sendPeriodicMetrics() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			metrics := c.server.memoryManager.GetMetrics()
			msg := map[string]interface{}{
				"type":    "metrics_update",
				"metrics": metrics,
			}
			select {
			case c.send <- msg:
			case <-c.done:
				return
			}
		}
	}
}
