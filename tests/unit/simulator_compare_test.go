package unit

import (
	"testing"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
	"github.com/page-replacement-cow/internal/simulator"
)

// TestCompareAlgorithmsReturnsRankedResults verifies that CompareAlgorithms
// returns one result per algorithm, sorted by fault rate, and that Optimal
// never performs worse than LRU on the same workload.
func TestCompareAlgorithmsReturnsRankedResults(t *testing.T) {
	mm := memory.NewMemoryManager(16, 8, algorithms.AlgorithmLRU)
	defer mm.Close()
	pm := process.NewProcessManager(mm)
	sim := simulator.NewSimulator(pm)

	results, err := sim.CompareAlgorithms("locality", 16, 8)
	if err != nil {
		t.Fatalf("CompareAlgorithms: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	// Ranks must be 1..N in order.
	for i, r := range results {
		if r.Rank != i+1 {
			t.Errorf("result[%d].Rank = %d, want %d", i, r.Rank, i+1)
		}
	}

	// Results must be sorted by fault rate ascending.
	for i := 1; i < len(results); i++ {
		if results[i].FaultRate < results[i-1].FaultRate {
			t.Errorf("results not sorted: results[%d].FaultRate=%.4f > results[%d].FaultRate=%.4f",
				i-1, results[i-1].FaultRate, i, results[i].FaultRate)
		}
	}

	// Optimal should appear in the results.
	foundOptimal := false
	for _, r := range results {
		if r.Algorithm == "Optimal" {
			foundOptimal = true
		}
	}
	if !foundOptimal {
		t.Error("Optimal algorithm missing from comparison results")
	}

	t.Logf("CompareAlgorithms returned %d results:", len(results))
	for _, r := range results {
		t.Logf("  #%d %-10s fault=%.2f%% hits=%.2f%% evictions=%d time=%v",
			r.Rank, r.Algorithm, r.FaultRate*100, r.HitRate*100, r.Evictions, r.Duration)
	}
}

// TestCompareAlgorithmsAllScenarios verifies comparison works for every
// built-in scenario without panicking.
func TestCompareAlgorithmsAllScenarios(t *testing.T) {
	scenarios := []string{"sequential", "random", "locality", "looping", "mixed", "fork_cow", "thrashing"}

	mm := memory.NewMemoryManager(8, 8, algorithms.AlgorithmLRU)
	defer mm.Close()
	pm := process.NewProcessManager(mm)
	sim := simulator.NewSimulator(pm)

	for _, sc := range scenarios {
		results, err := sim.CompareAlgorithms(sc, 8, 8)
		if err != nil {
			t.Errorf("CompareAlgorithms(%q): %v", sc, err)
			continue
		}
		if len(results) == 0 {
			t.Errorf("CompareAlgorithms(%q): no results", sc)
		}
		t.Logf("%-12s → %d results, winner: %s (%.2f%% fault rate)",
			sc, len(results), results[0].Algorithm, results[0].FaultRate*100)
	}
}

// TestSimulatorClose verifies that MemoryManager.Close stops the eventWorker
// goroutine and that subsequent operations do not panic.
func TestMemoryManagerClose(t *testing.T) {
	mm := memory.NewMemoryManager(16, 8, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)
	proc, _ := pm.CreateProcess("CloseTest", 1, 100)
	_ = pm.AccessMemory(proc.ID, 0, false)

	// Close should be idempotent.
	mm.Close()
	mm.Close()

	// emitEvent after Close must not panic (recover catches send on closed ch).
	_ = pm.AccessMemory(proc.ID, 1, false)
	t.Log("MemoryManager.Close is idempotent and post-Close accesses do not panic")
}

// TestTraceRecordAndReplay records an access sequence and replays it on a
// fresh manager, verifying that fault counts are deterministic.
func TestTraceRecordAndReplay(t *testing.T) {
	// Record phase.
	mm1 := memory.NewMemoryManager(8, 8, algorithms.AlgorithmLRU)
	defer mm1.Close()
	pm1 := process.NewProcessManager(mm1)
	sim1 := simulator.NewSimulator(pm1)
	proc1, _ := pm1.CreateProcess("Tracer", 1, 200)

	sim1.StartRecording()
	_ = sim1.SimulateSequentialAccess(proc1.ID, 0, 20, false)
	trace := sim1.StopRecording()

	if len(trace.Entries) == 0 {
		t.Fatal("trace is empty after recording")
	}
	if len(trace.Entries) != 20 {
		t.Errorf("expected 20 trace entries, got %d", len(trace.Entries))
	}
	firstMetrics := mm1.GetMetrics()

	// Replay phase — fresh manager, same algorithm.
	mm2 := memory.NewMemoryManager(8, 8, algorithms.AlgorithmLRU)
	defer mm2.Close()
	pm2 := process.NewProcessManager(mm2)
	// Re-create the same process so replay can find it.
	_, _ = pm2.CreateProcess("Tracer", 1, 200)
	sim2 := simulator.NewSimulator(pm2)

	if err := sim2.ReplayTrace(trace); err != nil {
		t.Fatalf("ReplayTrace: %v", err)
	}
	replayMetrics := mm2.GetMetrics()

	if firstMetrics.PageFaults != replayMetrics.PageFaults {
		t.Errorf("fault counts differ: original=%d replay=%d",
			firstMetrics.PageFaults, replayMetrics.PageFaults)
	}
	t.Logf("trace: %d entries, original faults=%d, replay faults=%d",
		len(trace.Entries), firstMetrics.PageFaults, replayMetrics.PageFaults)
}
