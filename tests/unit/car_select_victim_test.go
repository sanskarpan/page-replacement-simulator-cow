package unit

import (
	"testing"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
)

// TestCARAllRefBitSetSelectsVictim verifies that CAR can still evict a frame
// when all T1 entries have their reference bit set (all get promoted to T2).
// Before the fix, the loop bound `len(c.t1)*2` was re-evaluated on every
// iteration so it shrank as entries moved, causing the loop to exit early
// without scanning the full initial T1 population.
func TestCARAllRefBitSetSelectsVictim(t *testing.T) {
	mm := memory.NewMemoryManager(4, 8, algorithms.AlgorithmCAR)
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("CAR", 1, 100)

	// Fill all 4 frames.
	for i := uint64(0); i < 4; i++ {
		if err := pm.AccessMemory(proc.ID, i, false); err != nil {
			t.Fatalf("fill frame %d: %v", i, err)
		}
	}

	// Re-access all 4 to set their reference bits in CAR.
	for i := uint64(0); i < 4; i++ {
		_ = pm.AccessMemory(proc.ID, i, false)
	}

	// Access a 5th page — must evict one of the 4 even though all had refBit set.
	// Before the fix this could fail or select a nil victim due to early loop exit.
	if err := pm.AccessMemory(proc.ID, 99, false); err != nil {
		t.Fatalf("expected eviction to succeed, got: %v", err)
	}

	used := mm.GetFrameTable().GetUsedFrameCount()
	if used != 4 {
		t.Errorf("expected 4 used frames after eviction, got %d", used)
	}
	t.Logf("CAR all-refBit test passed: used_frames=%d", used)
}

// TestCARScanCoversAllT1Entries verifies that after a run where T1 entries
// with refBit=true are promoted, a subsequent SelectVictim still finds a clean
// candidate (the early-exit bug would leave clean entries permanently skipped).
func TestCARScanCoversAllT1Entries(t *testing.T) {
	mm := memory.NewMemoryManager(6, 8, algorithms.AlgorithmCAR)
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("CARScan", 1, 100)

	// Fill 6 frames.
	for i := uint64(0); i < 6; i++ {
		_ = pm.AccessMemory(proc.ID, i, false)
	}

	// Re-access first 5 (set refBit); leave page 5 cold (no refBit).
	for i := uint64(0); i < 5; i++ {
		_ = pm.AccessMemory(proc.ID, i, false)
	}

	// Access pages 6 and 7 — forces two evictions. Both should succeed.
	for _, p := range []uint64{6, 7} {
		if err := pm.AccessMemory(proc.ID, p, false); err != nil {
			t.Fatalf("eviction for page %d: %v", p, err)
		}
	}

	metrics := mm.GetMetrics()
	if metrics.Evictions < 2 {
		t.Errorf("expected at least 2 evictions, got %d", metrics.Evictions)
	}
	t.Logf("CAR scan coverage test: evictions=%d", metrics.Evictions)
}
