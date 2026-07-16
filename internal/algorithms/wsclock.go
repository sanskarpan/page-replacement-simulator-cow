package algorithms

import (
	"fmt"
	"sync"
	"time"

	"github.com/page-replacement-cow/pkg/models"
)

type WSClock struct {
	name          string
	hand          int32
	workingSetAge time.Duration
	mu            sync.RWMutex
}

func NewWSClock(workingSetWindowMs int64) *WSClock {
	return &WSClock{
		name:          "WSClock",
		hand:          0,
		workingSetAge: time.Duration(workingSetWindowMs) * time.Millisecond,
	}
}

func (w *WSClock) SetTime(t time.Time) {
	// no-op: SelectVictim uses time.Now() for accurate age computation
}

func (w *WSClock) SelectVictim(frames []*models.Frame) (*models.Frame, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames available for eviction")
	}

	numFrames := int32(len(frames))
	iterations := int32(0)
	maxIterations := numFrames * 2

	for iterations < maxIterations {
		if w.hand >= numFrames {
			w.hand = 0
		}
		idx := w.hand
		w.hand = (w.hand + 1) % numFrames
		iterations++

		frame := frames[idx]
		if frame.IsFree() || frame.IsPinned() {
			continue
		}

		age := time.Now().Sub(frame.GetLastAccessTime())

		if frame.GetReferenceBit() {
			frame.ClearReferenceBit()
			continue
		}

		if age > w.workingSetAge {
			if frame.IsModified() {
				// Simulator-only simplification: clear the dirty bit inline
				// without performing actual I/O. A real OS kernel would queue
				// an async writeback here and defer eviction until it completes;
				// we skip that to keep the simulator self-contained.
				frame.Modified.Store(false)
				continue
			}
			return frame, nil
		}
	}

	for i := int32(0); i < numFrames; i++ {
		frame := frames[i]
		if !frame.IsFree() && !frame.IsPinned() {
			age := time.Now().Sub(frame.GetLastAccessTime())
			if age > w.workingSetAge && !frame.IsModified() {
				w.hand = (i + 1) % numFrames
				return frame, nil
			}
		}
	}

	for i := int32(0); i < numFrames; i++ {
		frame := frames[i]
		if !frame.IsFree() && !frame.IsPinned() {
			w.hand = (i + 1) % numFrames
			return frame, nil
		}
	}

	return nil, fmt.Errorf("no evictable frame found")
}


func (w *WSClock) OnPageAccess(frame *models.Frame, write bool) {
	frame.Access(write)
}

func (w *WSClock) OnPageFault(frame *models.Frame) {
	if frame != nil {
		frame.Access(false)
	}
}

func (w *WSClock) OnPageEviction(frame *models.Frame) {
	if frame != nil {
		frame.ClearReferenceBit()
	}
}

func (w *WSClock) GetName() string { return w.name }

func (w *WSClock) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.hand = 0
}

func (w *WSClock) GetWorkingSetAge() time.Duration {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.workingSetAge
}