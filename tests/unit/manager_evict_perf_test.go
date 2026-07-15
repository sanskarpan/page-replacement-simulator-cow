package unit

import (
	"testing"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
)

func TestEvictPageCorrectlyUnmapsPage(t *testing.T) {
	mm := memory.NewMemoryManager(4, 4, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("Evict", 1, 100)
	// Fill all 4 frames.
	for i := uint64(0); i < 4; i++ {
		_ = pm.AccessMemory(proc.ID, i, false)
	}
	// Access page 99 — forces eviction of one of the 4 pages.
	if err := pm.AccessMemory(proc.ID, 99, false); err != nil {
		t.Fatalf("access after eviction: %v", err)
	}

	// Exactly 4 frames should be used (eviction freed one, then new one loaded).
	ft := mm.GetFrameTable()
	used := ft.GetUsedFrameCount()
	if used != 4 {
		t.Errorf("expected 4 used frames after eviction, got %d", used)
	}
}

func BenchmarkEvictPage(b *testing.B) {
	mm := memory.NewMemoryManager(32, 8, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)
	proc, _ := pm.CreateProcess("B", 1, 10000)
	// Fill memory with many pages to make O(n) vs O(1) measurable.
	for i := uint64(0); i < 32; i++ {
		_ = pm.AccessMemory(proc.ID, i, false)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pm.AccessMemory(proc.ID, uint64(32+i%100), false)
	}
}
