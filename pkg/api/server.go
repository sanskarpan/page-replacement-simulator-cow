// Package api implements the HTTP/WebSocket server for the Page Replacement
// Simulator. It exposes a REST API at /api/... for process management, memory
// access simulation, algorithm control, and statistics; a WebSocket endpoint
// at /api/ws for real-time metric streaming; and liveness/readiness probes at
// /healthz and /readyz.
package api

import (
	"strings"
	"sync"
	"time"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/monitor"
	"github.com/page-replacement-cow/internal/process"
	"github.com/page-replacement-cow/internal/simulator"
)

const (
	// MaxRequestBodyBytes caps all JSON request bodies to prevent OOM from crafted payloads.
	// Exported so the main package middleware can reference it without duplication.
	MaxRequestBodyBytes = 1 << 20 // 1 MB

	// maxHistoryItems caps query-param values for history/events pagination.
	maxHistoryItems = 1000
)

// Server is the API server
type Server struct {
	processManager *process.ProcessManager
	memoryManager  *memory.MemoryManager
	monitor        *monitor.Monitor
	simulator      *simulator.Simulator

	// WebSocket clients
	clients   map[*Client]bool
	clientsMu sync.RWMutex

	// Broadcast channel
	broadcast       chan interface{}
	broadcastClosed bool
	broadcastMu     sync.RWMutex

	// Monitor stop channel
	monitorStop chan struct{}

	// allowedOrigins is the set of hostnames permitted for CORS and WebSocket origin checks.
	// A nil map with allowAllOrigins=true accepts every origin.
	allowedOrigins  map[string]bool
	allowAllOrigins bool

	// simulationMu guards against concurrent simulation runs which would race on shared state.
	simulationMu sync.Mutex
}

// NewServer creates a new API server
func NewServer(numFrames int32, tlbSize int, algType algorithms.AlgorithmType) *Server {
	mm := memory.NewMemoryManager(numFrames, tlbSize, algType)
	pm := process.NewProcessManager(mm)
	mon := monitor.NewMonitor(pm, mm)
	sim := simulator.NewSimulator(pm)

	server := &Server{
		processManager: pm,
		memoryManager:  mm,
		monitor:        mon,
		simulator:      sim,
		clients:        make(map[*Client]bool),
		broadcast:      make(chan interface{}, 100),
	}

	// Start broadcast handler
	go server.handleBroadcast()

	// Start periodic monitoring (1-second interval)
	server.monitorStop = mon.StartPeriodicCapture(time.Second)

	return server
}

// Shutdown cleanly shuts down the server
func (s *Server) Shutdown() {
	close(s.monitorStop)
	s.broadcastMu.Lock()
	s.broadcastClosed = true
	close(s.broadcast)
	s.broadcastMu.Unlock()
	s.memoryManager.Close()
}

// handleBroadcast handles broadcasting messages to all WebSocket clients
func (s *Server) handleBroadcast() {
	for message := range s.broadcast {
		var deadClients []*Client

		s.clientsMu.RLock()
		for client := range s.clients {
			select {
			case client.send <- message:
			default:
				// Mark dead but do NOT close here — closing under RLock races
				// with UnregisterClient's close under WLock (double-close panic).
				deadClients = append(deadClients, client)
			}
		}
		s.clientsMu.RUnlock()

		if len(deadClients) > 0 {
			s.clientsMu.Lock()
			for _, client := range deadClients {
				// Guard: UnregisterClient may have already closed and removed this client.
				if _, ok := s.clients[client]; ok {
					close(client.send)
					delete(s.clients, client)
				}
			}
			s.clientsMu.Unlock()
		}
	}
}

// RegisterClient registers a WebSocket client
func (s *Server) RegisterClient(client *Client) {
	s.clientsMu.Lock()
	s.clients[client] = true
	s.clientsMu.Unlock()
}

// UnregisterClient unregisters a WebSocket client
func (s *Server) UnregisterClient(client *Client) {
	s.clientsMu.Lock()
	if _, ok := s.clients[client]; ok {
		delete(s.clients, client)
		close(client.send)
	}
	s.clientsMu.Unlock()
}

// Broadcast broadcasts a message to all clients
func (s *Server) Broadcast(message interface{}) {
	s.broadcastMu.RLock()
	closed := s.broadcastClosed
	s.broadcastMu.RUnlock()
	if closed {
		return
	}
	select {
	case s.broadcast <- message:
	default:
		// Broadcast channel full, skip
	}
}

// GetProcessManager returns the process manager
func (s *Server) GetProcessManager() *process.ProcessManager {
	return s.processManager
}

// GetMemoryManager returns the memory manager
func (s *Server) GetMemoryManager() *memory.MemoryManager {
	return s.memoryManager
}

// GetMonitor returns the monitor
func (s *Server) GetMonitor() *monitor.Monitor {
	return s.monitor
}

// GetSimulator returns the simulator
func (s *Server) GetSimulator() *simulator.Simulator {
	return s.simulator
}

// SetAllowedOrigins configures which Origin hostnames may open WebSocket connections.
// Replaces the package-level upgrader.CheckOrigin that previously allowed all origins.
func (s *Server) SetAllowedOrigins(domains []string) {
	s.allowedOrigins = make(map[string]bool)
	for _, d := range domains {
		d = strings.TrimSpace(d)
		if d == "*" {
			s.allowAllOrigins = true
			return
		}
		s.allowedOrigins[d] = true
	}
}
