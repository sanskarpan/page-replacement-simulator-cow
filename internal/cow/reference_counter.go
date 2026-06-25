package cow

import (
	"sync"
	"sync/atomic"
)

// ReferenceCounter tracks reference counts for pages
type ReferenceCounter struct {
	counts    map[uint64]*atomic.Int32
	totalRefs atomic.Int64
	mu        sync.RWMutex
}

// NewReferenceCounter creates a new reference counter
func NewReferenceCounter() *ReferenceCounter {
	return &ReferenceCounter{
		counts: make(map[uint64]*atomic.Int32),
	}
}

// Increment increments the reference count for a page
func (rc *ReferenceCounter) Increment(pageID uint64) int32 {
	rc.mu.Lock()
	counter, exists := rc.counts[pageID]
	if !exists {
		counter = &atomic.Int32{}
		rc.counts[pageID] = counter
	}
	rc.mu.Unlock()

	newCount := counter.Add(1)
	rc.totalRefs.Add(1)
	return newCount
}

// Decrement decrements the reference count for a page
func (rc *ReferenceCounter) Decrement(pageID uint64) int32 {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	counter, exists := rc.counts[pageID]
	if !exists {
		return 0
	}

	newCount := counter.Add(-1)
	rc.totalRefs.Add(-1)

	if newCount <= 0 {
		delete(rc.counts, pageID)
	}

	return newCount
}

// Get returns the current reference count for a page
func (rc *ReferenceCounter) Get(pageID uint64) int32 {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	counter, exists := rc.counts[pageID]
	if !exists {
		return 0
	}

	return counter.Load()
}

// GetTotalRefs returns the total number of references
func (rc *ReferenceCounter) GetTotalRefs() int64 {
	return rc.totalRefs.Load()
}

// Clear clears all reference counts
func (rc *ReferenceCounter) Clear() {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.counts = make(map[uint64]*atomic.Int32)
	rc.totalRefs.Store(0)
}

// GetAllRefs returns a snapshot of all reference counts
func (rc *ReferenceCounter) GetAllRefs() map[uint64]int32 {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	snapshot := make(map[uint64]int32)
	for pageID, counter := range rc.counts {
		snapshot[pageID] = counter.Load()
	}

	return snapshot
}
