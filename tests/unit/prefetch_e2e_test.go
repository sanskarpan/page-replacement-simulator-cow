package unit

import (
	"testing"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
)

// TestPrefetchReducesFaultsOnSequentialAccess verifies the complete prefetch
// pipeline: sequential accesses → DetectSequential → GetPrefetchPages → frames
// loaded ahead-of-time → later accesses hit without faulting.
func TestPrefetchReducesFaultsOnSequentialAccess(t *testing.T) {
	mm := memory.NewMemoryManager(64, 16, algorithms.AlgorithmLRU)
	defer mm.Close()
	mm.EnableClustering(true)
	pm := process.NewProcessManager(mm)

	proc, err := pm.CreateProcess("prefetch-test", 1, 200)
	if err != nil {
		t.Fatalf("CreateProcess: %v", err)
	}

	// Access pages 0, 1, 2 sequentially — this builds the cluster (anchor=0).
	for i := uint64(0); i < 3; i++ {
		if err := pm.AccessMemory(proc.ID, i, false); err != nil {
			t.Fatalf("access page %d: %v", i, err)
		}
	}

	faultsBefore := mm.GetMetrics().PageFaults

	// Pages 3–5 may have been prefetched by tryPrefetch after accesses 0→1→2.
	// Access them now: if prefetched, they register as TLB hits (no fault).
	for i := uint64(3); i < 6; i++ {
		if err := pm.AccessMemory(proc.ID, i, false); err != nil {
			t.Fatalf("access page %d: %v", i, err)
		}
	}

	faultsAfter := mm.GetMetrics().PageFaults
	newFaults := faultsAfter - faultsBefore

	// With prefetch enabled and 64 frames, at least some pages 3–5 should be
	// pre-loaded. We require strictly fewer than 3 new faults (at least 1 hit).
	if newFaults >= 3 {
		t.Errorf("expected prefetch to eliminate some faults: got %d new faults for 3 accesses (all missed)", newFaults)
	}
	t.Logf("prefetch test: faults before=%d, faults after=%d, new faults=%d",
		faultsBefore, faultsAfter, newFaults)
}

// TestPrefetchCrossProcessIsolation verifies that a cluster registered for
// processA does not produce prefetch pages when looked up for processB.
func TestPrefetchCrossProcessIsolation(t *testing.T) {
	mm := memory.NewMemoryManager(64, 16, algorithms.AlgorithmLRU)
	defer mm.Close()
	mm.EnableClustering(true)
	pm := process.NewProcessManager(mm)

	procA, _ := pm.CreateProcess("A", 1, 200)
	procB, _ := pm.CreateProcess("B", 1, 200)

	// procA builds sequential cluster at anchor=10.
	for _, p := range []uint64{10, 11, 12} {
		if err := pm.AccessMemory(procA.ID, p, false); err != nil {
			t.Fatalf("procA access %d: %v", p, err)
		}
	}

	// procB accesses the same anchor pages — its own recentAccesses is separate,
	// but the prefetch call for B looks up clusterKey("B", 10), not ("A", 10).
	faultsB := mm.GetMetrics().PageFaults
	for _, p := range []uint64{10, 11, 12} {
		if err := pm.AccessMemory(procB.ID, p, false); err != nil {
			t.Fatalf("procB access %d: %v", p, err)
		}
	}
	newFaultsB := mm.GetMetrics().PageFaults - faultsB

	// procB should fault on all 3 of its first accesses — no cross-process spill.
	if newFaultsB != 3 {
		t.Errorf("cross-process isolation broken: procB had %d faults for first 3 accesses, expected 3",
			newFaultsB)
	}
	t.Logf("cross-process isolation OK: procB faults=%d", newFaultsB)
}

// TestPrefetchDirectClusterManager exercises GetPrefetchPages with the correct
// processID key and verifies no results for a different processID at the same
// anchor — the core PROD-043 fix.
func TestPrefetchDirectClusterManager(t *testing.T) {
	pcm := memory.NewPageClusterManager(3, 8)

	// Register cluster for "procA" at anchor=100.
	c := pcm.DetectSequential("procA", []uint64{100, 101, 102})
	if c == nil {
		t.Fatal("DetectSequential returned nil for valid sequence")
	}

	// Correct processID: should return 8 pages starting at 101.
	pagesA := pcm.GetPrefetchPages("procA", 100)
	if len(pagesA) != 8 {
		t.Errorf("GetPrefetchPages(procA,100): expected 8, got %d", len(pagesA))
	}
	if pagesA[0] != 101 {
		t.Errorf("first prefetch page: expected 101, got %d", pagesA[0])
	}

	// Different processID at same anchor: must return nothing.
	pagesB := pcm.GetPrefetchPages("procB", 100)
	if len(pagesB) != 0 {
		t.Errorf("cross-process isolation: GetPrefetchPages(procB,100) returned %d pages, expected 0", len(pagesB))
	}

	// ClearClusters for procA: procA's entry gone, nothing for procB was ever added.
	pcm.ClearClusters("procA")
	pagesAAfter := pcm.GetPrefetchPages("procA", 100)
	if len(pagesAAfter) != 0 {
		t.Errorf("after ClearClusters(procA): expected 0 pages, got %d", len(pagesAAfter))
	}
}
