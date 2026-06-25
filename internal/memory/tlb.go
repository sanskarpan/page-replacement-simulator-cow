package memory

import (
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// TLBEntry represents a single TLB entry
type TLBEntry struct {
	VirtualPage uint64
	FrameNumber int32
	ProcessID   string
	Valid       bool
	LastAccess  time.Time
}

// TLB (Translation Lookaside Buffer) is a cache for page table translations
type TLB struct {
	entries  map[string]*TLBEntry // Key: processID:virtualPage
	capacity int
	hits     atomic.Int64
	misses   atomic.Int64
	mu       sync.RWMutex
}

// NewTLB creates a new TLB with the specified capacity
func NewTLB(capacity int) *TLB {
	tlb := &TLB{
		entries:  make(map[string]*TLBEntry),
		capacity: capacity,
	}
	tlb.hits.Store(0)
	tlb.misses.Store(0)
	return tlb
}

// makeKey creates a key for the TLB entry
func (t *TLB) makeKey(processID string, virtualPage uint64) string {
	return processID + ":" + strconv.FormatUint(virtualPage, 10)
}

// Lookup looks up a virtual page in the TLB
func (t *TLB) Lookup(processID string, virtualPage uint64) (int32, bool) {
	t.mu.Lock()
	key := t.makeKey(processID, virtualPage)
	entry, exists := t.entries[key]
	if exists && entry.Valid {
		entry.LastAccess = time.Now()
		t.mu.Unlock()
		t.hits.Add(1)
		return entry.FrameNumber, true
	}
	t.mu.Unlock()

	t.misses.Add(1)
	return -1, false
}

// Insert inserts a translation into the TLB
func (t *TLB) Insert(processID string, virtualPage uint64, frameNumber int32) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := t.makeKey(processID, virtualPage)

	// If at capacity, evict an entry (simple LRU)
	if len(t.entries) >= t.capacity {
		t.evictLRU()
	}

	t.entries[key] = &TLBEntry{
		VirtualPage: virtualPage,
		FrameNumber: frameNumber,
		ProcessID:   processID,
		Valid:       true,
		LastAccess:  time.Now(),
	}
}

// evictLRU evicts the least recently used entry
func (t *TLB) evictLRU() {
	var oldestKey string
	var oldestTime time.Time
	first := true

	for key, entry := range t.entries {
		if first || entry.LastAccess.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.LastAccess
			first = false
		}
	}

	if oldestKey != "" {
		delete(t.entries, oldestKey)
	}
}

// Invalidate invalidates a TLB entry
func (t *TLB) Invalidate(processID string, virtualPage uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := t.makeKey(processID, virtualPage)
	delete(t.entries, key)
}

// InvalidateProcess invalidates all entries for a process
func (t *TLB) InvalidateProcess(processID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for key, entry := range t.entries {
		if entry.ProcessID == processID {
			delete(t.entries, key)
		}
	}
}

// Clear clears all TLB entries
func (t *TLB) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.entries = make(map[string]*TLBEntry)
	t.hits.Store(0)
	t.misses.Store(0)
}

// GetHitRate returns the TLB hit rate
func (t *TLB) GetHitRate() float64 {
	hits := t.hits.Load()
	misses := t.misses.Load()
	total := hits + misses

	if total == 0 {
		return 0.0
	}
	return float64(hits) / float64(total)
}

// GetStats returns TLB statistics
func (t *TLB) GetStats() TLBStats {
	t.mu.RLock()
	size := len(t.entries)
	t.mu.RUnlock()

	return TLBStats{
		Capacity: t.capacity,
		Size:     size,
		Hits:     t.hits.Load(),
		Misses:   t.misses.Load(),
		HitRate:  t.GetHitRate(),
	}
}

// TLBStats contains TLB statistics
type TLBStats struct {
	Capacity int
	Size     int
	Hits     int64
	Misses   int64
	HitRate  float64
}
