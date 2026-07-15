package unit

import (
	"testing"
	"time"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/pkg/models"
)

func TestWSClockDirtyFrameWrittenBack(t *testing.T) {
	wsc := algorithms.NewWSClock(100) // 100ms working-set window

	// Single dirty frame so the algorithm is forced to return it.
	frame := models.NewFrame(0)
	frame.Allocate(0, "p")
	frame.Access(true) // sets Modified=true, ReferenceBit=1

	time.Sleep(200 * time.Millisecond)
	wsc.SetTime(time.Now())

	// First call: reference bit is set, so it's cleared and we continue past it.
	// Second call: reference bit is cleared, dirty — with the bug, Modified stays true.
	// We call twice because the first SelectVictim clears the reference bit only.
	_, _ = wsc.SelectVictim([]*models.Frame{frame})

	victim, err := wsc.SelectVictim([]*models.Frame{frame})
	if err != nil {
		t.Fatalf("SelectVictim: %v", err)
	}
	if victim == nil {
		t.Fatal("expected a victim, got nil")
	}

	// After selection from the dirty-frame path, Modified must be cleared
	// (simulated writeback complete).
	if victim.IsModified() {
		t.Error("expected dirty frame to be marked clean after writeback, Modified is still true (BUG-11)")
	}
	t.Logf("WSClock dirty test: victim frame %d, modified=%v", victim.ID, victim.IsModified())
}
