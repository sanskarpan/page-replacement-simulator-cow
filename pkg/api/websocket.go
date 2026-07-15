package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in development
	},
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
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
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
				log.Printf("WebSocket error: %v", err)
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
				log.Printf("JSON marshal error: %v", err)
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
		log.Printf("JSON unmarshal error: %v", err)
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
		// Send current state
		c.send <- map[string]interface{}{
			"type":      "state_update",
			"status":    c.server.monitor.GetSystemStatus(),
			"processes": c.server.monitor.GetProcessDetails(),
			"frames":    c.server.monitor.GetFrameDetails(),
		}
	}
}

// sendPeriodicMetrics sends metrics updates periodically
func (c *Client) sendPeriodicMetrics() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			metrics := c.server.memoryManager.GetMetrics()
			c.send <- map[string]interface{}{
				"type":    "metrics_update",
				"metrics": metrics,
			}
		case <-c.done:
			return
		}
	}
}
