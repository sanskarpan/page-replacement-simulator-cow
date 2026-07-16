package models

import (
	"sync"
	"sync/atomic"
	"time"
)

// PageState represents the state of a page
type PageState int32

const (
	PageInvalid PageState = iota
	PageValid
	PageDirty
	PageShared
)

// Page represents a virtual memory page
type Page struct {
	ID          uint64    // Virtual page number
	ProcessID   string    // Owner process
	FrameNumber int32     // Physical frame number (-1 if not in memory)
	// json:"-" on all atomic fields prevents the encoder from emitting raw
	// atomic struct internals; use accessor methods or a DTO for serialization.
	State       atomic.Int32 `json:"-"`

	// Access tracking
	LastAccessed atomic.Int64 `json:"-"` // Unix nano timestamp
	AccessCount  atomic.Int64 `json:"-"` // For LFU
	ReferenceBit atomic.Int32 `json:"-"` // For CLOCK (0 or 1)

	// Copy-on-Write
	Shared       atomic.Bool  `json:"-"`
	RefCount     atomic.Int32 `json:"-"` // Reference count for CoW
	OriginalPage uint64                   // Original page ID if this is a CoW copy

	// Metadata
	Dirty     atomic.Bool `json:"-"`
	Present   atomic.Bool `json:"-"` // In physical memory
	ReadOnly  atomic.Bool `json:"-"` // For CoW shared pages
	CreatedAt time.Time

	mu sync.RWMutex
}

// NewPage creates a new page
func NewPage(id uint64, processID string) *Page {
	p := &Page{
		ID:        id,
		ProcessID: processID,
		CreatedAt: time.Now(),
	}
	p.FrameNumber = -1
	p.State.Store(int32(PageInvalid))
	p.LastAccessed.Store(time.Now().UnixNano())
	p.AccessCount.Store(0)
	p.ReferenceBit.Store(0)
	p.RefCount.Store(1)
	p.Present.Store(false)
	p.Shared.Store(false)
	p.Dirty.Store(false)
	p.ReadOnly.Store(false)
	return p
}

// Access records an access to this page
func (p *Page) Access(write bool) {
	p.LastAccessed.Store(time.Now().UnixNano())
	p.AccessCount.Add(1)
	p.ReferenceBit.Store(1) // Set reference bit for CLOCK

	if write {
		p.Dirty.Store(true)
		// Attempt transition from Valid first, then Shared.
		if !p.State.CompareAndSwap(int32(PageValid), int32(PageDirty)) {
			p.State.CompareAndSwap(int32(PageShared), int32(PageDirty))
		}
	}
}

// GetLastAccessTime returns last access time
func (p *Page) GetLastAccessTime() time.Time {
	return time.Unix(0, p.LastAccessed.Load())
}

// GetAccessCount returns the access count
func (p *Page) GetAccessCount() int64 {
	return p.AccessCount.Load()
}

// SetFrame sets the frame number
func (p *Page) SetFrame(frameNum int32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.FrameNumber = frameNum
	if frameNum >= 0 {
		p.Present.Store(true)
		p.State.Store(int32(PageValid))
	} else {
		p.Present.Store(false)
		p.State.Store(int32(PageInvalid))
	}
}

// GetFrame returns the frame number
func (p *Page) GetFrame() int32 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.FrameNumber
}

// IsPresent returns true if page is in physical memory
func (p *Page) IsPresent() bool {
	return p.Present.Load()
}

// IsDirty returns true if page has been modified
func (p *Page) IsDirty() bool {
	return p.Dirty.Load()
}

// IsShared returns true if page is shared (CoW)
func (p *Page) IsShared() bool {
	return p.Shared.Load()
}

// MakeShared marks page as shared for CoW.
// Both flags are set under mu so a concurrent Access() call can never observe
// Shared=true with ReadOnly=false.
func (p *Page) MakeShared() {
	p.mu.Lock()
	p.Shared.Store(true)
	p.ReadOnly.Store(true)
	p.mu.Unlock()
}

// ClearReferenceBit clears the reference bit and returns old value
func (p *Page) ClearReferenceBit() bool {
	return p.ReferenceBit.Swap(0) == 1
}

// GetReferenceBit returns the current reference bit
func (p *Page) GetReferenceBit() bool {
	return p.ReferenceBit.Load() == 1
}

// IncrementRefCount increments reference count
func (p *Page) IncrementRefCount() int32 {
	return p.RefCount.Add(1)
}

// DecrementRefCount decrements reference count, clamping at zero to prevent underflow.
func (p *Page) DecrementRefCount() int32 {
	for {
		cur := p.RefCount.Load()
		if cur <= 0 {
			return 0
		}
		if p.RefCount.CompareAndSwap(cur, cur-1) {
			return cur - 1
		}
	}
}

// GetRefCount returns current reference count
func (p *Page) GetRefCount() int32 {
	return p.RefCount.Load()
}

// Clone creates a copy of the page for CoW
func (p *Page) Clone(newID uint64) *Page {
	p.mu.RLock()
	defer p.mu.RUnlock()

	newPage := &Page{
		ID:           newID,
		ProcessID:    p.ProcessID,
		FrameNumber:  -1,
		OriginalPage: p.ID,
		CreatedAt:    time.Now(),
	}

	newPage.State.Store(int32(PageInvalid))
	newPage.LastAccessed.Store(time.Now().UnixNano())
	newPage.AccessCount.Store(0)
	newPage.ReferenceBit.Store(0)
	newPage.RefCount.Store(1)
	newPage.Present.Store(false)
	newPage.Shared.Store(false)
	newPage.Dirty.Store(false)
	newPage.ReadOnly.Store(false)

	return newPage
}

// LockShared acquires the page's read lock for a multi-field atomic snapshot.
func (p *Page) LockShared()   { p.mu.RLock() }

// UnlockShared releases the page's read lock.
func (p *Page) UnlockShared() { p.mu.RUnlock() }

// GetStateString returns a string representation of the page state
func (p *Page) GetStateString() string {
	state := PageState(p.State.Load())
	switch state {
	case PageInvalid:
		return "Invalid"
	case PageValid:
		return "Valid"
	case PageDirty:
		return "Dirty"
	case PageShared:
		return "Shared"
	default:
		return "Unknown"
	}
}
