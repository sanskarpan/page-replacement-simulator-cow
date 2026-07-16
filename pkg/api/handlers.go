package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/pkg/models"
)

// processJSON is a serialization-safe view of models.Process.
// Atomic fields in models.Process do not marshal as numbers with encoding/json,
// so this DTO loads them via their Load() methods before encoding.
type processJSON struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Priority       int32    `json:"priority"`
	State          int32    `json:"state"`
	VirtualPages   uint64   `json:"virtual_pages"`
	PageTableSize  uint64   `json:"page_table_size"`
	WorkingSetSize int32    `json:"working_set_size"`
	PageFaults     int64    `json:"page_faults"`
	PageHits       int64    `json:"page_hits"`
	MemoryAccesses int64    `json:"memory_accesses"`
	CoWCopies      int64    `json:"cow_copies"`
	CPUTimeNs      int64    `json:"cpu_time_ns"`
	ParentID       string   `json:"parent_id"`
	Children       []string `json:"children"`
	CreatedAt      int64    `json:"created_at_ns"`
}

func marshalProcess(p *models.Process) processJSON {
	return processJSON{
		ID:             p.ID,
		Name:           p.Name,
		Priority:       p.Priority,
		State:          p.State.Load(),
		VirtualPages:   p.VirtualPages,
		PageTableSize:  p.PageTableSize,
		WorkingSetSize: p.WorkingSetSize,
		PageFaults:     p.PageFaults.Load(),
		PageHits:       p.PageHits.Load(),
		MemoryAccesses: p.MemoryAccesses.Load(),
		CoWCopies:      p.CoWCopies.Load(),
		CPUTimeNs:      p.CPUTime.Load(),
		ParentID:       p.ParentID,
		Children:       p.GetChildren(),
		CreatedAt:      p.CreatedAt.UnixNano(),
	}
}

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
		writeError(w, http.StatusNotFound, "process not found")
		return
	}

	writeJSON(w, http.StatusOK, marshalProcess(process))
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
		writeError(w, http.StatusInternalServerError, "failed to create process")
		return
	}

	dto := marshalProcess(process)

	// Broadcast update
	s.Broadcast(map[string]interface{}{
		"type":    "process_created",
		"process": dto,
	})

	writeJSON(w, http.StatusCreated, dto)
}

// HandleTerminateProcess terminates a process
func (s *Server) HandleTerminateProcess(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pid := vars["id"]

	if err := s.processManager.TerminateProcess(pid); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to terminate process")
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
		writeError(w, http.StatusInternalServerError, "failed to fork process")
		return
	}

	childDTO := marshalProcess(child)

	// Broadcast update
	s.Broadcast(map[string]interface{}{
		"type":   "process_forked",
		"parent": pid,
		"child":  childDTO,
	})

	writeJSON(w, http.StatusCreated, childDTO)
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
		writeError(w, http.StatusInternalServerError, "memory access failed")
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
		writeError(w, http.StatusNotFound, "page table not found")
		return
	}

	writeJSON(w, http.StatusOK, pageTable.GetAllPages())
}

// HandleGetHistory returns historical metrics
func (s *Server) HandleGetHistory(w http.ResponseWriter, r *http.Request) {
	last := clampedQueryInt(r, "last", 100, 1, maxHistoryItems)
	history := s.monitor.GetHistory(last)
	writeJSON(w, http.StatusOK, history)
}

// HandleGetEvents returns recent events
func (s *Server) HandleGetEvents(w http.ResponseWriter, r *http.Request) {
	last := clampedQueryInt(r, "last", 50, 1, maxHistoryItems)
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

	algType, ok := algorithms.ParseAlgorithmType(req.Algorithm)
	if !ok {
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

// HandleRunSimulation runs a simulation scenario.
// At most one simulation may run at a time; concurrent requests get 409.
func (s *Server) HandleRunSimulation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scenario string `json:"scenario"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Scenario == "" {
		writeError(w, http.StatusBadRequest, "scenario is required")
		return
	}

	if !s.simulationMu.TryLock() {
		writeError(w, http.StatusConflict, "a simulation is already running")
		return
	}

	go func() {
		defer s.simulationMu.Unlock()

		result, err := s.simulator.RunScenario(req.Scenario)
		if err != nil {
			// Do not leak internal error details over the wire.
			slog.Error("simulation failed", "scenario", req.Scenario, "error", err)
			s.Broadcast(map[string]interface{}{
				"type":     "simulation_error",
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

// HandleGetNumaStats returns NUMA statistics
func (s *Server) HandleGetNumaStats(w http.ResponseWriter, r *http.Request) {
	nm := s.memoryManager.GetNumaManager()
	nodes := nm.GetNodes()
	writeJSON(w, http.StatusOK, nodes)
}

// HandleGetCompressionStats returns compression statistics
func (s *Server) HandleGetCompressionStats(w http.ResponseWriter, r *http.Request) {
	cm := s.memoryManager.GetCompressionManager()
	writeJSON(w, http.StatusOK, cm.GetStats())
}

// HandleEnableFeature toggles advanced features
func (s *Server) HandleEnableFeature(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Feature string `json:"feature"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	switch req.Feature {
	case "numa":
		s.memoryManager.EnableNuma(req.Enabled)
	case "compression":
		s.memoryManager.EnableCompression(req.Enabled)
	case "clustering":
		s.memoryManager.EnableClustering(req.Enabled)
	default:
		// Do not echo back req.Feature; it is untrusted user input.
		writeError(w, http.StatusBadRequest, "unknown feature; valid values: numa, compression, clustering")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"feature": req.Feature,
		"enabled": req.Enabled,
	})
}

// HandleMapHugePage allocates a 2MB huge-page mapping for a process.
// POST /api/processes/{id}/hugepage   Body: {"huge_page": N}
func (s *Server) HandleMapHugePage(w http.ResponseWriter, r *http.Request) {
	pid := mux.Vars(r)["id"]

	var req struct {
		HugePage uint64 `json:"huge_page"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// A huge-page index N maps to virtual address N<<21. Indices >= 2^43 overflow
	// a 64-bit address, so reject them before the shift is performed.
	if req.HugePage > (1<<43)-1 {
		writeError(w, http.StatusBadRequest, "huge_page index out of range")
		return
	}

	frameID, err := s.memoryManager.MapHugePage(pid, req.HugePage)
	if err != nil {
		slog.Error("MapHugePage failed", "pid", pid, "hugepage_idx", req.HugePage, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to map huge page")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"process_id":      pid,
		"huge_page_index": req.HugePage,
		"frame_id":        frameID,
		"virtual_addr":    req.HugePage << 21,
		"size":            "2MB",
	})
}

// HandleGetHugePages returns all huge-page mappings for a process.
// GET /api/processes/{id}/hugepages
func (s *Server) HandleGetHugePages(w http.ResponseWriter, r *http.Request) {
	pid := mux.Vars(r)["id"]

	pages, err := s.memoryManager.GetHugePages(pid)
	if err != nil {
		writeError(w, http.StatusNotFound, "process not found")
		return
	}
	writeJSON(w, http.StatusOK, pages)
}

// HandleGetWorkingSet returns the current working-set size for a process.
// GET /api/processes/{id}/workingset
func (s *Server) HandleGetWorkingSet(w http.ResponseWriter, r *http.Request) {
	pid := mux.Vars(r)["id"]

	info, err := s.memoryManager.GetWorkingSetInfo(pid)
	if err != nil {
		writeError(w, http.StatusNotFound, "process not found")
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// HandleGetMultiLevelPageTable returns a multi-level page table for a process
func (s *Server) HandleGetMultiLevelPageTable(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pid := vars["id"]

	mpt, err := s.memoryManager.GetMultiLevelPageTable(pid)
	if err != nil {
		writeError(w, http.StatusNotFound, "process not found")
		return
	}

	entries := make([]map[string]interface{}, 0)
	mpt.WalkPages(func(addr uint64, entry *memory.PageTableEntry, huge bool) {
		entries = append(entries, map[string]interface{}{
			"virtual_addr": addr,
			"frame":        entry.FrameNumber,
			"present":      entry.Present.Load(),
			"dirty":        entry.Dirty.Load(),
			"huge":         huge,
		})
	})

	writeJSON(w, http.StatusOK, entries)
}

// HandleCompareAlgorithms runs all algorithms on the same scenario and returns
// a ranked list sorted by ascending fault rate.
// POST /api/simulation/compare
// Body: {"scenario":"locality","frames":32,"tlb":16}
func (s *Server) HandleCompareAlgorithms(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scenario string `json:"scenario"`
		Frames   int32  `json:"frames"`
		TLB      int    `json:"tlb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Frames <= 0 {
		req.Frames = 32
	}
	if req.TLB <= 0 {
		req.TLB = 16
	}
	if req.Scenario == "" {
		req.Scenario = "mixed"
	}
	if req.Frames > 65536 {
		writeError(w, http.StatusBadRequest, "frames exceeds maximum of 65536")
		return
	}

	results, err := s.simulator.CompareAlgorithms(req.Scenario, req.Frames, req.TLB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "simulation failed")
		return
	}
	writeJSON(w, http.StatusOK, results)
}

// HandleFrameCountSweep sweeps frame counts for one algorithm and returns the
// Belady curve (fault rate vs. frame count).
// POST /api/simulation/sweep
// Body: {"scenario":"locality","algorithm":"LRU","frame_min":4,"frame_max":64,"tlb":16}
func (s *Server) HandleFrameCountSweep(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scenario  string `json:"scenario"`
		Algorithm string `json:"algorithm"`
		FrameMin  int32  `json:"frame_min"`
		FrameMax  int32  `json:"frame_max"`
		TLB       int    `json:"tlb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Algorithm == "" {
		req.Algorithm = "LRU"
	}
	if req.Scenario == "" {
		req.Scenario = "mixed"
	}
	if req.FrameMin < 2 {
		req.FrameMin = 2
	}
	if req.FrameMax <= req.FrameMin {
		req.FrameMax = req.FrameMin * 8
	}
	if req.FrameMax > 65536 {
		req.FrameMax = 65536
	}
	if req.TLB <= 0 {
		req.TLB = 16
	}

	frameCounts := geometricFrameRange(req.FrameMin, req.FrameMax, 10)
	results, err := s.simulator.CompareFrameCounts(req.Scenario, req.Algorithm, frameCounts, req.TLB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "sweep failed")
		return
	}
	writeJSON(w, http.StatusOK, results)
}

// HandleGetThrashingStatus returns current thrashing detector state.
// GET /api/simulation/thrashing
func (s *Server) HandleGetThrashingStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.monitor.GetThrashingInfo())
}

// geometricFrameRange generates up to maxPoints frame counts in a geometric
// progression from min to max (inclusive).
// int64 intermediates prevent overflow when curr > MaxInt32/2.
func geometricFrameRange(min, max int32, maxPoints int) []int32 {
	result := []int32{min}
	curr := int64(min)
	for len(result) < maxPoints && curr < int64(max) {
		next := curr * 2
		if next > int64(max) {
			next = int64(max)
		}
		result = append(result, int32(next))
		curr = next
	}
	return result
}

// HandleHealthz is a liveness probe — returns 200 when the process is alive.
func (s *Server) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleReadyz is a readiness probe — returns 200 once the simulation engine
// is fully initialized and able to serve traffic.
func (s *Server) HandleReadyz(w http.ResponseWriter, r *http.Request) {
	if s.memoryManager == nil || s.processManager == nil {
		writeError(w, http.StatusServiceUnavailable, "not ready")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// Helper functions

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("writeJSON encode", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// clampedQueryInt reads a URL query parameter as an integer, returning
// defaultVal if absent or unparseable, and clamping to [min, max].
func clampedQueryInt(r *http.Request, param string, defaultVal, min, max int) int {
	val := defaultVal
	if s := r.URL.Query().Get(param); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			val = n
		}
	}
	if val < min {
		val = min
	}
	if val > max {
		val = max
	}
	return val
}

// SetupRoutes sets up all API routes
func SetupRoutes(s *Server) *mux.Router {
	r := mux.NewRouter()

	// Health probes (outside /api prefix so load-balancers and k8s can reach them cheaply)
	r.HandleFunc("/healthz", s.HandleHealthz).Methods("GET")
	r.HandleFunc("/readyz", s.HandleReadyz).Methods("GET")

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
	api.HandleFunc("/processes/{id}/hugepage", s.HandleMapHugePage).Methods("POST")
	api.HandleFunc("/processes/{id}/hugepages", s.HandleGetHugePages).Methods("GET")
	api.HandleFunc("/processes/{id}/workingset", s.HandleGetWorkingSet).Methods("GET")

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
	api.HandleFunc("/simulation/compare", s.HandleCompareAlgorithms).Methods("POST")
	api.HandleFunc("/simulation/sweep", s.HandleFrameCountSweep).Methods("POST")
	api.HandleFunc("/simulation/thrashing", s.HandleGetThrashingStatus).Methods("GET")

	// Advanced features
	api.HandleFunc("/numa/stats", s.HandleGetNumaStats).Methods("GET")
	api.HandleFunc("/compression/stats", s.HandleGetCompressionStats).Methods("GET")
	api.HandleFunc("/feature/toggle", s.HandleEnableFeature).Methods("POST")
	api.HandleFunc("/processes/{id}/mpt", s.HandleGetMultiLevelPageTable).Methods("GET")

	// WebSocket
	api.HandleFunc("/ws", s.HandleWebSocket)

	return r
}
