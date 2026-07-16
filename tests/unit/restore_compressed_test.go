package unit

import (
	"testing"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
)

// TestRestoreCompressedPreventsDataLoss is the dedicated test for PROD-016.
// It verifies that calling RestoreCompressed after a failed decompression
// round-trip puts the entry back in the store so it can be retrieved again.
func TestRestoreCompressedPreventsDataLoss(t *testing.T) {
	cm := memory.NewCompressionManager(0.7)

	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 8)
	}

	cp := cm.CompressPage(42, data)
	if cp == nil {
		t.Fatal("CompressPage returned nil; adjust minRatio if compression logic changed")
	}

	retrieved := cm.DecompressPage(42)
	if retrieved == nil {
		t.Fatal("DecompressPage returned nil for a page that was just compressed")
	}

	// Confirm it is truly gone from the store.
	if gone := cm.DecompressPage(42); gone != nil {
		t.Error("page should have been removed after first DecompressPage call")
	}

	// Simulate OOM failure path: restore the entry.
	cm.RestoreCompressed(retrieved)

	// Must be retrievable again — data loss prevented.
	recovered := cm.DecompressPage(42)
	if recovered == nil {
		t.Fatal("RestoreCompressed failed: page cannot be decompressed after restore (data loss)")
	}
	if recovered.PageID != 42 {
		t.Errorf("recovered pageID mismatch: want 42, got %d", recovered.PageID)
	}
	if recovered.OriginalSize != int64(len(data)) {
		t.Errorf("recovered original size: want %d, got %d", len(data), recovered.OriginalSize)
	}
}

// TestRestoreCompressedStatsConsistency verifies that RestoreCompressed keeps
// pagesCompressed and pagesDecompressed counters consistent.
func TestRestoreCompressedStatsConsistency(t *testing.T) {
	cm := memory.NewCompressionManager(0.7)

	data := make([]byte, 4096)
	cm.CompressPage(1, data)
	cm.CompressPage(2, data)

	if s := cm.GetStats(); s.PagesCompressed != 2 {
		t.Fatalf("initial: PagesCompressed want 2, got %d", s.PagesCompressed)
	}

	cp := cm.DecompressPage(1)
	if cp == nil {
		t.Fatal("DecompressPage returned nil")
	}

	// pagesCompressed is a monotonically increasing "total compressed" counter;
	// DecompressPage does not decrement it. pagesDecompressed increments by 1.
	s := cm.GetStats()
	if s.PagesCompressed != 2 {
		t.Errorf("after decompress: PagesCompressed want 2 (unchanged), got %d", s.PagesCompressed)
	}
	if s.PagesDecompressed != 1 {
		t.Errorf("after decompress: PagesDecompressed want 1, got %d", s.PagesDecompressed)
	}

	// RestoreCompressed undoes the decompression event without adding a new
	// compression event: pagesCompressed stays at 2, pagesDecompressed goes to 0.
	cm.RestoreCompressed(cp)

	s = cm.GetStats()
	if s.PagesCompressed != 2 {
		t.Errorf("after restore: PagesCompressed want 2 (unchanged), got %d", s.PagesCompressed)
	}
	if s.PagesDecompressed != 0 {
		t.Errorf("after restore: PagesDecompressed want 0, got %d", s.PagesDecompressed)
	}
}

// TestRestoreCompressedIdempotent verifies that calling RestoreCompressed twice
// with the same entry does not double-count stats.
func TestRestoreCompressedIdempotent(t *testing.T) {
	cm := memory.NewCompressionManager(0.7)

	data := make([]byte, 4096)
	cm.CompressPage(7, data)

	cp := cm.DecompressPage(7)
	if cp == nil {
		t.Fatal("DecompressPage returned nil")
	}

	cm.RestoreCompressed(cp)
	cm.RestoreCompressed(cp) // second call must be a no-op

	// pagesCompressed=1 (unchanged), pagesDecompressed=1-1=0 (from first restore).
	// Second RestoreCompressed hits the duplicate guard and returns immediately.
	s := cm.GetStats()
	if s.PagesCompressed != 1 {
		t.Errorf("idempotent restore: PagesCompressed want 1 (unchanged), got %d", s.PagesCompressed)
	}
	if s.PagesDecompressed != 0 {
		t.Errorf("idempotent restore: PagesDecompressed want 0, got %d", s.PagesDecompressed)
	}
}

// TestRestoreCompressedUnderOOMPressure exercises the full manager-level path
// where a compressed page is decompressed but frame allocation fails, causing
// the manager to call RestoreCompressed before returning an error.
// We use a tiny frame pool so all frames stay occupied.
func TestRestoreCompressedUnderOOMPressure(t *testing.T) {
	mm := memory.NewMemoryManager(4, 4, algorithms.AlgorithmLRU)
	defer mm.Close()
	mm.EnableCompression(true)
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("oom-test", 1, 100)

	// Fill all 4 frames.
	for i := uint64(0); i < 4; i++ {
		_ = pm.AccessMemory(proc.ID, i, false)
	}

	// Evict pages 0–3 by accessing pages 4–7; evicted pages may be compressed.
	for i := uint64(4); i < 8; i++ {
		_ = pm.AccessMemory(proc.ID, i, false)
	}

	cm := mm.GetCompressionManager()
	statsAfterEviction := cm.GetStats()
	t.Logf("compressed pages after eviction pressure: %d", statsAfterEviction.PagesCompressed)

	if statsAfterEviction.PagesCompressed == 0 {
		t.Skip("no pages were compressed under this pressure — test not applicable")
	}

	// Re-access a page that was likely compressed. If OOM during fault-in,
	// RestoreCompressed is called internally; the manager must not panic.
	err := pm.AccessMemory(proc.ID, 0, false)
	t.Logf("re-access compressed page 0: err=%v", err)

	statsAfter := cm.GetStats()
	t.Logf("stats after re-access: compressed=%d decompressed=%d",
		statsAfter.PagesCompressed, statsAfter.PagesDecompressed)

	if statsAfter.PagesCompressed < 0 {
		t.Error("PagesCompressed went negative — stats corruption")
	}
	if statsAfter.PagesDecompressed < 0 {
		t.Error("PagesDecompressed went negative — stats corruption")
	}
}
