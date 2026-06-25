package unit

import (
	"testing"
	"time"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/pkg/models"
)

// Helper function to create test frames
func createTestFrames(count int) []*models.Frame {
	frames := make([]*models.Frame, count)
	for i := 0; i < count; i++ {
		frames[i] = models.NewFrame(int32(i))
		frames[i].Allocate(uint64(i), "test-process")
	}
	return frames
}

// TestLRU tests the LRU algorithm
func TestLRU(t *testing.T) {
	lru := algorithms.NewLRU()

	frames := createTestFrames(5)

	// Access frames in order: 0, 1, 2, 3, 4
	for i := 0; i < 5; i++ {
		lru.OnPageAccess(frames[i], false)
	}

	// Frame 0 should be the victim (least recently used)
	victim, err := lru.SelectVictim(frames)
	if err != nil {
		t.Fatalf("SelectVictim failed: %v", err)
	}

	if victim.ID != 0 {
		t.Errorf("Expected victim frame 0, got %d", victim.ID)
	}

	t.Logf("LRU test passed - selected frame %d as victim", victim.ID)
}

// TestCLOCK tests the CLOCK algorithm
func TestCLOCK(t *testing.T) {
	clock := algorithms.NewCLOCK()

	frames := createTestFrames(5)

	// Set reference bits
	for i := 0; i < 5; i++ {
		clock.OnPageAccess(frames[i], false)
	}

	// Clear reference bit on frame 2
	frames[2].ClearReferenceBit()

	// Frame 2 should be selected (first with ref bit = 0)
	victim, err := clock.SelectVictim(frames)
	if err != nil {
		t.Fatalf("SelectVictim failed: %v", err)
	}

	if victim.ID != 2 {
		t.Errorf("Expected victim frame 2, got %d", victim.ID)
	}

	t.Logf("CLOCK test passed - selected frame %d as victim", victim.ID)
}

// TestLFU tests the LFU algorithm
func TestLFU(t *testing.T) {
	lfu := algorithms.NewLFU()

	frames := createTestFrames(5)

	// Access frames with different frequencies
	// Frame 0: 1 access
	// Frame 1: 3 accesses
	// Frame 2: 2 accesses
	// Frame 3: 5 accesses
	// Frame 4: 1 access

	lfu.OnPageAccess(frames[0], false)
	for i := 0; i < 3; i++ {
		lfu.OnPageAccess(frames[1], false)
	}
	for i := 0; i < 2; i++ {
		lfu.OnPageAccess(frames[2], false)
	}
	for i := 0; i < 5; i++ {
		lfu.OnPageAccess(frames[3], false)
	}
	lfu.OnPageAccess(frames[4], false)

	// Frame 0 or 4 should be selected (lowest frequency)
	victim, err := lfu.SelectVictim(frames)
	if err != nil {
		t.Fatalf("SelectVictim failed: %v", err)
	}

	if victim.ID != 0 && victim.ID != 4 {
		t.Errorf("Expected victim frame 0 or 4, got %d", victim.ID)
	}

	t.Logf("LFU test passed - selected frame %d as victim", victim.ID)
}

// TestFIFO tests the FIFO algorithm
func TestFIFO(t *testing.T) {
	fifo := algorithms.NewFIFO()

	frames := createTestFrames(5)

	// Frame 0 should be the oldest (loaded first)
	victim, err := fifo.SelectVictim(frames)
	if err != nil {
		t.Fatalf("SelectVictim failed: %v", err)
	}

	if victim.ID != 0 {
		t.Errorf("Expected victim frame 0, got %d", victim.ID)
	}

	t.Logf("FIFO test passed - selected frame %d as victim", victim.ID)
}

// TestAlgorithmGetName tests GetName method
func TestAlgorithmGetName(t *testing.T) {
	tests := []struct {
		algo algorithms.PageReplacementAlgorithm
		name string
	}{
		{algorithms.NewLRU(), "LRU"},
		{algorithms.NewCLOCK(), "CLOCK"},
		{algorithms.NewLFU(), "LFU"},
		{algorithms.NewFIFO(), "FIFO"},
		{algorithms.NewOptimal(), "Optimal"},
	}

	for _, test := range tests {
		if test.algo.GetName() != test.name {
			t.Errorf("Expected name %s, got %s", test.name, test.algo.GetName())
		}
	}

	t.Log("Algorithm GetName tests passed")
}

// TestAlgorithmReset tests Reset method
func TestAlgorithmReset(t *testing.T) {
	lru := algorithms.NewLRU()
	frames := createTestFrames(3)

	// Access some frames
	for _, frame := range frames {
		lru.OnPageAccess(frame, false)
	}

	// Reset
	lru.Reset()

	// Should still work after reset
	victim, err := lru.SelectVictim(frames)
	if err != nil {
		t.Fatalf("SelectVictim failed after reset: %v", err)
	}

	if victim == nil {
		t.Error("Expected a victim after reset")
	}

	t.Log("Algorithm reset test passed")
}

// TestEmptyFrameList tests behavior with empty frame list
func TestEmptyFrameList(t *testing.T) {
	lru := algorithms.NewLRU()

	_, err := lru.SelectVictim([]*models.Frame{})
	if err == nil {
		t.Error("Expected error with empty frame list")
	}

	t.Log("Empty frame list test passed")
}

// TestPinnedFrames tests that pinned frames are not selected
func TestPinnedFrames(t *testing.T) {
	lru := algorithms.NewLRU()

	frames := createTestFrames(3)

	// Pin first two frames
	frames[0].Pin()
	frames[1].Pin()

	// Frame 2 should be selected (only unpinned one)
	victim, err := lru.SelectVictim(frames)
	if err != nil {
		t.Fatalf("SelectVictim failed: %v", err)
	}

	if victim.ID != 2 {
		t.Errorf("Expected unpinned frame 2, got %d", victim.ID)
	}

	t.Log("Pinned frames test passed")
}

// TestRandom tests the Random algorithm
func TestRandom(t *testing.T) {
	random := algorithms.NewRandom()

	frames := createTestFrames(5)

	victim, err := random.SelectVictim(frames)
	if err != nil {
		t.Fatalf("SelectVictim failed: %v", err)
	}

	if victim == nil {
		t.Fatal("Expected a victim frame")
	}

	if victim.IsFree() || victim.IsPinned() {
		t.Error("Victim should not be free or pinned")
	}

	t.Logf("Random test passed - selected frame %d as victim", victim.ID)
}

// TestRandomAllPinned tests Random algorithm with all frames pinned
func TestRandomAllPinned(t *testing.T) {
	random := algorithms.NewRandom()

	frames := createTestFrames(3)
	for _, f := range frames {
		f.Pin()
	}

	_, err := random.SelectVictim(frames)
	if err == nil {
		t.Error("Expected error when all frames are pinned")
	}

	t.Log("Random all-pinned test passed")
}

// TestCLOCKAllRefBitsSet tests CLOCK with all reference bits initially set
func TestCLOCKAllRefBitsSet(t *testing.T) {
	clock := algorithms.NewCLOCK()

	frames := createTestFrames(5)

	for i := 0; i < 5; i++ {
		clock.OnPageAccess(frames[i], false)
	}

	victim, err := clock.SelectVictim(frames)
	if err != nil {
		t.Fatalf("SelectVictim failed: %v", err)
	}

	if victim.IsFree() || victim.IsPinned() {
		t.Error("Victim should not be free or pinned")
	}

	t.Logf("CLOCK all-ref-bits test passed - selected frame %d", victim.ID)
}

// TestFIFOWithAllSameLoadTime tests FIFO tie-breaking
func TestFIFOWithAllSameLoadTime(t *testing.T) {
	fifo := algorithms.NewFIFO()

	frames := createTestFrames(5)

	victim, err := fifo.SelectVictim(frames)
	if err != nil {
		t.Fatalf("SelectVictim failed: %v", err)
	}

	if victim == nil {
		t.Fatal("Expected a victim frame")
	}

	t.Logf("FIFO same load time test passed - selected frame %d", victim.ID)
}

// TestLFUTieBreaking tests LFU tie-breaking with LRU
func TestLFUTieBreaking(t *testing.T) {
	lfu := algorithms.NewLFU()

	frames := createTestFrames(3)

	lfu.OnPageAccess(frames[0], false)
	lfu.OnPageAccess(frames[1], false)
	lfu.OnPageAccess(frames[2], false)

	time.Sleep(1 * time.Millisecond)
	lfu.OnPageAccess(frames[0], false)
	time.Sleep(1 * time.Millisecond)
	lfu.OnPageAccess(frames[1], false)

	victim, err := lfu.SelectVictim(frames)
	if err != nil {
		t.Fatalf("SelectVictim failed: %v", err)
	}

	if victim.ID != 2 {
		t.Errorf("LFU tie-break: expected frame 2 (lowest freq then oldest), got %d", victim.ID)
	}

	t.Logf("LFU tie-breaking test passed - selected frame %d", victim.ID)
}

// TestAlgorithmGetNameWithRandom tests GetName for Random algorithm
func TestAlgorithmGetNameWithRandom(t *testing.T) {
	random := algorithms.NewRandom()
	if random.GetName() != "Random" {
		t.Errorf("Expected name 'Random', got '%s'", random.GetName())
	}
	t.Log("Random GetName test passed")
}
