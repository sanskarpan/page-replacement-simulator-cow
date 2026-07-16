// Package simulator drives pre-defined workload scenarios against the memory
// subsystem, collects results, and exposes comparison helpers for benchmarking
// algorithms and frame-count sweeps.
package simulator

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
	"github.com/page-replacement-cow/pkg/models"
)

// TraceEntry captures a single memory access for deterministic replay.
type TraceEntry struct {
	ProcessID   string `json:"process_id"`
	VirtualPage uint64 `json:"virtual_page"`
	Write       bool   `json:"write"`
}

// Trace is a recorded sequence of memory accesses.
type Trace struct {
	Entries []TraceEntry `json:"entries"`
	Seed    int64        `json:"seed"`
}

// Save marshals the trace to JSON and writes it to path.
func (t *Trace) Save(path string) error {
	data, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal trace: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// LoadTrace reads and unmarshals a trace from path.
func LoadTrace(path string) (*Trace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read trace: %w", err)
	}
	var t Trace
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("unmarshal trace: %w", err)
	}
	return &t, nil
}

// AlgorithmResult holds per-algorithm metrics from a comparison run.
type AlgorithmResult struct {
	Rank       int
	Algorithm  string
	FaultRate  float64
	HitRate    float64
	PageFaults int64
	PageHits   int64
	Evictions  int64
	CoWCopies  int64
	Duration   time.Duration
}

type Simulator struct {
	processManager *process.ProcessManager
	rng            *rand.Rand
	seed           int64
	mu             sync.Mutex
	scenarioMu     sync.Mutex
	fastMode       bool         // skip per-access delays when true
	recording      bool         // whether to append to traceEntries
	traceEntries   []TraceEntry // populated when recording == true
}

func NewSimulator(processManager *process.ProcessManager) *Simulator {
	seed := time.Now().UnixNano()
	return &Simulator{
		processManager: processManager,
		rng:            rand.New(rand.NewSource(seed)),
		seed:           seed,
	}
}

// SetFastMode disables the per-access 1ms delay when true, useful for
// benchmarks and algorithm comparison where wall-clock latency is irrelevant.
func (s *Simulator) SetFastMode(fast bool) { s.fastMode = fast }

// sleep respects fastMode — skips the delay in comparison/benchmark runs.
func (s *Simulator) sleep() {
	if !s.fastMode {
		time.Sleep(time.Millisecond)
	}
}

// accessMemory is a thin wrapper that records the access when tracing is active.
func (s *Simulator) accessMemory(pid string, page uint64, write bool) error {
	if s.recording {
		s.traceEntries = append(s.traceEntries, TraceEntry{ProcessID: pid, VirtualPage: page, Write: write})
	}
	return s.processManager.AccessMemory(pid, page, write)
}

// StartRecording begins capturing all memory accesses made through this simulator.
func (s *Simulator) StartRecording() {
	s.traceEntries = make([]TraceEntry, 0, 1024)
	s.recording = true
}

// StopRecording stops capture and returns the recorded trace.
func (s *Simulator) StopRecording() *Trace {
	s.recording = false
	t := &Trace{Entries: s.traceEntries, Seed: s.seed}
	s.traceEntries = nil
	return t
}

// ReplayTrace replays a previously recorded trace on the current process manager.
// Processes referenced by the trace must already exist.
func (s *Simulator) ReplayTrace(t *Trace) error {
	for _, e := range t.Entries {
		if err := s.processManager.AccessMemory(e.ProcessID, e.VirtualPage, e.Write); err != nil {
			return fmt.Errorf("replay failed at page %d (pid=%s write=%v): %w",
				e.VirtualPage, e.ProcessID, e.Write, err)
		}
		s.sleep()
	}
	return nil
}

// CompareAlgorithms runs scenario on every available algorithm with the same
// RNG seed and returns results ranked by page fault rate (ascending).
// numFrames and tlbSize configure each algorithm's memory manager.
func (s *Simulator) CompareAlgorithms(scenario string, numFrames int32, tlbSize int) ([]AlgorithmResult, error) {
	allAlgos := []algorithms.AlgorithmType{
		algorithms.AlgorithmOptimal,
		algorithms.AlgorithmOPTPlus,
		algorithms.AlgorithmLRU,
		algorithms.AlgorithmARC,
		algorithms.AlgorithmCAR,
		algorithms.AlgorithmCLOCK,
		algorithms.AlgorithmWSClock,
		algorithms.AlgorithmLFU,
		algorithms.AlgorithmNRU,
		algorithms.AlgorithmPFF,
		algorithms.AlgorithmFIFO,
		algorithms.AlgorithmRandom,
	}

	results := make([]AlgorithmResult, 0, len(allAlgos))

	for _, algType := range allAlgos {
		mm := memory.NewMemoryManager(numFrames, tlbSize, algType)
		pm := process.NewProcessManager(mm)

		sim := &Simulator{
			processManager: pm,
			seed:           s.seed,
			rng:            rand.New(rand.NewSource(s.seed)),
			fastMode:       true,
		}

		start := time.Now()
		_, err := sim.RunScenario(scenario)
		elapsed := time.Since(start)
		mm.Close()

		if err != nil {
			// skip algorithms that error rather than aborting the whole comparison
			continue
		}

		m := mm.GetMetrics()
		results = append(results, AlgorithmResult{
			Algorithm:  algorithms.GetAlgorithmName(algType),
			FaultRate:  m.PageFaultRate,
			HitRate:    m.PageHitRate,
			PageFaults: m.PageFaults,
			PageHits:   m.PageHits,
			Evictions:  m.Evictions,
			CoWCopies:  m.CoWCopies,
			Duration:   elapsed,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].FaultRate < results[j].FaultRate
	})
	for i := range results {
		results[i].Rank = i + 1
	}
	return results, nil
}

// FrameCountResult holds per-frame-count fault metrics from a sweep run.
type FrameCountResult struct {
	NumFrames  int32   `json:"num_frames"`
	Algorithm  string  `json:"algorithm"`
	FaultRate  float64 `json:"fault_rate"`
	HitRate    float64 `json:"hit_rate"`
	PageFaults int64   `json:"page_faults"`
	PageHits   int64   `json:"page_hits"`
	Evictions  int64   `json:"evictions"`
}

// CompareFrameCounts sweeps across the given frame counts for a single algorithm,
// returning how the fault rate changes with available memory (the Belady curve).
// All runs use the same RNG seed as the parent simulator for fair comparison.
func (s *Simulator) CompareFrameCounts(scenario, algName string, frameCounts []int32, tlbSize int) ([]FrameCountResult, error) {
	algType, err := parseAlgorithmName(algName)
	if err != nil {
		return nil, err
	}

	results := make([]FrameCountResult, 0, len(frameCounts))
	for _, fc := range frameCounts {
		mm := memory.NewMemoryManager(fc, tlbSize, algType)
		pm := process.NewProcessManager(mm)
		sim := &Simulator{
			processManager: pm,
			seed:           s.seed,
			rng:            rand.New(rand.NewSource(s.seed)),
			fastMode:       true,
		}

		_, runErr := sim.RunScenario(scenario)
		mm.Close()
		if runErr != nil {
			continue
		}

		m := mm.GetMetrics()
		results = append(results, FrameCountResult{
			NumFrames:  fc,
			Algorithm:  algName,
			FaultRate:  m.PageFaultRate,
			HitRate:    m.PageHitRate,
			PageFaults: m.PageFaults,
			PageHits:   m.PageHits,
			Evictions:  m.Evictions,
		})
	}
	return results, nil
}

// parseAlgorithmName maps a human-readable algorithm name to its AlgorithmType.
func parseAlgorithmName(name string) (algorithms.AlgorithmType, error) {
	switch name {
	case "LRU":
		return algorithms.AlgorithmLRU, nil
	case "CLOCK":
		return algorithms.AlgorithmCLOCK, nil
	case "LFU":
		return algorithms.AlgorithmLFU, nil
	case "FIFO":
		return algorithms.AlgorithmFIFO, nil
	case "Optimal":
		return algorithms.AlgorithmOptimal, nil
	case "Random":
		return algorithms.AlgorithmRandom, nil
	case "ARC":
		return algorithms.AlgorithmARC, nil
	case "CAR":
		return algorithms.AlgorithmCAR, nil
	case "WSClock":
		return algorithms.AlgorithmWSClock, nil
	case "PFF":
		return algorithms.AlgorithmPFF, nil
	case "OPT+":
		return algorithms.AlgorithmOPTPlus, nil
	case "NRU":
		return algorithms.AlgorithmNRU, nil
	default:
		return algorithms.AlgorithmLRU, fmt.Errorf("unknown algorithm: %s", name)
	}
}

func (s *Simulator) SimulateSequentialAccess(pid string, startPage, numPages uint64, write bool) error {
	for i := uint64(0); i < numPages; i++ {
		if err := s.accessMemory(pid, startPage+i, write); err != nil {
			return fmt.Errorf("sequential access failed at page %d: %v", startPage+i, err)
		}
		s.sleep()
	}
	return nil
}

func (s *Simulator) SimulateRandomAccess(pid string, maxPage uint64, numAccesses int, writeRatio float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := 0; i < numAccesses; i++ {
		page := uint64(s.rng.Intn(int(maxPage)))
		write := s.rng.Float64() < writeRatio

		if err := s.accessMemory(pid, page, write); err != nil {
			return fmt.Errorf("random access failed at page %d: %v", page, err)
		}
		s.sleep()
	}
	return nil
}

func (s *Simulator) SimulateLocalityAccess(pid string, workingSetSize uint64, numAccesses int, writeRatio float64) error {
	if workingSetSize == 0 {
		return fmt.Errorf("working set size must be > 0")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	workingSet := make([]uint64, workingSetSize)
	for i := range workingSet {
		workingSet[i] = uint64(s.rng.Intn(1000))
	}

	for i := 0; i < numAccesses; i++ {
		var page uint64
		if s.rng.Float64() < 0.8 {
			page = workingSet[s.rng.Intn(len(workingSet))]
		} else {
			page = uint64(s.rng.Intn(1000))
		}

		write := s.rng.Float64() < writeRatio

		if err := s.accessMemory(pid, page, write); err != nil {
			return fmt.Errorf("locality access failed at page %d: %v", page, err)
		}
		s.sleep()
	}
	return nil
}

func (s *Simulator) SimulateLoopingAccess(pid string, loopSize uint64, numIterations int, writeRatio float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for iter := 0; iter < numIterations; iter++ {
		for i := uint64(0); i < loopSize; i++ {
			write := s.rng.Float64() < writeRatio
			if err := s.accessMemory(pid, i, write); err != nil {
				return fmt.Errorf("looping access failed at page %d: %v", i, err)
			}
			s.sleep()
		}
	}
	return nil
}

func (s *Simulator) SimulateCustomPattern(pid string, pages []uint64, writePages map[uint64]bool) error {
	for _, page := range pages {
		write := writePages[page]
		if err := s.accessMemory(pid, page, write); err != nil {
			return fmt.Errorf("custom pattern access failed at page %d: %v", page, err)
		}
		s.sleep()
	}
	return nil
}

func (s *Simulator) RunScenario(scenario string) (*ScenarioResult, error) {
	s.scenarioMu.Lock()
	defer s.scenarioMu.Unlock()

	result := &ScenarioResult{
		Scenario:  scenario,
		StartTime: time.Now(),
	}

	mm := s.processManager.GetMemoryManager()
	algName := mm.GetAlgorithm().GetName()
	if algName == "Optimal" || algName == "OPT+" {
		if err := s.precomputeAndSetOptimal(scenario, mm); err != nil {
			result.Error = err.Error()
			return result, err
		}
	}

	var err error

	switch scenario {
	case "sequential":
		err = s.runSequentialScenario()
	case "random":
		err = s.runRandomScenario()
	case "locality":
		err = s.runLocalityScenario()
	case "looping":
		err = s.runLoopingScenario()
	case "mixed":
		err = s.runMixedScenario()
	case "fork_cow":
		err = s.runForkCoWScenario()
	case "thrashing":
		err = s.runThrashingScenario()
	default:
		err = fmt.Errorf("unknown scenario: %s", scenario)
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Success = (err == nil)

	if err != nil {
		result.Error = err.Error()
	}

	result.Metrics = mm.GetMetrics()

	return result, err
}

func (s *Simulator) precomputeAndSetOptimal(scenario string, mm *memory.MemoryManager) error {
	accesses, err := s.precomputeAccesses(scenario)
	if err != nil {
		return err
	}
	mm.SetFutureAccesses(accesses)
	return nil
}

func (s *Simulator) precomputeAccesses(scenario string) ([]uint64, error) {
	rng := rand.New(rand.NewSource(s.seed))

	switch scenario {
	case "sequential":
		return s.precomputeSequentialAccesses(), nil
	case "random":
		return s.precomputeRandomAccesses(rng, 200, 100, 0.3), nil
	case "locality":
		return s.precomputeLocalityAccesses(rng, 20, 100, 0.2), nil
	case "looping":
		return s.precomputeLoopingAccesses(), nil
	case "mixed":
		return s.precomputeMixedAccesses(rng), nil
	case "fork_cow":
		return s.precomputeForkCoWAccesses(rng), nil
	case "thrashing":
		return s.precomputeRandomAccesses(rng, 1000, 200, 0.5), nil
	default:
		return nil, fmt.Errorf("unknown scenario: %s", scenario)
	}
}

func (s *Simulator) precomputeSequentialAccesses() []uint64 {
	accesses := make([]uint64, 100)
	for i := uint64(0); i < 100; i++ {
		accesses[i] = i
	}
	return accesses
}

func (s *Simulator) precomputeRandomAccesses(rng *rand.Rand, maxPage uint64, numAccesses int, writeRatio float64) []uint64 {
	accesses := make([]uint64, 0, numAccesses)
	for i := 0; i < numAccesses; i++ {
		page := uint64(rng.Intn(int(maxPage)))
		accesses = append(accesses, page)
		_ = rng.Float64() < writeRatio
	}
	return accesses
}

func (s *Simulator) precomputeLocalityAccesses(rng *rand.Rand, workingSetSize uint64, numAccesses int, writeRatio float64) []uint64 {
	workingSet := make([]uint64, workingSetSize)
	for i := range workingSet {
		workingSet[i] = uint64(rng.Intn(1000))
	}

	accesses := make([]uint64, 0, numAccesses)
	for i := 0; i < numAccesses; i++ {
		var page uint64
		if rng.Float64() < 0.8 {
			page = workingSet[rng.Intn(len(workingSet))]
		} else {
			page = uint64(rng.Intn(1000))
		}
		accesses = append(accesses, page)
		_ = rng.Float64() < writeRatio
	}
	return accesses
}

func (s *Simulator) precomputeLoopingAccesses() []uint64 {
	accesses := make([]uint64, 0, 15*5)
	for iter := 0; iter < 5; iter++ {
		for i := uint64(0); i < 15; i++ {
			accesses = append(accesses, i)
		}
	}
	return accesses
}

func (s *Simulator) precomputeMixedAccesses(rng *rand.Rand) []uint64 {
	accesses := make([]uint64, 0, 80)
	for i := uint64(0); i < 20; i++ {
		accesses = append(accesses, i)
	}
	for i := 0; i < 30; i++ {
		page := uint64(rng.Intn(100))
		accesses = append(accesses, page)
		_ = rng.Float64() < 0.5
	}
	ws := make([]uint64, 10)
	for i := range ws {
		ws[i] = uint64(rng.Intn(1000))
	}
	for i := 0; i < 30; i++ {
		var page uint64
		if rng.Float64() < 0.8 {
			page = ws[rng.Intn(len(ws))]
		} else {
			page = uint64(rng.Intn(1000))
		}
		accesses = append(accesses, page)
		_ = rng.Float64() < 0.3
	}
	return accesses
}

func (s *Simulator) precomputeForkCoWAccesses(rng *rand.Rand) []uint64 {
	accesses := make([]uint64, 0, 70)
	for i := uint64(0); i < 30; i++ {
		accesses = append(accesses, i)
	}
	for i := 0; i < 10; i++ {
		page := uint64(rng.Intn(30))
		accesses = append(accesses, page)
		_ = rng.Float64() < 0.0
	}
	for i := 0; i < 10; i++ {
		page := uint64(rng.Intn(30))
		accesses = append(accesses, page)
		_ = rng.Float64() < 0.0
	}
	for i := 0; i < 10; i++ {
		page := uint64(rng.Intn(30))
		accesses = append(accesses, page)
		_ = rng.Float64() < 1.0
	}
	for i := 0; i < 10; i++ {
		page := uint64(rng.Intn(30))
		accesses = append(accesses, page)
		_ = rng.Float64() < 1.0
	}
	return accesses
}

func (s *Simulator) runSequentialScenario() error {
	proc, err := s.processManager.CreateProcess("Sequential", 1, 1000)
	if err != nil {
		return err
	}

	return s.SimulateSequentialAccess(proc.ID, 0, 100, false)
}

func (s *Simulator) runRandomScenario() error {
	proc, err := s.processManager.CreateProcess("Random", 1, 1000)
	if err != nil {
		return err
	}

	return s.SimulateRandomAccess(proc.ID, 200, 100, 0.3)
}

func (s *Simulator) runLocalityScenario() error {
	proc, err := s.processManager.CreateProcess("Locality", 1, 1000)
	if err != nil {
		return err
	}

	return s.SimulateLocalityAccess(proc.ID, 20, 100, 0.2)
}

func (s *Simulator) runLoopingScenario() error {
	proc, err := s.processManager.CreateProcess("Looping", 1, 1000)
	if err != nil {
		return err
	}

	return s.SimulateLoopingAccess(proc.ID, 15, 5, 0.4)
}

func (s *Simulator) runMixedScenario() error {
	proc, err := s.processManager.CreateProcess("Mixed", 1, 1000)
	if err != nil {
		return err
	}

	if err := s.SimulateSequentialAccess(proc.ID, 0, 20, false); err != nil {
		return err
	}

	if err := s.SimulateRandomAccess(proc.ID, 100, 30, 0.5); err != nil {
		return err
	}

	return s.SimulateLocalityAccess(proc.ID, 10, 30, 0.3)
}

func (s *Simulator) runForkCoWScenario() error {
	parent, err := s.processManager.CreateProcess("Parent", 1, 1000)
	if err != nil {
		return err
	}

	if err := s.SimulateSequentialAccess(parent.ID, 0, 30, false); err != nil {
		return err
	}

	child, err := s.processManager.ForkProcess(parent.ID)
	if err != nil {
		return err
	}

	if err := s.SimulateRandomAccess(parent.ID, 30, 10, 0.0); err != nil {
		return err
	}
	if err := s.SimulateRandomAccess(child.ID, 30, 10, 0.0); err != nil {
		return err
	}

	if err := s.SimulateRandomAccess(child.ID, 30, 10, 1.0); err != nil {
		return err
	}

	return s.SimulateRandomAccess(parent.ID, 30, 10, 1.0)
}

func (s *Simulator) runThrashingScenario() error {
	proc, err := s.processManager.CreateProcess("Thrashing", 1, 10000)
	if err != nil {
		return err
	}

	return s.SimulateRandomAccess(proc.ID, 1000, 200, 0.5)
}

type ScenarioResult struct {
	Scenario  string
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
	Success   bool
	Error     string
	Metrics   *models.MetricsSnapshot
}

func (s *Simulator) GetAvailableScenarios() []string {
	return []string{
		"sequential",
		"random",
		"locality",
		"looping",
		"mixed",
		"fork_cow",
		"thrashing",
	}
}

func (s *Simulator) GetScenarioDescription(scenario string) string {
	descriptions := map[string]string{
		"sequential": "Sequential memory access pattern",
		"random":     "Random memory access pattern",
		"locality":   "Temporal locality with working set",
		"looping":    "Looping access pattern",
		"mixed":      "Mixed access patterns (sequential + random + locality)",
		"fork_cow":   "Process fork with Copy-on-Write demonstration",
		"thrashing":  "Memory thrashing scenario",
	}

	desc, exists := descriptions[scenario]
	if !exists {
		return "Unknown scenario"
	}
	return desc
}