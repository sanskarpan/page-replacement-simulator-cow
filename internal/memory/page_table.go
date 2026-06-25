package memory

import (
	"fmt"
	"sync"

	"github.com/page-replacement-cow/pkg/models"
)

// PageTable represents a process's page table
type PageTable struct {
	ProcessID string
	Entries   map[uint64]*models.Page // Virtual page number -> Page
	mu        sync.RWMutex
}

// NewPageTable creates a new page table
func NewPageTable(processID string) *PageTable {
	return &PageTable{
		ProcessID: processID,
		Entries:   make(map[uint64]*models.Page),
	}
}

// AddPage adds a page to the page table
func (pt *PageTable) AddPage(virtualPage uint64) *models.Page {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	// Check if page already exists
	if page, exists := pt.Entries[virtualPage]; exists {
		return page
	}

	// Create new page
	page := models.NewPage(virtualPage, pt.ProcessID)
	pt.Entries[virtualPage] = page
	return page
}

// GetPage retrieves a page from the page table
func (pt *PageTable) GetPage(virtualPage uint64) (*models.Page, error) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	page, exists := pt.Entries[virtualPage]
	if !exists {
		return nil, fmt.Errorf("page %d not found in page table", virtualPage)
	}
	return page, nil
}

// GetOrCreatePage gets a page or creates it if it doesn't exist
func (pt *PageTable) GetOrCreatePage(virtualPage uint64) *models.Page {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if page, exists := pt.Entries[virtualPage]; exists {
		return page
	}

	page := models.NewPage(virtualPage, pt.ProcessID)
	pt.Entries[virtualPage] = page
	return page
}

// RemovePage removes a page from the page table
func (pt *PageTable) RemovePage(virtualPage uint64) error {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if _, exists := pt.Entries[virtualPage]; !exists {
		return fmt.Errorf("page %d not found", virtualPage)
	}

	delete(pt.Entries, virtualPage)
	return nil
}

// GetAllPages returns all pages in the page table
func (pt *PageTable) GetAllPages() []*models.Page {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	pages := make([]*models.Page, 0, len(pt.Entries))
	for _, page := range pt.Entries {
		pages = append(pages, page)
	}
	return pages
}

// GetPresentPages returns all pages currently in memory
func (pt *PageTable) GetPresentPages() []*models.Page {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	pages := make([]*models.Page, 0)
	for _, page := range pt.Entries {
		if page.IsPresent() {
			pages = append(pages, page)
		}
	}
	return pages
}

// GetSharedPages returns all shared (CoW) pages
func (pt *PageTable) GetSharedPages() []*models.Page {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	pages := make([]*models.Page, 0)
	for _, page := range pt.Entries {
		if page.IsShared() {
			pages = append(pages, page)
		}
	}
	return pages
}

// Clear clears all entries from the page table
func (pt *PageTable) Clear() {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.Entries = make(map[uint64]*models.Page)
}

// Size returns the number of entries in the page table
func (pt *PageTable) Size() int {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return len(pt.Entries)
}

// Clone creates a copy of this page table for fork/CoW
func (pt *PageTable) Clone(newProcessID string) *PageTable {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	newPT := NewPageTable(newProcessID)
	for vpn, page := range pt.Entries {
		// Create shallow copy - pages will be shared initially
		newPage := &models.Page{
			ID:           page.ID,
			ProcessID:    newProcessID,
			FrameNumber:  page.GetFrame(),
			OriginalPage: page.ID,
			CreatedAt:    page.CreatedAt,
		}
		newPage.State.Store(page.State.Load())
		newPage.LastAccessed.Store(page.LastAccessed.Load())
		newPage.AccessCount.Store(0)
		newPage.ReferenceBit.Store(page.ReferenceBit.Load())
		newPage.RefCount.Store(page.RefCount.Load())
		newPage.Present.Store(page.Present.Load())
		newPage.Shared.Store(true) // Mark as shared for CoW
		newPage.Dirty.Store(false)
		newPage.ReadOnly.Store(true) // Mark as read-only

		newPT.Entries[vpn] = newPage
	}

	return newPT
}

// UpdateFrame updates the frame mapping for a page
func (pt *PageTable) UpdateFrame(virtualPage uint64, frameNum int32) error {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	page, exists := pt.Entries[virtualPage]
	if !exists {
		return fmt.Errorf("page %d not found", virtualPage)
	}

	page.SetFrame(frameNum)
	return nil
}

// GetStats returns statistics about the page table
func (pt *PageTable) GetStats() PageTableStats {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	stats := PageTableStats{
		TotalPages:   len(pt.Entries),
		PresentPages: 0,
		SharedPages:  0,
		DirtyPages:   0,
	}

	for _, page := range pt.Entries {
		if page.IsPresent() {
			stats.PresentPages++
		}
		if page.IsShared() {
			stats.SharedPages++
		}
		if page.IsDirty() {
			stats.DirtyPages++
		}
	}

	return stats
}

// PageTableStats contains statistics about a page table
type PageTableStats struct {
	TotalPages   int
	PresentPages int
	SharedPages  int
	DirtyPages   int
}
