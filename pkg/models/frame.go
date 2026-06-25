package models

import (
	"sync"
	"sync/atomic"
	"time"
)

// Frame represents a physical memory frame
type Frame struct {
	ID          int32  // Frame number
	PageID      uint64 // Currently mapped page ID (0 if free)
	ProcessID   string // Process owning this frame

	// State
	Free     atomic.Bool
	Pinned   atomic.Bool // Pinned frames cannot be evicted
	Modified atomic.Bool // Dirty bit

	// Tracking
	LoadedAt    atomic.Int64 // Unix nano when page was loaded
	LastAccess  atomic.Int64 // Unix nano of last access
	AccessCount atomic.Int64 // Access counter

	mu sync.RWMutex

	// For CLOCK algorithm
	ReferenceBit atomic.Int32
}

// NewFrame creates a new frame
func NewFrame(id int32) *Frame {
	f := &Frame{
		ID: id,
	}
	f.Free.Store(true)
	f.Pinned.Store(false)
	f.Modified.Store(false)
	f.ReferenceBit.Store(0)
	f.LoadedAt.Store(0)
	f.LastAccess.Store(0)
	f.AccessCount.Store(0)
	return f
}

// Allocate allocates this frame to a page
func (f *Frame) Allocate(pageID uint64, processID string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.PageID = pageID
	f.ProcessID = processID
	f.Free.Store(false)
	f.Modified.Store(false)
	f.ReferenceBit.Store(1)

	now := time.Now().UnixNano()
	f.LoadedAt.Store(now)
	f.LastAccess.Store(now)
	f.AccessCount.Store(0)
}

// Release releases this frame
func (f *Frame) Release() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.PageID = 0
	f.ProcessID = ""
	f.Free.Store(true)
	f.Modified.Store(false)
	f.ReferenceBit.Store(0)
	f.LoadedAt.Store(0)
	f.LastAccess.Store(0)
	f.AccessCount.Store(0)
}

// Access records an access to this frame
func (f *Frame) Access(write bool) {
	f.LastAccess.Store(time.Now().UnixNano())
	f.AccessCount.Add(1)
	f.ReferenceBit.Store(1)

	if write {
		f.Modified.Store(true)
	}
}

// IsFree returns true if frame is free
func (f *Frame) IsFree() bool {
	return f.Free.Load()
}

// IsPinned returns true if frame is pinned
func (f *Frame) IsPinned() bool {
	return f.Pinned.Load()
}

// IsModified returns true if frame has been modified
func (f *Frame) IsModified() bool {
	return f.Modified.Load()
}

// Pin pins this frame
func (f *Frame) Pin() {
	f.Pinned.Store(true)
}

// Unpin unpins this frame
func (f *Frame) Unpin() {
	f.Pinned.Store(false)
}

// GetReferenceBit returns the reference bit
func (f *Frame) GetReferenceBit() bool {
	return f.ReferenceBit.Load() == 1
}

// ClearReferenceBit clears the reference bit and returns old value
func (f *Frame) ClearReferenceBit() bool {
	return f.ReferenceBit.Swap(0) == 1
}

// GetLastAccessTime returns the last access time
func (f *Frame) GetLastAccessTime() time.Time {
	return time.Unix(0, f.LastAccess.Load())
}

// GetLoadedTime returns when the page was loaded
func (f *Frame) GetLoadedTime() time.Time {
	return time.Unix(0, f.LoadedAt.Load())
}

// GetAccessCount returns the access count
func (f *Frame) GetAccessCount() int64 {
	return f.AccessCount.Load()
}

// GetPageID returns the current page ID
func (f *Frame) GetPageID() uint64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.PageID
}

// GetProcessID returns the current process ID
func (f *Frame) GetProcessID() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.ProcessID
}

// GetAge returns how long the page has been in this frame (nanoseconds)
func (f *Frame) GetAge() int64 {
	if f.IsFree() {
		return 0
	}
	loadedAt := f.LoadedAt.Load()
	if loadedAt == 0 {
		return 0
	}
	return time.Now().UnixNano() - loadedAt
}
