package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/page-replacement-cow/internal/algorithms"
)

// HandleGetStatus returns system status
func (s *Server) HandleGetStatus(w http.ResponseWriter, r *http.Request) {
	status := s.monitor.GetSystemStatus()
	writeJSON(w, http.StatusOK, status)
}

// HandleGetMetrics returns current metrics
func (s *Server) HandleGetMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := s.memoryManager.GetMetrics()
	writeJSON(w, http.StatusOK, metrics)
}

// HandleGetProcesses returns all processes
func (s *Server) HandleGetProcesses(w http.ResponseWriter, r *http.Request) {
	processes := s.monitor.GetProcessDetails()
	writeJSON(w, http.StatusOK, processes)
}

// HandleGetProcess returns a specific process
func (s *Server) HandleGetProcess(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pid := vars["id"]

	process, err := s.processManager.GetProcess(pid)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, process)
}

// HandleCreateProcess creates a new process
func (s *Server) HandleCreateProcess(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string `json:"name"`
		Priority     int32  `json:"priority"`
		VirtualPages uint64 `json:"virtual_pages"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Priority < 0 || req.Priority > 10 {
		writeError(w, http.StatusBadRequest, "priority must be between 0 and 10")
		return
	}
	if req.VirtualPages == 0 || req.VirtualPages > 100000 {
		writeError(w, http.StatusBadRequest, "virtual_pages must be between 1 and 100000")
		return
	}

	process, err := s.processManager.CreateProcess(req.Name, req.Priority, req.VirtualPages)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Broadcast update
	s.Broadcast(map[string]interface{}{
		"type":    "process_created",
		"process": process,
	})

	writeJSON(w, http.StatusCreated, process)
}

// HandleTerminateProcess terminates a process
func (s *Server) HandleTerminateProcess(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pid := vars["id"]

	if err := s.processManager.TerminateProcess(pid); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Broadcast update
	s.Broadcast(map[string]interface{}{
		"type": "process_terminated",
		"pid":  pid,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "terminated"})
}

// HandleForkProcess forks a process
func (s *Server) HandleForkProcess(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pid := vars["id"]

	child, err := s.processManager.ForkProcess(pid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Broadcast update
	s.Broadcast(map[string]interface{}{
		"type":   "process_forked",
		"parent": pid,
		"child":  child,
	})

	writeJSON(w, http.StatusCreated, child)
}

// HandleAccessMemory performs a memory access
func (s *Server) HandleAccessMemory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProcessID   string `json:"process_id"`
		VirtualPage uint64 `json:"virtual_page"`
		Write       bool   `json:"write"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.processManager.AccessMemory(req.ProcessID, req.VirtualPage, req.Write); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleGetFrames returns frame information
func (s *Server) HandleGetFrames(w http.ResponseWriter, r *http.Request) {
	frames := s.monitor.GetFrameDetails()
	writeJSON(w, http.StatusOK, frames)
}

// HandleGetPageTable returns a process's page table
func (s *Server) HandleGetPageTable(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pid := vars["id"]

	pageTable, err := s.memoryManager.GetPageTable(pid)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, pageTable.GetAllPages())
}

// HandleGetHistory returns historical metrics
func (s *Server) HandleGetHistory(w http.ResponseWriter, r *http.Request) {
	lastStr := r.URL.Query().Get("last")
	last := 100

	if lastStr != "" {
		if val, err := strconv.Atoi(lastStr); err == nil {
			last = val
		}
	}

	history := s.monitor.GetHistory(last)
	writeJSON(w, http.StatusOK, history)
}

// HandleGetEvents returns recent events
func (s *Server) HandleGetEvents(w http.ResponseWriter, r *http.Request) {
	lastStr := r.URL.Query().Get("last")
	last := 50

	if lastStr != "" {
		if val, err := strconv.Atoi(lastStr); err == nil {
			last = val
		}
	}

	events := s.monitor.GetEvents(last)
	writeJSON(w, http.StatusOK, events)
}

// HandleSetAlgorithm sets the page replacement algorithm
func (s *Server) HandleSetAlgorithm(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Algorithm string `json:"algorithm"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var algType algorithms.AlgorithmType

	switch req.Algorithm {
	case "LRU":
		algType = algorithms.AlgorithmLRU
	case "CLOCK":
		algType = algorithms.AlgorithmCLOCK
	case "LFU":
		algType = algorithms.AlgorithmLFU
	case "FIFO":
		algType = algorithms.AlgorithmFIFO
	case "Optimal":
		algType = algorithms.AlgorithmOptimal
	case "Random":
		algType = algorithms.AlgorithmRandom
	case "ARC":
		algType = algorithms.AlgorithmARC
	case "CAR":
		algType = algorithms.AlgorithmCAR
	case "WSClock":
		algType = algorithms.AlgorithmWSClock
	case "PFF":
		algType = algorithms.AlgorithmPFF
	case "OPT+":
		algType = algorithms.AlgorithmOPTPlus
	default:
		writeError(w, http.StatusBadRequest, "invalid algorithm")
		return
	}

	s.memoryManager.SetAlgorithm(algType)

	// Broadcast update
	s.Broadcast(map[string]interface{}{
		"type":      "algorithm_changed",
		"algorithm": req.Algorithm,
	})

	writeJSON(w, http.StatusOK, map[string]string{"algorithm": req.Algorithm})
}

// HandleRunSimulation runs a simulation scenario
func (s *Server) HandleRunSimulation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scenario string `json:"scenario"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Run simulation in background
	go func() {
		result, err := s.simulator.RunScenario(req.Scenario)
		if err != nil {
			s.Broadcast(map[string]interface{}{
				"type":    "simulation_error",
				"error":   err.Error(),
				"scenario": req.Scenario,
			})
			return
		}

		s.Broadcast(map[string]interface{}{
			"type":   "simulation_complete",
			"result": result,
		})
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

// HandleGetScenarios returns available scenarios
func (s *Server) HandleGetScenarios(w http.ResponseWriter, r *http.Request) {
	scenarios := s.simulator.GetAvailableScenarios()

	scenarioList := make([]map[string]string, len(scenarios))
	for i, scenario := range scenarios {
		scenarioList[i] = map[string]string{
			"name":        scenario,
			"description": s.simulator.GetScenarioDescription(scenario),
		}
	}

	writeJSON(w, http.StatusOK, scenarioList)
}

// HandleReset resets the system
func (s *Server) HandleReset(w http.ResponseWriter, r *http.Request) {
	s.processManager.Reset()
	s.memoryManager.Reset()
	s.monitor.ClearHistory()

	// Broadcast update
	s.Broadcast(map[string]interface{}{
		"type": "system_reset",
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

// HandleGetTLBStats returns TLB statistics
func (s *Server) HandleGetTLBStats(w http.ResponseWriter, r *http.Request) {
	tlb := s.memoryManager.GetTLB()
	stats := tlb.GetStats()
	writeJSON(w, http.StatusOK, stats)
}

// HandleGetCoWStats returns Copy-on-Write statistics
func (s *Server) HandleGetCoWStats(w http.ResponseWriter, r *http.Request) {
	cow := s.memoryManager.GetCoWManager()
	stats := cow.GetStats()
	writeJSON(w, http.StatusOK, stats)
}

// Helper functions

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// SetupRoutes sets up all API routes
func SetupRoutes(s *Server) *mux.Router {
	r := mux.NewRouter()

	// API routes
	api := r.PathPrefix("/api").Subrouter()

	// System
	api.HandleFunc("/status", s.HandleGetStatus).Methods("GET")
	api.HandleFunc("/metrics", s.HandleGetMetrics).Methods("GET")
	api.HandleFunc("/history", s.HandleGetHistory).Methods("GET")
	api.HandleFunc("/events", s.HandleGetEvents).Methods("GET")
	api.HandleFunc("/reset", s.HandleReset).Methods("POST")

	// Processes
	api.HandleFunc("/processes", s.HandleGetProcesses).Methods("GET")
	api.HandleFunc("/processes", s.HandleCreateProcess).Methods("POST")
	api.HandleFunc("/processes/{id}", s.HandleGetProcess).Methods("GET")
	api.HandleFunc("/processes/{id}", s.HandleTerminateProcess).Methods("DELETE")
	api.HandleFunc("/processes/{id}/fork", s.HandleForkProcess).Methods("POST")
	api.HandleFunc("/processes/{id}/pages", s.HandleGetPageTable).Methods("GET")

	// Memory
	api.HandleFunc("/memory/access", s.HandleAccessMemory).Methods("POST")
	api.HandleFunc("/memory/frames", s.HandleGetFrames).Methods("GET")
	api.HandleFunc("/memory/algorithm", s.HandleSetAlgorithm).Methods("POST")

	// TLB and CoW
	api.HandleFunc("/tlb/stats", s.HandleGetTLBStats).Methods("GET")
	api.HandleFunc("/cow/stats", s.HandleGetCoWStats).Methods("GET")

	// Simulation
	api.HandleFunc("/simulation/scenarios", s.HandleGetScenarios).Methods("GET")
	api.HandleFunc("/simulation/run", s.HandleRunSimulation).Methods("POST")

	// WebSocket
	api.HandleFunc("/ws", s.HandleWebSocket)

	return r
}
