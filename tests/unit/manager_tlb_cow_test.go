package unit

import (
	"testing"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
)

func TestTLBHitAfterCoWCopy(t *testing.T) {
	mm := memory.NewMemoryManager(64, 32, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	parent, _ := pm.CreateProcess("Parent", 1, 100)
	for i := uint64(0); i < 5; i++ {
		_ = pm.AccessMemory(parent.ID, i, false)
	}
	child, _ := pm.ForkProcess(parent.ID)

	// Child writes trigger CoW — handleCoW should prime the TLB with
	// the original virtual page ID (page.ID), not the synthetic CoW ID.
	for i := uint64(0); i < 5; i++ {
		_ = pm.AccessMemory(child.ID, i, true)
	}

	// Snapshot TLB stats after writes (all writes were misses since child's TLB was empty).
	tlbMid := mm.GetTLB().GetStats()

	// Re-read the same pages — CoW inserts should have primed TLB for page.ID keys.
	for i := uint64(0); i < 5; i++ {
		_ = pm.AccessMemory(child.ID, i, false)
	}

	tlbAfter := mm.GetTLB().GetStats()
	newHits := tlbAfter.Hits - tlbMid.Hits
	if newHits == 0 {
		t.Errorf("expected TLB hits on re-reads after CoW, got 0 new hits — "+
			"TLB was not primed with correct virtual page (BUG-12)")
	}
	t.Logf("TLB after CoW re-reads: new_hits=%d (total hits=%d misses=%d rate=%.2f%%)",
		newHits, tlbAfter.Hits, tlbAfter.Misses, tlbAfter.HitRate*100)
}
