package simulator

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
	"github.com/page-replacement-cow/pkg/models"
)

type Simulator struct {
	processManager *process.ProcessManager
	rng            *rand.Rand
	seed           int64
	mu             sync.Mutex
	scenarioMu     sync.Mutex
}

func NewSimulator(processManager *process.ProcessManager) *Simulator {
	seed := time.Now().UnixNano()
	return &Simulator{
		processManager: processManager,
		rng:            rand.New(rand.NewSource(seed)),
		seed:           seed,
	}
}

func (s *Simulator) SimulateSequentialAccess(pid string, startPage, numPages uint64, write bool) error {
	for i := uint64(0); i < numPages; i++ {
		if err := s.processManager.AccessMemory(pid, startPage+i, write); err != nil {
			return fmt.Errorf("sequential access failed at page %d: %v", startPage+i, err)
		}
		time.Sleep(1 * time.Millisecond)
	}
	return nil
}

func (s *Simulator) SimulateRandomAccess(pid string, maxPage uint64, numAccesses int, writeRatio float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := 0; i < numAccesses; i++ {
		page := uint64(s.rng.Intn(int(maxPage)))
		write := s.rng.Float64() < writeRatio

		if err := s.processManager.AccessMemory(pid, page, write); err != nil {
			return fmt.Errorf("random access failed at page %d: %v", page, err)
		}
		time.Sleep(1 * time.Millisecond)
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

		if err := s.processManager.AccessMemory(pid, page, write); err != nil {
			return fmt.Errorf("locality access failed at page %d: %v", page, err)
		}
		time.Sleep(1 * time.Millisecond)
	}
	return nil
}

func (s *Simulator) SimulateLoopingAccess(pid string, loopSize uint64, numIterations int, writeRatio float64) error {
	for iter := 0; iter < numIterations; iter++ {
		for i := uint64(0); i < loopSize; i++ {
			write := s.rng.Float64() < writeRatio
			if err := s.processManager.AccessMemory(pid, i, write); err != nil {
				return fmt.Errorf("looping access failed at page %d: %v", i, err)
			}
			time.Sleep(1 * time.Millisecond)
		}
	}
	return nil
}

func (s *Simulator) SimulateCustomPattern(pid string, pages []uint64, writePages map[uint64]bool) error {
	for _, page := range pages {
		write := writePages[page]
		if err := s.processManager.AccessMemory(pid, page, write); err != nil {
			return fmt.Errorf("custom pattern access failed at page %d: %v", page, err)
		}
		time.Sleep(1 * time.Millisecond)
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
	if mm.GetAlgorithm().GetName() == "Optimal" {
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