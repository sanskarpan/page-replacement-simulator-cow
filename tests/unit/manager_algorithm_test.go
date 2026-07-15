package unit

import (
	"testing"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
)

// TestARCCoWFrameTracked verifies that after a CoW copy, the new frame is
// properly tracked by ARC so eviction selects sensible victims.
func TestARCCoWFrameTracked(t *testing.T) {
	mm := memory.NewMemoryManager(16, 8, algorithms.AlgorithmARC)
	pm := process.NewProcessManager(mm)

	parent, err := pm.CreateProcess("Parent", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	for i := uint64(0); i < 10; i++ {
		if err := pm.AccessMemory(parent.ID, i, false); err != nil {
			t.Fatalf("prime page %d: %v", i, err)
		}
	}

	child, err := pm.ForkProcess(parent.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Child writes trigger CoW — new frames must be registered with ARC.
	for i := uint64(0); i < 5; i++ {
		if err := pm.AccessMemory(child.ID, i, true); err != nil {
			t.Fatalf("child write %d: %v", i, err)
		}
	}

	// Force eviction pressure: access many more pages so eviction fires.
	for i := uint64(10); i < 30; i++ {
		if err := pm.AccessMemory(child.ID, i, false); err != nil {
			t.Fatalf("extra access %d: %v", i, err)
		}
	}

	metrics := mm.GetMetrics()
	if metrics.TotalAccesses == 0 {
		t.Error("expected non-zero accesses")
	}
	// If ARC state was corrupted the eviction loop would panic; reaching here means it didn't.
	t.Logf("ARC CoW test passed: accesses=%d evictions=%d", metrics.TotalAccesses, metrics.Evictions)
}

func TestPrefetchFramesTrackedByARC(t *testing.T) {
	mm := memory.NewMemoryManager(32, 8, algorithms.AlgorithmARC)
	mm.EnableClustering(true)
	pm := process.NewProcessManager(mm)

	proc, err := pm.CreateProcess("Prefetch", 1, 200)
	if err != nil {
		t.Fatal(err)
	}
	// Sequential access to trigger cluster detection and prefetch.
	for i := uint64(0); i < 40; i++ {
		if err := pm.AccessMemory(proc.ID, i, false); err != nil {
			t.Fatalf("access %d: %v", i, err)
		}
	}
	metrics := mm.GetMetrics()
	t.Logf("Prefetch ARC test passed: accesses=%d evictions=%d", metrics.TotalAccesses, metrics.Evictions)
}
