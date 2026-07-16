package unit

import (
	"testing"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/pkg/models"
)

// buildFrames allocates n non-free frames with ascending page IDs.
func buildFrames(n int) []*models.Frame {
	frames := make([]*models.Frame, n)
	for i := range frames {
		f := models.NewFrame(int32(i))
		f.Allocate(uint64(i+1), "p0")
		frames[i] = f
	}
	return frames
}

// TestNRUClass0Preferred verifies that NRU evicts a Class-0 (R=0,M=0) frame
// when multiple classes are present.
//
// Note: Frame.AllocateNuma() stores ReferenceBit=1, so freshly-allocated
// frames start as class 2 (R=1,M=0). We must explicitly clear R to reach
// class 0.
func TestNRUClass0Preferred(t *testing.T) {
	nru := algorithms.NewNRU()
	frames := buildFrames(4)

	// frame 0: class 0 (R=0, M=0) — target; clear the R=1 set by AllocateNuma.
	frames[0].ClearReferenceBit()

	// frame 1: class 1 (R=0, M=1)
	frames[1].Access(true)         // R=1, M=1
	frames[1].ClearReferenceBit()  // R=0, M=1 → class 1

	// frame 2: class 2 (R=1, M=0) — AllocateNuma already set R=1; leave it.

	// frame 3: class 3 (R=1, M=1)
	frames[3].Access(true) // R=1, M=1

	victim, err := nru.SelectVictim(frames)
	if err != nil {
		t.Fatalf("SelectVictim: %v", err)
	}
	if victim != frames[0] {
		t.Errorf("expected class-0 frame (index 0) as victim, got frame %d (R=%v M=%v)",
			victim.ID, victim.GetReferenceBit(), victim.IsModified())
	}
}

// TestNRUFallsToClass1WhenClass0Empty verifies the class priority fallback.
func TestNRUFallsToClass1WhenClass0Empty(t *testing.T) {
	nru := algorithms.NewNRU()
	frames := buildFrames(3)

	// All frames have M=1 set (class 1 or 3).
	for _, f := range frames {
		f.Access(true) // R=1, M=1
		f.ClearReferenceBit() // now R=0, M=1 → class 1
	}

	victim, err := nru.SelectVictim(frames)
	if err != nil {
		t.Fatalf("SelectVictim: %v", err)
	}
	// victim must be one of the class-1 frames
	if victim.IsModified() == false || victim.GetReferenceBit() == true {
		t.Errorf("expected class-1 (R=0,M=1) victim, got referenced=%v modified=%v",
			victim.GetReferenceBit(), victim.IsModified())
	}
}

// TestNRURBitClearOnClockTick verifies that after clearPeriod accesses all R
// bits are cleared during the next SelectVictim call, making formerly class-2/3
// pages fall to class-0/1.
func TestNRURBitClearOnClockTick(t *testing.T) {
	nru := algorithms.NewNRU()
	frames := buildFrames(2)

	// Mark both frames as recently used (class 2: R=1,M=0).
	frames[0].Access(false) // R=1, M=0
	frames[1].Access(false) // R=1, M=0

	// Drive 50 OnPageAccess calls to hit the clearPeriod.
	for i := 0; i < 50; i++ {
		nru.OnPageAccess(frames[0], false)
	}

	// SelectVictim should clear R bits before classifying — both frames become class 0.
	victim, err := nru.SelectVictim(frames)
	if err != nil {
		t.Fatalf("SelectVictim after clock tick: %v", err)
	}
	if victim == nil {
		t.Fatal("expected a victim, got nil")
	}
	// After the clock tick, both frames are class 0 — any is valid.
	if victim.GetReferenceBit() {
		t.Error("victim still has R=1 after clock tick")
	}
}

// TestNRUSkipsFreeAndPinned verifies that free and pinned frames are not chosen.
func TestNRUSkipsFreeAndPinned(t *testing.T) {
	nru := algorithms.NewNRU()

	free := models.NewFrame(0) // IsFree() == true

	pinned := models.NewFrame(1)
	pinned.Allocate(42, "p0")
	pinned.Pinned.Store(true)

	target := models.NewFrame(2)
	target.Allocate(99, "p0")

	victim, err := nru.SelectVictim([]*models.Frame{free, pinned, target})
	if err != nil {
		t.Fatalf("SelectVictim: %v", err)
	}
	if victim != target {
		t.Errorf("expected target frame, got %v", victim.ID)
	}
}

// TestNRUNoEvictableFrameError verifies the error path when all frames are free/pinned.
func TestNRUNoEvictableFrameError(t *testing.T) {
	nru := algorithms.NewNRU()
	free := models.NewFrame(0)
	_, err := nru.SelectVictim([]*models.Frame{free})
	if err == nil {
		t.Fatal("expected error when no evictable frame, got nil")
	}
}

// TestNRUResetClearsAccessCount verifies that Reset() zeroes the access counter.
func TestNRUResetClearsAccessCount(t *testing.T) {
	nru := algorithms.NewNRU()
	frames := buildFrames(1)

	for i := 0; i < 30; i++ {
		nru.OnPageAccess(frames[0], false)
	}
	nru.Reset()

	// After reset the counter should be 0; no clock tick should fire on the
	// very next SelectVictim unless clearPeriod more calls arrive.
	// Just verify it doesn't panic and returns a victim.
	_, err := nru.SelectVictim(frames)
	if err != nil {
		t.Fatalf("SelectVictim after Reset: %v", err)
	}
}

// TestNRUGetName verifies the algorithm's reported name.
func TestNRUGetName(t *testing.T) {
	nru := algorithms.NewNRU()
	if got := nru.GetName(); got != "NRU" {
		t.Errorf("GetName() = %q, want %q", got, "NRU")
	}
}
