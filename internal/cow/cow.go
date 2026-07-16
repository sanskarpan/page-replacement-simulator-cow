// Package cow implements Copy-on-Write semantics for forked processes.
// On fork, pages are marked Shared+ReadOnly; the first write triggers a
// physical copy so parent and child diverge cleanly.
package cow

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/page-replacement-cow/pkg/models"
)

// CopyOnWrite manages copy-on-write functionality
type CopyOnWrite struct {
	// Shared page tracking
	sharedPages  map[uint64]*SharedPage // Page ID -> SharedPage
	refCounter   *ReferenceCounter
	nextPageID   atomic.Uint64

	// Statistics
	copiesCreated atomic.Int64
	copiesAvoided atomic.Int64

	mu sync.RWMutex
}

// SharedPage represents a page that is shared between processes
type SharedPage struct {
	PageID      uint64
	FrameNumber int32
	Processes   map[string]bool // Set of process IDs sharing this page
	RefCount    atomic.Int32
	mu          sync.RWMutex
}

// NewCopyOnWrite creates a new CoW manager
func NewCopyOnWrite() *CopyOnWrite {
	cow := &CopyOnWrite{
		sharedPages: make(map[uint64]*SharedPage),
		refCounter:  NewReferenceCounter(),
	}
	cow.nextPageID.Store(1000000) // Start high to avoid conflicts
	cow.copiesCreated.Store(0)
	cow.copiesAvoided.Store(0)
	return cow
}

// SharePage marks a page as shared between processes
func (cow *CopyOnWrite) SharePage(pageID uint64, frameNumber int32, processIDs []string) error {
	cow.mu.Lock()
	defer cow.mu.Unlock()

	sharedPage, exists := cow.sharedPages[pageID]
	if !exists {
		sharedPage = &SharedPage{
			PageID:      pageID,
			FrameNumber: frameNumber,
			Processes:   make(map[string]bool),
		}
		sharedPage.RefCount.Store(0)
		cow.sharedPages[pageID] = sharedPage
	}

	// Add all processes
	for _, pid := range processIDs {
		if !sharedPage.Processes[pid] {
			sharedPage.Processes[pid] = true
			sharedPage.RefCount.Add(1)
			cow.refCounter.Increment(pageID)
		}
	}

	return nil
}

// HandleWrite handles a write to a potentially shared page
// Returns: (needsCopy, newPageID, error)
func (cow *CopyOnWrite) HandleWrite(pageID uint64, processID string, page *models.Page) (bool, uint64, error) {
	cow.mu.RLock()
	_, isShared := cow.sharedPages[pageID]
	cow.mu.RUnlock()

	if !isShared || !page.IsShared() {
		cow.copiesAvoided.Add(1)
		return false, 0, nil
	}

	// Hold the write lock for the entire check-and-decrement so no two
	// concurrent writers can both see refCount>1 for the same page (TOCTOU fix).
	cow.mu.Lock()
	sharedPage, exists := cow.sharedPages[pageID]
	if !exists {
		cow.mu.Unlock()
		cow.copiesAvoided.Add(1)
		return false, 0, nil
	}

	refCount := sharedPage.RefCount.Load()
	if refCount <= 1 {
		// Last reference: take exclusive ownership without copying.
		// Inline the unshare while still holding cow.mu to close the TOCTOU
		// window where a concurrent ForkProcess could add a new sharer between
		// our Unlock() and unsharePageInternal's re-Lock().
		if sharedPage.Processes[processID] {
			delete(sharedPage.Processes, processID)
			sharedPage.RefCount.Add(-1)
			cow.refCounter.Decrement(pageID)
			if sharedPage.RefCount.Load() <= 0 {
				delete(cow.sharedPages, pageID)
			}
		}
		// Clear flags while still under cow.mu to prevent a concurrent ForkProcess
		// from interleaving between the Unlock and these stores.
		page.Shared.Store(false)
		page.ReadOnly.Store(false)
		cow.mu.Unlock()
		cow.copiesAvoided.Add(1)
		return false, 0, nil
	}

	// Multiple references: decrement atomically while holding the lock so no
	// concurrent writer also sees refCount>1 for this process entry.
	if sharedPage.Processes[processID] {
		delete(sharedPage.Processes, processID)
		sharedPage.RefCount.Add(-1)
		cow.refCounter.Decrement(pageID)
		if sharedPage.RefCount.Load() <= 0 {
			delete(cow.sharedPages, pageID)
		}
	}
	cow.mu.Unlock()

	newPageID := cow.nextPageID.Add(1)
	return true, newPageID, nil
}

// decrementRefCount decrements the reference count for a page
func (cow *CopyOnWrite) decrementRefCount(pageID uint64, processID string) {
	cow.mu.Lock()
	defer cow.mu.Unlock()

	sharedPage, exists := cow.sharedPages[pageID]
	if !exists {
		return
	}

	sharedPage.mu.Lock()
	defer sharedPage.mu.Unlock()

	if sharedPage.Processes[processID] {
		delete(sharedPage.Processes, processID)
		sharedPage.RefCount.Add(-1)
		cow.refCounter.Decrement(pageID)

		// If no more references, remove from shared pages
		if sharedPage.RefCount.Load() <= 0 {
			delete(cow.sharedPages, pageID)
		}
	}
}

// unsharePageInternal removes a page from shared tracking
func (cow *CopyOnWrite) unsharePageInternal(pageID uint64, processID string) {
	cow.mu.Lock()
	defer cow.mu.Unlock()

	sharedPage, exists := cow.sharedPages[pageID]
	if !exists {
		return
	}

	sharedPage.mu.Lock()
	defer sharedPage.mu.Unlock()

	if sharedPage.Processes[processID] {
		delete(sharedPage.Processes, processID)
		sharedPage.RefCount.Add(-1)
		cow.refCounter.Decrement(pageID)

		if sharedPage.RefCount.Load() <= 0 {
			delete(cow.sharedPages, pageID)
		}
	}
}

// UnsharePage removes a page from shared tracking for a process
func (cow *CopyOnWrite) UnsharePage(pageID uint64, processID string) {
	cow.decrementRefCount(pageID, processID)
}

// GetRefCount returns the reference count for a page
func (cow *CopyOnWrite) GetRefCount(pageID uint64) int32 {
	cow.mu.RLock()
	defer cow.mu.RUnlock()

	sharedPage, exists := cow.sharedPages[pageID]
	if !exists {
		return 0
	}

	return sharedPage.RefCount.Load()
}

// IsShared returns true if a page is shared
func (cow *CopyOnWrite) IsShared(pageID uint64) bool {
	cow.mu.RLock()
	defer cow.mu.RUnlock()

	sharedPage, exists := cow.sharedPages[pageID]
	if !exists {
		return false
	}

	return sharedPage.RefCount.Load() > 1
}

// GetSharedPages returns all shared pages
func (cow *CopyOnWrite) GetSharedPages() []uint64 {
	cow.mu.RLock()
	defer cow.mu.RUnlock()

	pages := make([]uint64, 0, len(cow.sharedPages))
	for pageID := range cow.sharedPages {
		pages = append(pages, pageID)
	}
	return pages
}

// ForkProcess creates shared mappings for a forked process
func (cow *CopyOnWrite) ForkProcess(parentID, childID string, pages []*models.Page) error {
	cow.mu.Lock()
	defer cow.mu.Unlock()

	for _, page := range pages {
		if !page.IsPresent() {
			continue
		}

		pageID := page.ID
		frameNumber := page.GetFrame()

		// Create or get shared page
		sharedPage, exists := cow.sharedPages[pageID]
		if !exists {
			sharedPage = &SharedPage{
				PageID:      pageID,
				FrameNumber: frameNumber,
				Processes:   make(map[string]bool),
			}
			sharedPage.RefCount.Store(0)
			cow.sharedPages[pageID] = sharedPage
		}

		// Add both parent and child
		if !sharedPage.Processes[parentID] {
			sharedPage.Processes[parentID] = true
			sharedPage.RefCount.Add(1)
			cow.refCounter.Increment(pageID)
		}

		if !sharedPage.Processes[childID] {
			sharedPage.Processes[childID] = true
			sharedPage.RefCount.Add(1)
			cow.refCounter.Increment(pageID)
		}

		// Mark pages as shared and read-only
		page.MakeShared()
	}

	return nil
}

// RemoveProcess removes all shared pages for a process
func (cow *CopyOnWrite) RemoveProcess(processID string) {
	cow.mu.Lock()
	defer cow.mu.Unlock()

	// Find all pages shared by this process
	pagesToRemove := make([]uint64, 0)

	for pageID, sharedPage := range cow.sharedPages {
		sharedPage.mu.Lock()
		if sharedPage.Processes[processID] {
			delete(sharedPage.Processes, processID)
			sharedPage.RefCount.Add(-1)
			cow.refCounter.Decrement(pageID)

			if sharedPage.RefCount.Load() <= 0 {
				pagesToRemove = append(pagesToRemove, pageID)
			}
		}
		sharedPage.mu.Unlock()
	}

	// Remove pages with no references
	for _, pageID := range pagesToRemove {
		delete(cow.sharedPages, pageID)
	}
}

// GetStats returns CoW statistics
func (cow *CopyOnWrite) GetStats() CoWStats {
	cow.mu.RLock()
	defer cow.mu.RUnlock()

	return CoWStats{
		SharedPages:   len(cow.sharedPages),
		CopiesCreated: cow.copiesCreated.Load(),
		CopiesAvoided: cow.copiesAvoided.Load(),
		TotalRefs:     cow.refCounter.GetTotalRefs(),
	}
}

// CoWStats contains CoW statistics
type CoWStats struct {
	SharedPages   int
	CopiesCreated int64
	CopiesAvoided int64
	TotalRefs     int64
}

// Reset resets all CoW state
func (cow *CopyOnWrite) Reset() {
	cow.mu.Lock()
	defer cow.mu.Unlock()

	cow.sharedPages = make(map[uint64]*SharedPage)
	cow.refCounter = NewReferenceCounter()
	cow.copiesCreated.Store(0)
	cow.copiesAvoided.Store(0)
}

// CopyPage creates a physical copy of a page
// This would trigger actual memory copy in a real system
func (cow *CopyOnWrite) CopyPage(originalPage *models.Page, newPageID uint64, processID string) (*models.Page, error) {
	if originalPage == nil {
		return nil, fmt.Errorf("original page is nil")
	}

	// Create new page with new ID
	newPage := models.NewPage(newPageID, processID)
	newPage.OriginalPage = originalPage.ID

	// Copy metadata (but not frame - that will be allocated separately)
	newPage.LastAccessed.Store(originalPage.LastAccessed.Load())
	newPage.Shared.Store(false)
	newPage.ReadOnly.Store(false)
	newPage.RefCount.Store(1)

	cow.copiesCreated.Add(1)

	return newPage, nil
}
