package algorithms

import (
	"fmt"
	"sync"

	"github.com/page-replacement-cow/pkg/models"
)

// LFU implements the Least Frequently Used page replacement algorithm
type LFU struct {
	name string
	mu   sync.RWMutex
}

// NewLFU creates a new LFU algorithm instance
func NewLFU() *LFU {
	return &LFU{
		name: "LFU",
	}
}

// SelectVictim selects the least frequently used page for eviction
func (lfu *LFU) SelectVictim(frames []*models.Frame) (*models.Frame, error) {
	lfu.mu.RLock()
	defer lfu.mu.RUnlock()

	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames available for eviction")
	}

	var victim *models.Frame
	var lowestCount int64 = -1
	first := true

	for _, frame := range frames {
		// Skip free or pinned frames
		if frame.IsFree() || frame.IsPinned() {
			continue
		}

		accessCount := frame.GetAccessCount()
		if first || accessCount < lowestCount {
			victim = frame
			lowestCount = accessCount
			first = false
		} else if accessCount == lowestCount {
			// Tie-breaker: use LRU among pages with same frequency
			if victim != nil && frame.GetLastAccessTime().Before(victim.GetLastAccessTime()) {
				victim = frame
			}
		}
	}

	if victim == nil {
		return nil, fmt.Errorf("no evictable frame found")
	}

	return victim, nil
}

// OnPageAccess is called when a page is accessed
func (lfu *LFU) OnPageAccess(frame *models.Frame, write bool) {
	// Frame already tracks access count
	frame.Access(write)
}

// OnPageFault is called when a page fault occurs
func (lfu *LFU) OnPageFault(frame *models.Frame) {
	// Initialize access count
	if frame != nil {
		frame.Access(false)
	}
}

// OnPageEviction is called when a page is evicted
func (lfu *LFU) OnPageEviction(frame *models.Frame) {
	// No special handling needed for LFU
}

// GetName returns the name of the algorithm
func (lfu *LFU) GetName() string {
	return lfu.name
}

// Reset resets the algorithm state
func (lfu *LFU) Reset() {
	lfu.mu.Lock()
	defer lfu.mu.Unlock()
	// LFU is stateless, nothing to reset
}
