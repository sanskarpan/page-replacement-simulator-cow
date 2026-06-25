package algorithms

import (
	"fmt"
	"sync"
	"time"

	"github.com/page-replacement-cow/pkg/models"
)

// LRU implements the Least Recently Used page replacement algorithm
type LRU struct {
	name string
	mu   sync.RWMutex
}

// NewLRU creates a new LRU algorithm instance
func NewLRU() *LRU {
	return &LRU{
		name: "LRU",
	}
}

// SelectVictim selects the least recently used page for eviction
func (lru *LRU) SelectVictim(frames []*models.Frame) (*models.Frame, error) {
	lru.mu.RLock()
	defer lru.mu.RUnlock()

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

		lastAccess := frame.GetLastAccessTime()
		if first || lastAccess.Before(oldestTime) {
			victim = frame
			oldestTime = lastAccess
			first = false
		}
	}

	if victim == nil {
		return nil, fmt.Errorf("no evictable frame found")
	}

	return victim, nil
}

// OnPageAccess is called when a page is accessed
func (lru *LRU) OnPageAccess(frame *models.Frame, write bool) {
	// Frame already tracks last access time
	frame.Access(write)
}

// OnPageFault is called when a page fault occurs
func (lru *LRU) OnPageFault(frame *models.Frame) {
	// Update access time
	if frame != nil {
		frame.Access(false)
	}
}

// OnPageEviction is called when a page is evicted
func (lru *LRU) OnPageEviction(frame *models.Frame) {
	// No special handling needed for LRU
}

// GetName returns the name of the algorithm
func (lru *LRU) GetName() string {
	return lru.name
}

// Reset resets the algorithm state
func (lru *LRU) Reset() {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	// LRU is stateless, nothing to reset
}
