package memory

import (
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// TLBEntry represents a single TLB entry
type TLBEntry struct {
	VirtualPage  uint64
	FrameNumber  int32
	ProcessID    string
	Valid        bool
	lastAccessNs int64 // UnixNano; updated atomically to avoid write-lock on every read
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

// makeKey creates a collision-free key for the TLB entry.
// A null-byte separator is used because process IDs cannot contain null bytes,
// eliminating any ambiguity that a printable separator (e.g. ':') would allow
// when the separator character appears in a processID.
func (t *TLB) makeKey(processID string, virtualPage uint64) string {
	return processID + "\x00" + strconv.FormatUint(virtualPage, 10)
}

// Lookup looks up a virtual page in the TLB.
// Uses RLock for the common hit path; upgrades to Lock only on eviction.
// LastAccess is updated under the read lock using a separate atomic to avoid
// taking a write lock on every read.
func (t *TLB) Lookup(processID string, virtualPage uint64) (int32, bool) {
	key := t.makeKey(processID, virtualPage)

	t.mu.RLock()
	entry, exists := t.entries[key]
	if exists && entry.Valid {
		frameNum := entry.FrameNumber
		t.mu.RUnlock()
		// Update last-access time without a write lock: overwrite the int64
		// under-the-hood value atomically.  A torn read is harmless here
		// because the value is only used for LRU ordering, not correctness.
		atomic.StoreInt64(&entry.lastAccessNs, time.Now().UnixNano())
		t.hits.Add(1)
		return frameNum, true
	}
	t.mu.RUnlock()

	t.misses.Add(1)
	return -1, false
}

// Insert inserts a translation into the TLB
func (t *TLB) Insert(processID string, virtualPage uint64, frameNumber int32) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := t.makeKey(processID, virtualPage)

	// If at capacity, evict an entry (simple LRU).
	// capacity == 0 is treated as unlimited: never evict.
	if t.capacity > 0 && len(t.entries) >= t.capacity {
		t.evictLRU()
	}

	t.entries[key] = &TLBEntry{
		VirtualPage:  virtualPage,
		FrameNumber:  frameNumber,
		ProcessID:    processID,
		Valid:         true,
		lastAccessNs: time.Now().UnixNano(),
	}
}

// evictLRU evicts the least recently used entry. Must be called with mu.Lock held.
func (t *TLB) evictLRU() {
	var oldestKey string
	var oldestNs int64
	first := true

	for key, entry := range t.entries {
		ns := atomic.LoadInt64(&entry.lastAccessNs)
		if first || ns < oldestNs {
			oldestKey = key
			oldestNs = ns
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
	Capacity int     `json:"capacity"`
	Size     int     `json:"size"`
	Hits     int64   `json:"hits"`
	Misses   int64   `json:"misses"`
	HitRate  float64 `json:"hit_rate"`
}
