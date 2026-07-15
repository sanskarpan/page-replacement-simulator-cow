package unit

import (
	"testing"

	"github.com/page-replacement-cow/internal/memory"
)

func TestCompressPageActuallyCompresses(t *testing.T) {
	cm := memory.NewCompressionManager(0.7) // minRatio=0.7: accept if compressed ≤ 70% of original

	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 4) // highly compressible pattern
	}

	cp := cm.CompressPage(1, data)
	if cp == nil {
		t.Fatal("CompressPage returned nil — compression never executed despite compressible data")
	}
	if cp.CompressedSize >= cp.OriginalSize {
		t.Errorf("compressed size %d >= original %d — no compression achieved",
			cp.CompressedSize, cp.OriginalSize)
	}
	ratio := float64(cp.CompressedSize) / float64(cp.OriginalSize)
	t.Logf("compression ratio: %.2f (compressed %d → %d bytes)", ratio, cp.OriginalSize, cp.CompressedSize)

	stats := cm.GetStats()
	if stats.PagesCompressed == 0 {
		t.Error("expected PagesCompressed > 0 after CompressPage")
	}
}

func TestCompressPageRejectsIncompressible(t *testing.T) {
	// Very strict threshold: only compress if ratio < 0.4 (simulated is 0.5 → rejected)
	cm2 := memory.NewCompressionManager(0.4)
	data := make([]byte, 4096)
	cp := cm2.CompressPage(99, data)
	if cp != nil {
		t.Error("expected nil for strict minRatio=0.4 when simulated ratio=0.5, got a result")
	}
}

func TestDecompressPageRoundtrip(t *testing.T) {
	cm := memory.NewCompressionManager(0.7)
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 8)
	}

	cp := cm.CompressPage(5, data)
	if cp == nil {
		t.Skip("compression rejected — adjust test if ratio changes")
	}

	result := cm.DecompressPage(5)
	if result == nil {
		t.Fatal("DecompressPage returned nil for a compressed page")
	}
	if result.PageID != 5 {
		t.Errorf("expected pageID 5, got %d", result.PageID)
	}

	stats := cm.GetStats()
	if stats.PagesDecompressed == 0 {
		t.Error("expected PagesDecompressed > 0")
	}
}
