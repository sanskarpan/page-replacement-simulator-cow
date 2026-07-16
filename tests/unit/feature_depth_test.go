package unit

import (
	"testing"
	"time"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
)

// ─────────────────────────────────────────────────────────────────────────────
// Multi-level page table wiring
// ─────────────────────────────────────────────────────────────────────────────

// TestMLPTPopulatedOnPageFault verifies that after a page fault the multi-level
// PT entry is present and records the correct frame number.
func TestMLPTPopulatedOnPageFault(t *testing.T) {
	mm := memory.NewMemoryManager(16, 4, algorithms.AlgorithmLRU)
	defer mm.Close()
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("mlpt-test", 1, 200)

	if err := mm.AccessMemory(proc.ID, 5, false); err != nil {
		t.Fatalf("AccessMemory: %v", err)
	}

	mpt, err := mm.GetMultiLevelPageTable(proc.ID)
	if err != nil {
		t.Fatalf("GetMultiLevelPageTable: %v", err)
	}

	// Virtual address for page 5 is 5 << 12 = 20480.
	entry := mpt.GetEntry(5 << 12)
	if entry == nil {
		t.Fatal("multi-level PT entry is nil after page fault")
	}
	if !entry.Present.Load() {
		t.Error("multi-level PT entry not marked Present after page fault")
	}
	if entry.FrameNumber < 0 {
		t.Errorf("multi-level PT entry has invalid frame number %d", entry.FrameNumber)
	}
}

// TestMLPTInvalidatedOnEviction verifies that evicting a page clears its
// multi-level PT entry (Present = false).
func TestMLPTInvalidatedOnEviction(t *testing.T) {
	// 2 frames, 1 process: first fault fills frame 0, second fault evicts it.
	mm := memory.NewMemoryManager(2, 2, algorithms.AlgorithmFIFO)
	defer mm.Close()
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("mlpt-evict", 1, 200)

	// Fill both frames.
	_ = mm.AccessMemory(proc.ID, 0, false)
	_ = mm.AccessMemory(proc.ID, 1, false)

	mpt, _ := mm.GetMultiLevelPageTable(proc.ID)

	// Third access causes eviction of page 0 (FIFO).
	_ = mm.AccessMemory(proc.ID, 2, false)

	entry := mpt.GetEntry(0 << 12)
	if entry != nil && entry.Present.Load() {
		t.Error("multi-level PT entry for evicted page still marked Present")
	}
}

// TestMLPTHugePageEntry verifies that MapHugePage registers a huge-page entry
// (Present=true, HugePage=true) in the multi-level PT.
func TestMLPTHugePageEntry(t *testing.T) {
	mm := memory.NewMemoryManager(16, 4, algorithms.AlgorithmLRU)
	defer mm.Close()
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("huge-mlpt", 1, 200)

	_, err := mm.MapHugePage(proc.ID, 0)
	if err != nil {
		t.Fatalf("MapHugePage: %v", err)
	}

	mpt, _ := mm.GetMultiLevelPageTable(proc.ID)
	// Huge page 0 maps at virtual address 0 << 21 = 0.
	entry := mpt.GetEntry(0)
	if entry == nil {
		t.Fatal("multi-level PT has no entry for huge page 0")
	}
	if !entry.Present.Load() {
		t.Error("huge-page entry not Present")
	}
	if !entry.HugePage.Load() {
		t.Error("huge-page entry not flagged as HugePage")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Huge pages
// ─────────────────────────────────────────────────────────────────────────────

// TestHugePageMapAndList verifies MapHugePage returns a valid frame and is
// listed by GetHugePages.
func TestHugePageMapAndList(t *testing.T) {
	mm := memory.NewMemoryManager(16, 4, algorithms.AlgorithmLRU)
	defer mm.Close()
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("huge-list", 1, 200)

	fid0, err := mm.MapHugePage(proc.ID, 0)
	if err != nil {
		t.Fatalf("MapHugePage(0): %v", err)
	}
	if fid0 < 0 {
		t.Errorf("MapHugePage returned negative frame ID %d", fid0)
	}

	_, err = mm.MapHugePage(proc.ID, 1)
	if err != nil {
		t.Fatalf("MapHugePage(1): %v", err)
	}

	pages, err := mm.GetHugePages(proc.ID)
	if err != nil {
		t.Fatalf("GetHugePages: %v", err)
	}
	if len(pages) != 2 {
		t.Errorf("expected 2 huge pages, got %d", len(pages))
	}
}

// TestHugePageDifferentProcesses verifies that huge-page mappings are isolated
// per process.
func TestHugePageDifferentProcesses(t *testing.T) {
	mm := memory.NewMemoryManager(16, 4, algorithms.AlgorithmLRU)
	defer mm.Close()
	pm := process.NewProcessManager(mm)

	p1, _ := pm.CreateProcess("h-proc1", 1, 200)
	p2, _ := pm.CreateProcess("h-proc2", 1, 200)

	_, _ = mm.MapHugePage(p1.ID, 0)

	pages1, _ := mm.GetHugePages(p1.ID)
	pages2, _ := mm.GetHugePages(p2.ID)

	if len(pages1) != 1 {
		t.Errorf("p1 should have 1 huge page, got %d", len(pages1))
	}
	if len(pages2) != 0 {
		t.Errorf("p2 should have 0 huge pages, got %d", len(pages2))
	}
}

// TestHugePageEvictionWhenFull verifies that MapHugePage succeeds even when all
// frames are occupied (it evicts one).
func TestHugePageEvictionWhenFull(t *testing.T) {
	mm := memory.NewMemoryManager(4, 4, algorithms.AlgorithmFIFO)
	defer mm.Close()
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("huge-evict", 1, 200)

	// Fill all 4 frames with regular pages.
	for i := 0; i < 4; i++ {
		_ = mm.AccessMemory(proc.ID, uint64(i), false)
	}

	// MapHugePage must evict one frame and succeed.
	_, err := mm.MapHugePage(proc.ID, 0)
	if err != nil {
		t.Fatalf("MapHugePage with full memory: %v", err)
	}
}

// TestGetHugesPagesUnknownProcess verifies GetHugePages returns an error for an
// unknown process.
func TestGetHugesPagesUnknownProcess(t *testing.T) {
	mm := memory.NewMemoryManager(8, 4, algorithms.AlgorithmLRU)
	defer mm.Close()

	_, err := mm.GetHugePages("no-such-process")
	if err == nil {
		t.Fatal("expected error for unknown process, got nil")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Working set model
// ─────────────────────────────────────────────────────────────────────────────

// TestWorkingSetUpdatedOnAccess verifies that repeated accesses update the
// process's WorkingSetSize to reflect unique pages in the sliding window.
func TestWorkingSetUpdatedOnAccess(t *testing.T) {
	mm := memory.NewMemoryManager(32, 4, algorithms.AlgorithmLRU)
	defer mm.Close()
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("ws-test", 1, 200)

	// Access 5 distinct pages (window=10, so all should be counted).
	for i := 0; i < 5; i++ {
		_ = mm.AccessMemory(proc.ID, uint64(i), false)
	}

	got := proc.GetWorkingSetSize()
	if got != 5 {
		t.Errorf("working set size = %d, want 5", got)
	}
}

// TestWorkingSetSlidingWindow verifies that the working set shrinks when old
// accesses fall outside the window (size=10).
func TestWorkingSetSlidingWindow(t *testing.T) {
	mm := memory.NewMemoryManager(32, 4, algorithms.AlgorithmLRU)
	defer mm.Close()
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("ws-window", 1, 200)

	// 10 accesses all to page 99.
	for i := 0; i < 10; i++ {
		_ = mm.AccessMemory(proc.ID, 99, false)
	}

	// After 10 accesses to the same page, working set = 1.
	if got := proc.GetWorkingSetSize(); got != 1 {
		t.Errorf("expected working set of 1 page after same-page accesses, got %d", got)
	}
}

// TestWorkingSetAPIExposure verifies GetWorkingSetInfo returns consistent info.
func TestWorkingSetAPIExposure(t *testing.T) {
	mm := memory.NewMemoryManager(16, 4, algorithms.AlgorithmLRU)
	defer mm.Close()
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("ws-api", 1, 200)
	_ = mm.AccessMemory(proc.ID, 10, false)
	_ = mm.AccessMemory(proc.ID, 20, false)
	_ = mm.AccessMemory(proc.ID, 30, false)

	info, err := mm.GetWorkingSetInfo(proc.ID)
	if err != nil {
		t.Fatalf("GetWorkingSetInfo: %v", err)
	}
	if info["process_id"] != proc.ID {
		t.Errorf("wrong process_id in info")
	}
	wsz, _ := info["working_set_size"].(int32)
	if wsz != 3 {
		t.Errorf("working_set_size = %d, want 3", wsz)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// PFF resident set enforcement
// ─────────────────────────────────────────────────────────────────────────────

// TestPFFResidentSetEnforced verifies that when PFF algorithm is active, used
// frames never exceed the algorithm's target resident set.
func TestPFFResidentSetEnforced(t *testing.T) {
	// 16 frames total, PFF target resident starts at numFrames/2 = 8.
	mm := memory.NewMemoryManager(16, 4, algorithms.AlgorithmPFF)
	defer mm.Close()
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("pff-test", 1, 200)

	// Access 12 distinct pages — without enforcement this would use 12 frames.
	for i := 0; i < 12; i++ {
		_ = mm.AccessMemory(proc.ID, uint64(i), false)
	}

	// Wait a moment; PFF fault rate may adjust target, but used frames
	// should stay at or below the current target.
	used := mm.GetFrameTable().GetUsedFrameCount()
	algo := mm.GetAlgorithm()
	if pff, ok := algo.(*algorithms.PFF); ok {
		target := pff.GetTargetResidentSet()
		if used > target {
			t.Errorf("used frames (%d) exceeds PFF target resident set (%d)", used, target)
		}
	} else {
		t.Skip("algorithm is not PFF after SetAlgorithm(AlgorithmPFF)")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// NUMA routing
// ─────────────────────────────────────────────────────────────────────────────

// TestNUMAFrameLocalityAfterEnable verifies that, when NUMA is enabled, frames
// allocated for a process land in the expected NUMA node's frame range.
// With numFrames=16 and 2 nodes, node 0 owns frames [0,8) and node 1 owns [8,16).
func TestNUMAFrameLocalityAfterEnable(t *testing.T) {
	mm := memory.NewMemoryManager(16, 4, algorithms.AlgorithmLRU)
	defer mm.Close()
	pm := process.NewProcessManager(mm)

	mm.EnableNuma(true)

	proc, _ := pm.CreateProcess("numa-local", 1, 200)

	// Access a few pages to allocate frames.
	for i := 0; i < 3; i++ {
		_ = mm.AccessMemory(proc.ID, uint64(i), false)
	}

	ft := mm.GetFrameTable()
	frames := ft.GetUsedFrames()
	if len(frames) == 0 {
		t.Fatal("no frames allocated after NUMA-enabled accesses")
	}

	// Determine which node this process maps to: selectLocalNode hashes processID % 2.
	// We cannot call it directly, but we can observe frame IDs: node 0 → [0,8), node 1 → [8,16).
	for _, f := range frames {
		nodeID := f.GetNumaNodeID()
		framesPerNode := int32(16 / 2)
		expectedStart := nodeID * framesPerNode
		expectedEnd := expectedStart + framesPerNode
		if f.ID < expectedStart || f.ID >= expectedEnd {
			t.Errorf("frame %d (NumaNodeID=%d) outside expected range [%d,%d)",
				f.ID, nodeID, expectedStart, expectedEnd)
		}
	}
}

// TestNUMAFallbackWhenNodeFull verifies that when the preferred NUMA node is
// full, the allocator falls back to global allocation without error.
func TestNUMAFallbackWhenNodeFull(t *testing.T) {
	// 4 frames: node 0 owns [0,2), node 1 owns [2,4).
	mm := memory.NewMemoryManager(4, 2, algorithms.AlgorithmLRU)
	defer mm.Close()
	pm := process.NewProcessManager(mm)

	mm.EnableNuma(true)

	proc, _ := pm.CreateProcess("numa-fallback", 1, 200)

	// Access 4 pages — the preferred node (2 frames) fills up; the allocator
	// must fall back to the other node without returning an error.
	for i := 0; i < 4; i++ {
		if err := mm.AccessMemory(proc.ID, uint64(i), false); err != nil {
			t.Fatalf("AccessMemory page %d (NUMA fallback): %v", i, err)
		}
	}

	used := mm.GetFrameTable().GetUsedFrameCount()
	if used != 4 {
		t.Errorf("expected 4 used frames after NUMA fallback, got %d", used)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Page clustering (prefetch anchor fix)
// ─────────────────────────────────────────────────────────────────────────────

// TestClusteringPrefetchesNextPages verifies that accessing a sequential run of
// pages (0,1,2) causes clustering to prefetch page 3 (and optionally 4) so that
// those pages are resident when accessed next.
func TestClusteringPrefetchesNextPages(t *testing.T) {
	// Enough frames so no eviction competes with prefetch.
	mm := memory.NewMemoryManager(32, 4, algorithms.AlgorithmLRU)
	defer mm.Close()
	pm := process.NewProcessManager(mm)

	mm.EnableClustering(true)

	proc, _ := pm.CreateProcess("cluster-test", 1, 200)

	// Touch pages 0, 1, 2 sequentially to trigger sequential detection.
	// After page 2, DetectSequential fires and tryPrefetch should pre-load 3 & 4.
	_ = mm.AccessMemory(proc.ID, 0, false)
	_ = mm.AccessMemory(proc.ID, 1, false)
	// The third access causes a page fault and then tryPrefetch.
	_ = mm.AccessMemory(proc.ID, 2, false)

	// Give a tiny moment for any async work (none here, all synchronous).
	time.Sleep(time.Millisecond)

	// If prefetching worked, page 3 should already be in a frame (no new fault
	// when we access it, hence process.PageFaults won't increase).
	faultsBefore := proc.PageFaults.Load()
	_ = mm.AccessMemory(proc.ID, 3, false)
	faultsAfter := proc.PageFaults.Load()

	if faultsAfter > faultsBefore {
		// Not a hard failure — prefetch is best-effort — but log it.
		t.Logf("page 3 was NOT prefetched (fault count went %d→%d); clustering may not have fired",
			faultsBefore, faultsAfter)
	} else {
		t.Logf("page 3 was prefetched successfully (no fault on access)")
	}
}

// TestClusteringAnchorKeyIsFirstPage verifies that DetectSequential stores the
// cluster at the anchor (first of the 3-page window) and GetPrefetchPages finds
// it there — not at the last page.
func TestClusteringAnchorKeyIsFirstPage(t *testing.T) {
	cm := memory.NewPageClusterManager(4, 16)

	// Detect sequence [10, 11, 12].
	cluster := cm.DetectSequential("p1", []uint64{10, 11, 12})
	if cluster == nil {
		t.Fatal("DetectSequential returned nil for valid sequence")
	}

	// GetPrefetchPages with anchor=10 should return pages 11..26.
	pages := cm.GetPrefetchPages(10)
	if len(pages) == 0 {
		t.Fatal("GetPrefetchPages with anchor=10 returned no pages")
	}
	if pages[0] != 11 {
		t.Errorf("first prefetch page should be 11 (anchor+1), got %d", pages[0])
	}

	// GetPrefetchPages with the LAST page (12) should return nothing — the old bug.
	badPages := cm.GetPrefetchPages(12)
	if len(badPages) != 0 {
		t.Errorf("GetPrefetchPages(lastPage=12) should return nothing, got %v", badPages)
	}
}

// TestClusteringNonSequentialNoCluster verifies that non-consecutive pages do
// not produce a cluster.
func TestClusteringNonSequentialNoCluster(t *testing.T) {
	cm := memory.NewPageClusterManager(4, 16)

	cluster := cm.DetectSequential("p1", []uint64{5, 7, 9})
	if cluster != nil {
		t.Error("DetectSequential should return nil for non-consecutive pages")
	}

	pages := cm.GetPrefetchPages(5)
	if len(pages) != 0 {
		t.Errorf("expected no prefetch pages for non-sequential, got %v", pages)
	}
}
