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

func TestARCAlgorithmIntegration(t *testing.T) {
	mm := memory.NewMemoryManager(32, 16, algorithms.AlgorithmARC)
	pm := process.NewProcessManager(mm)

	proc, err := pm.CreateProcess("ARC-Test", 1, 200)
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}

	for i := uint64(0); i < 50; i++ {
		if err := pm.AccessMemory(proc.ID, i, true); err != nil {
			t.Fatalf("Failed to access page %d: %v", i, err)
		}
	}

	for i := uint64(0); i < 25; i++ {
		if err := pm.AccessMemory(proc.ID, i, false); err != nil {
			t.Fatalf("Failed to re-access page %d: %v", i, err)
		}
	}

	metrics := mm.GetMetrics()
	if metrics.TotalAccesses < 75 {
		t.Error("Expected all accesses to complete under ARC")
	}
	if metrics.Evictions < 15 {
		t.Logf("ARC had low evictions: %d (adaptive)", metrics.Evictions)
	}

	t.Logf("ARC integration test passed")
	t.Logf("  Accesses: %d, Evictions: %d, Fault Rate: %.2f%%", metrics.TotalAccesses, metrics.Evictions, metrics.PageFaultRate*100)
}

func TestCARAlgorithmIntegration(t *testing.T) {
	mm := memory.NewMemoryManager(32, 16, algorithms.AlgorithmCAR)
	pm := process.NewProcessManager(mm)

	proc, err := pm.CreateProcess("CAR-Test", 1, 200)
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}

	for i := uint64(0); i < 50; i++ {
		if err := pm.AccessMemory(proc.ID, i, true); err != nil {
			t.Fatalf("Failed to access page %d: %v", i, err)
		}
	}

	for i := uint64(0); i < 20; i++ {
		if err := pm.AccessMemory(proc.ID, i, false); err != nil {
			t.Fatalf("Failed to re-access page %d: %v", i, err)
		}
	}

	metrics := mm.GetMetrics()
	if metrics.TotalAccesses < 70 {
		t.Error("Expected all accesses to complete under CAR")
	}

	t.Logf("CAR integration test passed")
	t.Logf("  Accesses: %d, Fault Rate: %.2f%%", metrics.TotalAccesses, metrics.PageFaultRate*100)
}

func TestWSClockIntegration(t *testing.T) {
	mm := memory.NewMemoryManager(32, 16, algorithms.AlgorithmWSClock)
	pm := process.NewProcessManager(mm)

	proc, err := pm.CreateProcess("WSClock-Test", 1, 200)
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}

	for i := uint64(0); i < 40; i++ {
		if err := pm.AccessMemory(proc.ID, i, false); err != nil {
			t.Fatalf("Failed to access page %d: %v", i, err)
		}
	}

	for i := uint64(0); i < 15; i++ {
		if err := pm.AccessMemory(proc.ID, i, false); err != nil {
			t.Fatalf("Failed to re-access page %d: %v", i, err)
		}
	}

	metrics := mm.GetMetrics()
	if metrics.TotalAccesses < 55 {
		t.Error("Expected all accesses to complete under WSClock")
	}

	t.Logf("WSClock integration test passed")
	t.Logf("  Accesses: %d, Fault Rate: %.2f%%", metrics.TotalAccesses, metrics.PageFaultRate*100)
}

func TestPFFResidentSet(t *testing.T) {
	mm := memory.NewMemoryManager(64, 16, algorithms.AlgorithmPFF)
	pm := process.NewProcessManager(mm)

	proc, err := pm.CreateProcess("PFF-Test", 1, 500)
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}

	for i := uint64(0); i < 100; i++ {
		if err := pm.AccessMemory(proc.ID, i, false); err != nil {
			t.Fatalf("Failed to access page %d: %v", i, err)
		}
	}

	metrics := mm.GetMetrics()
	if metrics.TotalAccesses < 100 {
		t.Error("Expected all accesses to complete under PFF")
	}

	t.Logf("PFF resident set test passed")
	t.Logf("  Accesses: %d, Fault Rate: %.2f%%", metrics.TotalAccesses, metrics.PageFaultRate*100)
	t.Logf("  Used Frames: %d / %d", metrics.UsedFrames, metrics.TotalFrames)
}

func TestOPTPlusIntegration(t *testing.T) {
	mm := memory.NewMemoryManager(32, 16, algorithms.AlgorithmOPTPlus)
	pm := process.NewProcessManager(mm)

	proc, err := pm.CreateProcess("OPT+-Test", 1, 200)
	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}

	pages := []uint64{0, 1, 2, 3, 0, 1, 4, 0, 1, 2, 3, 4, 5, 0, 1, 2, 3, 4, 5, 6, 0, 1, 6, 0, 1, 2, 3, 4, 5, 6}
	for _, page := range pages {
		if err := pm.AccessMemory(proc.ID, page, false); err != nil {
			t.Fatalf("Failed to access page %d: %v", page, err)
		}
	}

	metrics := mm.GetMetrics()
	if metrics.TotalAccesses < 30 {
		t.Error("Expected all accesses to complete under OPT+")
	}

	t.Logf("OPT+ integration test passed")
	t.Logf("  Accesses: %d, Fault Rate: %.2f%%", metrics.TotalAccesses, metrics.PageFaultRate*100)
}

func TestNumaAwareness(t *testing.T) {
	nm := memory.NewNumaManager()
	nm.AddNode(models.NewNumaNode(0, "Node-0", 100, 64))
	nm.AddNode(models.NewNumaNode(1, "Node-1", 150, 64))

	nodes := nm.GetNodes()
	if len(nodes) != 2 {
		t.Errorf("Expected 2 NUMA nodes, got %d", len(nodes))
	}

	node, err := nm.GetNode(0)
	if err != nil {
		t.Fatalf("Failed to get NUMA node 0: %v", err)
	}
	if node.AccessCostNs != 100 {
		t.Errorf("Expected access cost 100ns, got %d", node.AccessCostNs)
	}

	closest, _ := nm.GetClosestNode(0)
	if closest.ID != 0 {
		t.Errorf("Expected closest node 0, got %d", closest.ID)
	}

	t.Logf("NUMA awareness test passed - %d nodes configured", len(nodes))
}

func TestPageClustering(t *testing.T) {
	pcm := memory.NewPageClusterManager(4, 16)

	seqPages := []uint64{10, 11, 12}
	cluster := pcm.DetectSequential("test", seqPages)
	if cluster == nil {
		t.Fatal("Expected to detect sequential pattern")
	}
	if !cluster.Sequential {
		t.Error("Expected sequential cluster")
	}

	prefetch := pcm.GetPrefetchPages(10)
	if len(prefetch) != 16 {
		t.Errorf("Expected 16 prefetch pages, got %d", len(prefetch))
	}

	nonSeqPages := []uint64{5, 20, 3}
	noCluster := pcm.DetectSequential("test2", nonSeqPages)
	if noCluster != nil {
		t.Error("Expected no cluster for non-sequential access")
	}

	t.Logf("Page clustering test passed")
}

func TestCompressionManager(t *testing.T) {
	cm := memory.NewCompressionManager(0.5)

	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 256)
	}

	cp := cm.CompressPage(100, data)
	if cp != nil {
		if cp.OriginalSize != int64(len(data)) {
			t.Error("Compression preserved original size")
		}
		t.Logf("Compressed: %d -> %d bytes (%.1f%%)", cp.OriginalSize, cp.CompressedSize, 100*float64(cp.CompressedSize)/float64(cp.OriginalSize))
	}

	stats := cm.GetStats()
	if stats.PagesCompressed > 0 {
		t.Logf("Compression stats: %d pages, ratio %.2f", stats.PagesCompressed, stats.CompressionRatio)
	}

	t.Logf("Compression manager test passed")
}

func TestMultiLevelPageTable(t *testing.T) {
	mlpt := memory.NewMultiLevelPageTable("test-process")

	addr1 := uint64(0x1000)
	mlpt.SetEntry(addr1, 42, false)
	entry := mlpt.GetEntry(addr1)
	if entry == nil || !entry.Present.Load() {
		t.Fatal("Expected page table entry to be present")
	}
	if entry.FrameNumber != 42 {
		t.Errorf("Expected frame 42, got %d", entry.FrameNumber)
	}

	addr2 := uint64(0x200000)
	mlpt.SetEntry(addr2, 10, true)
	hugeEntry := mlpt.GetEntry(addr2)
	if hugeEntry == nil || !hugeEntry.HugePage.Load() {
		t.Fatal("Expected huge page entry")
	}
	if hugeEntry.FrameNumber != 10 {
		t.Errorf("Expected frame 10 for huge page, got %d", hugeEntry.FrameNumber)
	}

	mlpt.InvalidateEntry(addr1)
	invalidated := mlpt.GetEntry(addr1)
	if invalidated != nil && invalidated.Present.Load() {
		t.Error("Expected entry to be invalidated")
	}

	count := 0
	mlpt.WalkPages(func(addr uint64, e *memory.PageTableEntry, huge bool) {
		count++
	})
	if count != 1 {
		t.Errorf("Expected 1 present page, got %d", count)
	}

	t.Logf("Multi-level page table test passed - %d present pages", count)
}
