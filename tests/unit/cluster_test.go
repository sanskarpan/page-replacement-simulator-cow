package unit

import (
	"testing"

	"github.com/page-replacement-cow/internal/memory"
)

func TestClearClustersOnlyAffectsTargetProcess(t *testing.T) {
	pcm := memory.NewPageClusterManager(3, 8)

	// Process A: pages 10,11,12
	pcm.DetectSequential("procA", []uint64{10, 11, 12})
	// Process B: pages 20,21,22
	pcm.DetectSequential("procB", []uint64{20, 21, 22})

	// Remove procA only.
	pcm.ClearClusters("procA")

	// procA's cluster should be gone.
	if pages := pcm.GetPrefetchPages(10); len(pages) > 0 {
		t.Errorf("expected procA cluster to be cleared, still got %d prefetch pages", len(pages))
	}

	// procB's cluster must survive.
	if pages := pcm.GetPrefetchPages(20); len(pages) == 0 {
		t.Error("procB cluster was wrongly cleared when procA was removed")
	}
}

func TestClearClustersEmptyStringClearsAll(t *testing.T) {
	pcm := memory.NewPageClusterManager(3, 8)
	pcm.DetectSequential("pA", []uint64{1, 2, 3})
	pcm.DetectSequential("pB", []uint64{5, 6, 7})

	pcm.ClearClusters("") // global reset

	if pages := pcm.GetPrefetchPages(1); len(pages) > 0 {
		t.Error("expected all clusters cleared on empty processID")
	}
	if pages := pcm.GetPrefetchPages(5); len(pages) > 0 {
		t.Error("expected all clusters cleared on empty processID")
	}
}
