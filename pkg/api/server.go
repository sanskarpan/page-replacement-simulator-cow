package api

import (
	"sync"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/monitor"
	"github.com/page-replacement-cow/internal/process"
	"github.com/page-replacement-cow/internal/simulator"
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
	broadcast chan interface{}

	// Monitor stop channel
	monitorStop chan struct{}
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

	// Start periodic monitoring
	server.monitorStop = mon.StartPeriodicCapture(1000 * 1000 * 1000) // 1 second

	return server
}

// Shutdown cleanly shuts down the server
func (s *Server) Shutdown() {
	close(s.monitorStop)
	close(s.broadcast)
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
