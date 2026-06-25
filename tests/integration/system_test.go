package integration

import (
	"testing"
	"time"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
	"github.com/page-replacement-cow/internal/simulator"
	"github.com/page-replacement-cow/pkg/models"
)

// TestBasicMemoryAccess tests basic memory access functionality
func TestBasicMemoryAccess(t *testing.T) {
	mm := memory.NewMemoryManager(64, 16, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	// Create a process
	proc, err := pm.CreateProcess("Test", 1, 100)
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}

	// Access some pages
	for i := uint64(0); i < 10; i++ {
		if err := pm.AccessMemory(proc.ID, i, false); err != nil {
			t.Fatalf("Failed to access page %d: %v", i, err)
		}
	}

	// Check metrics
	metrics := mm.GetMetrics()

	if metrics.PageFaults < 10 {
		t.Errorf("Expected at least 10 page faults, got %d", metrics.PageFaults)
	}

	if metrics.UsedFrames == 0 {
		t.Error("Expected some frames to be used")
	}

	t.Logf("Basic memory access test passed")
	t.Logf("  Page Faults: %d", metrics.PageFaults)
	t.Logf("  Page Hits: %d", metrics.PageHits)
	t.Logf("  Used Frames: %d", metrics.UsedFrames)
}

// TestPageReplacement tests page replacement functionality
func TestPageReplacement(t *testing.T) {
	mm := memory.NewMemoryManager(16, 8, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	proc, err := pm.CreateProcess("Test", 1, 100)
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}

	// Access more pages than available frames
	for i := uint64(0); i < 30; i++ {
		if err := pm.AccessMemory(proc.ID, i, false); err != nil {
			t.Fatalf("Failed to access page %d: %v", i, err)
		}
	}

	metrics := mm.GetMetrics()

	// Should have evictions
	if metrics.Evictions == 0 {
		t.Error("Expected some evictions to occur")
	}

	// Should have page faults
	if metrics.PageFaults < 30 {
		t.Errorf("Expected at least 30 page faults, got %d", metrics.PageFaults)
	}

	t.Logf("Page replacement test passed")
	t.Logf("  Evictions: %d", metrics.Evictions)
	t.Logf("  Page Faults: %d", metrics.PageFaults)
	t.Logf("  Page Hits: %d", metrics.PageHits)
}

// TestCopyOnWrite tests copy-on-write functionality
func TestCopyOnWrite(t *testing.T) {
	mm := memory.NewMemoryManager(64, 16, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	// Create parent process
	parent, err := pm.CreateProcess("Parent", 1, 100)
	if err != nil {
		t.Fatalf("Failed to create parent: %v", err)
	}

	// Parent accesses some pages
	for i := uint64(0); i < 10; i++ {
		if err := pm.AccessMemory(parent.ID, i, false); err != nil {
			t.Fatalf("Failed to access page %d: %v", i, err)
		}
	}

	// Fork child
	child, err := pm.ForkProcess(parent.ID)
	if err != nil {
		t.Fatalf("Failed to fork process: %v", err)
	}

	// Child reads shared pages (no CoW)
	for i := uint64(0); i < 10; i++ {
		if err := pm.AccessMemory(child.ID, i, false); err != nil {
			t.Fatalf("Failed to access page %d in child: %v", i, err)
		}
	}

	metricsBeforeWrite := mm.GetMetrics()

	// Child writes to shared pages (triggers CoW)
	for i := uint64(0); i < 5; i++ {
		if err := pm.AccessMemory(child.ID, i, true); err != nil {
			t.Fatalf("Failed to write page %d in child: %v", i, err)
		}
	}

	metricsAfterWrite := mm.GetMetrics()

	// Should have CoW copies
	cowCopies := metricsAfterWrite.CoWCopies - metricsBeforeWrite.CoWCopies
	if cowCopies == 0 {
		t.Error("Expected CoW copies to occur")
	}

	t.Logf("Copy-on-Write test passed")
	t.Logf("  CoW Copies: %d", cowCopies)
	t.Logf("  Shared Pages: %d", metricsAfterWrite.SharedPages)
}

// TestMultipleProcesses tests multiple processes
func TestMultipleProcesses(t *testing.T) {
	mm := memory.NewMemoryManager(128, 16, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	// Create multiple processes
	numProcesses := 5
	processes := make([]*models.Process, numProcesses)

	for i := 0; i < numProcesses; i++ {
		proc, err := pm.CreateProcess("Process", 1, 100)
		if err != nil {
			t.Fatalf("Failed to create process %d: %v", i, err)
		}
		processes[i] = proc
	}

	// Each process accesses different pages
	for i, proc := range processes {
		for j := uint64(0); j < 10; j++ {
			page := uint64(i*10) + j
			if err := pm.AccessMemory(proc.ID, page, false); err != nil {
				t.Fatalf("Failed to access page %d in process %d: %v", page, i, err)
			}
		}
	}

	metrics := mm.GetMetrics()

	if metrics.TotalProcesses != int32(numProcesses) {
		t.Errorf("Expected %d processes, got %d", numProcesses, metrics.TotalProcesses)
	}

	t.Logf("Multiple processes test passed")
	t.Logf("  Processes: %d", metrics.TotalProcesses)
	t.Logf("  Total Accesses: %d", metrics.TotalAccesses)
}

// TestSimulationScenarios tests all simulation scenarios
func TestSimulationScenarios(t *testing.T) {
	mm := memory.NewMemoryManager(64, 16, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)
	sim := simulator.NewSimulator(pm)

	scenarios := sim.GetAvailableScenarios()

	for _, scenario := range scenarios {
		t.Run(scenario, func(t *testing.T) {
			// Reset system
			pm.Reset()
			mm.Reset()

			// Run scenario
			result, err := sim.RunScenario(scenario)
			if err != nil {
				t.Fatalf("Scenario %s failed: %v", scenario, err)
			}

			if !result.Success {
				t.Fatalf("Scenario %s was not successful", scenario)
			}

			t.Logf("Scenario %s passed in %v", scenario, result.Duration)

			if result.Metrics != nil {
				t.Logf("  Page Faults: %d", result.Metrics.PageFaults)
				t.Logf("  Page Hits: %d", result.Metrics.PageHits)
				t.Logf("  Fault Rate: %.2f%%", result.Metrics.PageFaultRate*100)
			}
		})
	}
}

// TestAlgorithmComparison compares different algorithms
func TestAlgorithmComparison(t *testing.T) {
	algorithms := []struct {
		name string
		algo algorithms.AlgorithmType
	}{
		{"LRU", algorithms.AlgorithmLRU},
		{"CLOCK", algorithms.AlgorithmCLOCK},
		{"LFU", algorithms.AlgorithmLFU},
		{"FIFO", algorithms.AlgorithmFIFO},
	}

	for _, alg := range algorithms {
		t.Run(alg.name, func(t *testing.T) {
			mm := memory.NewMemoryManager(32, 16, alg.algo)
			pm := process.NewProcessManager(mm)
			sim := simulator.NewSimulator(pm)

			// Run mixed scenario
			result, err := sim.RunScenario("mixed")
			if err != nil {
				t.Fatalf("Algorithm %s failed: %v", alg.name, err)
			}

			if result.Metrics != nil {
				t.Logf("%s Results:", alg.name)
				t.Logf("  Fault Rate: %.2f%%", result.Metrics.PageFaultRate*100)
				t.Logf("  Hit Rate: %.2f%%", result.Metrics.PageHitRate*100)
				t.Logf("  Evictions: %d", result.Metrics.Evictions)
			}
		})
	}
}

// TestTLB tests TLB functionality
func TestTLB(t *testing.T) {
	mm := memory.NewMemoryManager(64, 8, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	proc, err := pm.CreateProcess("Test", 1, 100)
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}

	// Access same pages multiple times
	for round := 0; round < 3; round++ {
		for i := uint64(0); i < 5; i++ {
			if err := pm.AccessMemory(proc.ID, i, false); err != nil {
				t.Fatalf("Failed to access page %d: %v", i, err)
			}
		}
	}

	tlb := mm.GetTLB()
	stats := tlb.GetStats()

	if stats.Hits == 0 {
		t.Error("Expected some TLB hits")
	}

	t.Logf("TLB test passed")
	t.Logf("  Hits: %d", stats.Hits)
	t.Logf("  Misses: %d", stats.Misses)
	t.Logf("  Hit Rate: %.2f%%", stats.HitRate*100)
}

// TestStressMultipleConcurrentAccesses tests concurrent memory accesses
func TestStressMultipleConcurrentAccesses(t *testing.T) {
	mm := memory.NewMemoryManager(128, 16, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	proc, err := pm.CreateProcess("Stress", 1, 1000)
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}

	// Concurrent accesses
	numWorkers := 10
	done := make(chan bool, numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			for j := 0; j < 50; j++ {
				page := uint64(workerID*50 + j)
				pm.AccessMemory(proc.ID, page, j%2 == 0)
				time.Sleep(1 * time.Millisecond)
			}
			done <- true
		}(i)
	}

	// Wait for all workers
	for i := 0; i < numWorkers; i++ {
		<-done
	}

	metrics := mm.GetMetrics()

	t.Logf("Stress test passed")
	t.Logf("  Total Accesses: %d", metrics.TotalAccesses)
	t.Logf("  Page Faults: %d", metrics.PageFaults)
	t.Logf("  Used Frames: %d", metrics.UsedFrames)
}

// TestRandomAlgorithm tests the Random algorithm in integration
func TestRandomAlgorithm(t *testing.T) {
	mm := memory.NewMemoryManager(32, 16, algorithms.AlgorithmRandom)
	pm := process.NewProcessManager(mm)

	proc, err := pm.CreateProcess("Test", 1, 100)
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}

	for i := uint64(0); i < 30; i++ {
		if err := pm.AccessMemory(proc.ID, i, false); err != nil {
			t.Fatalf("Failed to access page %d: %v", i, err)
		}
	}

	metrics := mm.GetMetrics()
	if metrics.TotalAccesses == 0 {
		t.Error("Expected some accesses")
	}

	t.Logf("Random algorithm test passed")
	t.Logf("  Total Accesses: %d", metrics.TotalAccesses)
	t.Logf("  Evictions: %d", metrics.Evictions)
}

// TestCoWForkChain tests parent -> child -> grandchild fork chain
func TestCoWForkChain(t *testing.T) {
	mm := memory.NewMemoryManager(64, 16, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	parent, err := pm.CreateProcess("Parent", 1, 100)
	if err != nil {
		t.Fatalf("Failed to create parent: %v", err)
	}

	for i := uint64(0); i < 10; i++ {
		if err := pm.AccessMemory(parent.ID, i, false); err != nil {
			t.Fatalf("Failed to access page %d: %v", i, err)
		}
	}

	child, err := pm.ForkProcess(parent.ID)
	if err != nil {
		t.Fatalf("Failed to fork child: %v", err)
	}

	grandchild, err := pm.ForkProcess(child.ID)
	if err != nil {
		t.Fatalf("Failed to fork grandchild: %v", err)
	}

	for i := uint64(0); i < 5; i++ {
		if err := pm.AccessMemory(grandchild.ID, i, true); err != nil {
			t.Fatalf("Failed to write page %d in grandchild: %v", i, err)
		}
	}

	metrics := mm.GetMetrics()
	if metrics.CoWCopies == 0 {
		t.Error("Expected CoW copies during fork chain")
	}

	t.Logf("CoW fork chain test passed")
	t.Logf("  CoW Copies: %d", metrics.CoWCopies)
	t.Logf("  Shared Pages: %d", metrics.SharedPages)
}

// TestTLBKeyCorrectness verifies TLB key format fix
func TestTLBKeyCorrectness(t *testing.T) {
	mm := memory.NewMemoryManager(64, 16, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	proc, err := pm.CreateProcess("Test", 1, 1000)
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}

	largePages := []uint64{1, 100, 1000, 100000, 1000000}
	for _, page := range largePages {
		if err := pm.AccessMemory(proc.ID, page, false); err != nil {
			t.Fatalf("Failed to access page %d: %v", page, err)
		}
	}

	for _, page := range largePages {
		if err := pm.AccessMemory(proc.ID, page, false); err != nil {
			t.Fatalf("Failed to re-access page %d: %v", page, err)
		}
	}

	tlb := mm.GetTLB()
	stats := tlb.GetStats()

	if stats.Hits == 0 {
		t.Error("TLB should have hits for re-accessed pages")
	}

	t.Logf("TLB key correctness test passed")
	t.Logf("  TLB Hits: %d, Misses: %d, Hit Rate: %.2f%%", stats.Hits, stats.Misses, stats.HitRate*100)
}

// TestCoWCopyCountAccuracy verifies CoW copy count is not double-counted
func TestCoWCopyCountAccuracy(t *testing.T) {
	mm := memory.NewMemoryManager(64, 16, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	parent, err := pm.CreateProcess("Parent", 1, 100)
	if err != nil {
		t.Fatalf("Failed to create parent: %v", err)
	}

	for i := uint64(0); i < 10; i++ {
		if err := pm.AccessMemory(parent.ID, i, false); err != nil {
			t.Fatalf("Failed to access page %d: %v", i, err)
		}
	}

	child, err := pm.ForkProcess(parent.ID)
	if err != nil {
		t.Fatalf("Failed to fork child: %v", err)
	}

	metricsBefore := mm.GetMetrics()

	writeCount := 5
	for i := uint64(0); i < uint64(writeCount); i++ {
		if err := pm.AccessMemory(child.ID, i, true); err != nil {
			t.Fatalf("Failed to write page %d: %v", i, err)
		}
	}

	metricsAfter := mm.GetMetrics()
	cowCopies := metricsAfter.CoWCopies - metricsBefore.CoWCopies

	if cowCopies != int64(writeCount) {
		t.Errorf("Expected %d CoW copies, got %d (possible double counting)", writeCount, cowCopies)
	}

	t.Logf("CoW copy count accuracy test passed")
	t.Logf("  CoW Copies: %d (expected %d)", cowCopies, writeCount)
}

// TestConcurrentForkAndAccess tests concurrent fork and memory access
func TestConcurrentForkAndAccess(t *testing.T) {
	mm := memory.NewMemoryManager(128, 16, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	parent, err := pm.CreateProcess("Parent", 1, 1000)
	if err != nil {
		t.Fatalf("Failed to create parent: %v", err)
	}

	for i := uint64(0); i < 20; i++ {
		if err := pm.AccessMemory(parent.ID, i, false); err != nil {
			t.Fatalf("Failed to access page %d: %v", i, err)
		}
	}

	done := make(chan bool, 4)

	go func() {
		child, err := pm.ForkProcess(parent.ID)
		if err != nil {
			t.Logf("Fork failed: %v", err)
		}
		if child != nil {
			for i := uint64(0); i < 10; i++ {
				pm.AccessMemory(child.ID, i, true)
			}
		}
		done <- true
	}()

	go func() {
		for i := uint64(0); i < 20; i++ {
			pm.AccessMemory(parent.ID, i, false)
		}
		done <- true
	}()

	go func() {
		child2, err := pm.ForkProcess(parent.ID)
		if err != nil {
			t.Logf("Fork 2 failed: %v", err)
		}
		if child2 != nil {
			for i := uint64(0); i < 10; i++ {
				pm.AccessMemory(child2.ID, i, true)
			}
		}
		done <- true
	}()

	go func() {
		for i := uint64(0); i < 20; i++ {
			pm.AccessMemory(parent.ID, i, false)
		}
		done <- true
	}()

	for i := 0; i < 4; i++ {
		<-done
	}

	metrics := mm.GetMetrics()

	t.Logf("Concurrent fork+access test passed")
	t.Logf("  Total Accesses: %d", metrics.TotalAccesses)
	t.Logf("  CoW Copies: %d", metrics.CoWCopies)
	t.Logf("  Processes: %d", metrics.TotalProcesses)
}
