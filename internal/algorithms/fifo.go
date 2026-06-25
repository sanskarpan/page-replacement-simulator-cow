package algorithms

import (
	"fmt"
	"sync"
	"time"

	"github.com/page-replacement-cow/pkg/models"
)

// FIFO implements the First-In-First-Out page replacement algorithm
type FIFO struct {
	name string
	mu   sync.RWMutex
}

// NewFIFO creates a new FIFO algorithm instance
func NewFIFO() *FIFO {
	return &FIFO{
		name: "FIFO",
	}
}

// SelectVictim selects the oldest page for eviction
func (fifo *FIFO) SelectVictim(frames []*models.Frame) (*models.Frame, error) {
	fifo.mu.RLock()
	defer fifo.mu.RUnlock()

	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames available for eviction")
	}

	var victim *models.Frame
	var oldestTime time.Time
	first := true

	for _, frame := range frames {
		// Skip free or pinned frames
		if frame.IsFree() || frame.IsPinned() {
			continue
		}

		loadedTime := frame.GetLoadedTime()
		if first || loadedTime.Before(oldestTime) {
			victim = frame
			oldestTime = loadedTime
			first = false
		}
	}

	if victim == nil {
		return nil, fmt.Errorf("no evictable frame found")
	}

	return victim, nil
}

// OnPageAccess is called when a page is accessed
func (fifo *FIFO) OnPageAccess(frame *models.Frame, write bool) {
	// FIFO doesn't care about accesses, only load time
	if write {
		frame.Access(write) // Still track for dirty bit
	}
}

// OnPageFault is called when a page fault occurs
func (fifo *FIFO) OnPageFault(frame *models.Frame) {
	// Frame tracks loaded time automatically
}

// OnPageEviction is called when a page is evicted
func (fifo *FIFO) OnPageEviction(frame *models.Frame) {
	// No special handling needed for FIFO
}

// GetName returns the name of the algorithm
func (fifo *FIFO) GetName() string {
	return fifo.name
}

// Reset resets the algorithm state
func (fifo *FIFO) Reset() {
	fifo.mu.Lock()
	defer fifo.mu.Unlock()
	// FIFO is stateless, nothing to reset
}
