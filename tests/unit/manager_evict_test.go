package unit

import (
	"testing"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
)

func TestPageFaultWithAllFramesPinnedReturnsError(t *testing.T) {
	// 4 frames, 1 process
	mm := memory.NewMemoryManager(4, 4, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	proc, err := pm.CreateProcess("Pinned", 1, 100)
	if err != nil {
		t.Fatal(err)
	}

	// Fill all 4 frames.
	for i := uint64(0); i < 4; i++ {
		if err := pm.AccessMemory(proc.ID, i, false); err != nil {
			t.Fatalf("fill frame %d: %v", i, err)
		}
	}

	// Pin all frames so eviction is impossible.
	ft := mm.GetFrameTable()
	for _, f := range ft.GetAllFrames() {
		f.Pin()
	}

	// Accessing a new page must fail — no frame can be allocated or evicted.
	err = pm.AccessMemory(proc.ID, 99, false)
	if err == nil {
		t.Error("expected error when all frames pinned, got nil")
	} else {
		t.Logf("correctly returned error: %v", err)
	}
}
