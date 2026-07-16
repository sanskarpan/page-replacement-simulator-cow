package models

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics tracks system-wide metrics
type Metrics struct {
	// Memory statistics
	TotalFrames      int32
	UsedFrames       atomic.Int32
	FreeFrames       atomic.Int32
	PinnedFrames     atomic.Int32

	// Page statistics
	TotalPages       atomic.Int64
	PagesInMemory    atomic.Int32
	SharedPages      atomic.Int32
	DirtyPages       atomic.Int32

	// Fault statistics
	PageFaults       atomic.Int64
	PageHits         atomic.Int64
	TotalAccesses    atomic.Int64

	// Copy-on-Write statistics
	CoWCopies        atomic.Int64
	CoWSaves         atomic.Int64 // Avoided copies
	SharedPageReads  atomic.Int64

	// Page replacement statistics
	Evictions        atomic.Int64
	DirtyEvictions   atomic.Int64 // Evictions requiring write-back

	// Algorithm performance
	AvgSearchTimeNs  atomic.Int64
	LastEvictionTimeNs atomic.Int64

	// Observability
	DroppedEvents atomic.Int64 // events dropped because the internal channel was full

	// Process statistics
	TotalProcesses   atomic.Int32
	ActiveProcesses  atomic.Int32

	// Timing
	StartTime        time.Time
	mu               sync.RWMutex
}

// NewMetrics creates a new metrics tracker
func NewMetrics(totalFrames int32) *Metrics {
	m := &Metrics{
		TotalFrames: totalFrames,
		StartTime:   time.Now(),
	}
	m.FreeFrames.Store(totalFrames)
	m.UsedFrames.Store(0)
	m.PinnedFrames.Store(0)
	m.TotalPages.Store(0)
	m.PagesInMemory.Store(0)
	m.SharedPages.Store(0)
	m.DirtyPages.Store(0)
	m.PageFaults.Store(0)
	m.PageHits.Store(0)
	m.TotalAccesses.Store(0)
	m.CoWCopies.Store(0)
	m.CoWSaves.Store(0)
	m.SharedPageReads.Store(0)
	m.Evictions.Store(0)
	m.DirtyEvictions.Store(0)
	m.AvgSearchTimeNs.Store(0)
	m.LastEvictionTimeNs.Store(0)
	m.TotalProcesses.Store(0)
	m.ActiveProcesses.Store(0)
	m.DroppedEvents.Store(0)
	return m
}

// RecordPageFault records a page fault
func (m *Metrics) RecordPageFault() {
	m.PageFaults.Add(1)
	m.TotalAccesses.Add(1)
}

// RecordPageHit records a page hit
func (m *Metrics) RecordPageHit() {
	m.PageHits.Add(1)
	m.TotalAccesses.Add(1)
}

// RecordEviction records a page eviction
func (m *Metrics) RecordEviction(dirty bool) {
	m.Evictions.Add(1)
	if dirty {
		m.DirtyEvictions.Add(1)
	}
}

// RecordCoWCopy records a copy-on-write operation
func (m *Metrics) RecordCoWCopy() {
	m.CoWCopies.Add(1)
}

// RecordCoWSave records an avoided copy (shared read)
func (m *Metrics) RecordCoWSave() {
	m.CoWSaves.Add(1)
}

// RecordSharedPageRead records a read from a shared page
func (m *Metrics) RecordSharedPageRead() {
	m.SharedPageReads.Add(1)
}

// UpdateFrameStats updates frame statistics
func (m *Metrics) UpdateFrameStats(used, free, pinned int32) {
	m.UsedFrames.Store(used)
	m.FreeFrames.Store(free)
	m.PinnedFrames.Store(pinned)
}

// UpdatePageStats updates page statistics
func (m *Metrics) UpdatePageStats(inMemory, shared, dirty int32) {
	m.PagesInMemory.Store(inMemory)
	m.SharedPages.Store(shared)
	m.DirtyPages.Store(dirty)
}

// GetPageFaultRate returns the page fault rate (0.0 to 1.0)
func (m *Metrics) GetPageFaultRate() float64 {
	accesses := m.TotalAccesses.Load()
	if accesses == 0 {
		return 0.0
	}
	faults := m.PageFaults.Load()
	return float64(faults) / float64(accesses)
}

// GetPageHitRate returns the page hit rate (0.0 to 1.0)
func (m *Metrics) GetPageHitRate() float64 {
	accesses := m.TotalAccesses.Load()
	if accesses == 0 {
		return 0.0
	}
	hits := m.PageHits.Load()
	return float64(hits) / float64(accesses)
}

// GetUptime returns the system uptime.
// StartTime is read under m.mu to avoid a data race with Reset().
func (m *Metrics) GetUptime() time.Duration {
	m.mu.RLock()
	t := m.StartTime
	m.mu.RUnlock()
	return time.Since(t)
}

// GetSnapshot returns a snapshot of current metrics
func (m *Metrics) GetSnapshot() *MetricsSnapshot {
	return m.GetSnapshotWithStats(
		m.UsedFrames.Load(),
		m.FreeFrames.Load(),
		m.PinnedFrames.Load(),
		m.PagesInMemory.Load(),
		m.SharedPages.Load(),
		m.DirtyPages.Load(),
	)
}

// GetSnapshotWithStats returns a snapshot with externally computed frame/page statistics
func (m *Metrics) GetSnapshotWithStats(usedFrames, freeFrames, pinnedFrames, pagesInMemory, sharedPages, dirtyPages int32) *MetricsSnapshot {
	return &MetricsSnapshot{
		TotalFrames:        m.TotalFrames,
		UsedFrames:         usedFrames,
		FreeFrames:         freeFrames,
		PinnedFrames:       pinnedFrames,
		TotalPages:         m.TotalPages.Load(),
		PagesInMemory:      pagesInMemory,
		SharedPages:        sharedPages,
		DirtyPages:         dirtyPages,
		PageFaults:         m.PageFaults.Load(),
		PageHits:           m.PageHits.Load(),
		TotalAccesses:      m.TotalAccesses.Load(),
		CoWCopies:          m.CoWCopies.Load(),
		CoWSaves:           m.CoWSaves.Load(),
		SharedPageReads:    m.SharedPageReads.Load(),
		Evictions:          m.Evictions.Load(),
		DirtyEvictions:     m.DirtyEvictions.Load(),
		AvgSearchTimeNs:    m.AvgSearchTimeNs.Load(),
		LastEvictionTimeNs: m.LastEvictionTimeNs.Load(),
		TotalProcesses:     m.TotalProcesses.Load(),
		ActiveProcesses:    m.ActiveProcesses.Load(),
		PageFaultRate:      m.GetPageFaultRate(),
		PageHitRate:        m.GetPageHitRate(),
		Uptime:             m.GetUptime(),
		DroppedEvents:      m.DroppedEvents.Load(),
	}
}

// MetricsSnapshot is a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	TotalFrames        int32         `json:"total_frames"`
	UsedFrames         int32         `json:"used_frames"`
	FreeFrames         int32         `json:"free_frames"`
	PinnedFrames       int32         `json:"pinned_frames"`
	TotalPages         int64         `json:"total_pages"`
	PagesInMemory      int32         `json:"pages_in_memory"`
	SharedPages        int32         `json:"shared_pages"`
	DirtyPages         int32         `json:"dirty_pages"`
	PageFaults         int64         `json:"page_faults"`
	PageHits           int64         `json:"page_hits"`
	TotalAccesses      int64         `json:"total_accesses"`
	CoWCopies          int64         `json:"cow_copies"`
	CoWSaves           int64         `json:"cow_saves"`
	SharedPageReads    int64         `json:"shared_page_reads"`
	Evictions          int64         `json:"evictions"`
	DirtyEvictions     int64         `json:"dirty_evictions"`
	AvgSearchTimeNs    int64         `json:"avg_search_time_ns"`
	LastEvictionTimeNs int64         `json:"last_eviction_time_ns"`
	TotalProcesses     int32         `json:"total_processes"`
	ActiveProcesses    int32         `json:"active_processes"`
	PageFaultRate      float64       `json:"page_fault_rate"`
	PageHitRate        float64       `json:"page_hit_rate"`
	Uptime             time.Duration `json:"uptime_ns"`
	DroppedEvents      int64         `json:"dropped_events"`
}

// Reset resets all metrics
func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.UsedFrames.Store(0)
	m.FreeFrames.Store(m.TotalFrames)
	m.PinnedFrames.Store(0)
	m.TotalPages.Store(0)
	m.PagesInMemory.Store(0)
	m.SharedPages.Store(0)
	m.DirtyPages.Store(0)
	m.PageFaults.Store(0)
	m.PageHits.Store(0)
	m.TotalAccesses.Store(0)
	m.CoWCopies.Store(0)
	m.CoWSaves.Store(0)
	m.SharedPageReads.Store(0)
	m.Evictions.Store(0)
	m.DirtyEvictions.Store(0)
	m.AvgSearchTimeNs.Store(0)
	m.LastEvictionTimeNs.Store(0)
	m.DroppedEvents.Store(0)
	m.StartTime = time.Now()
}
