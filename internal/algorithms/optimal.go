package algorithms

import (
	"fmt"
	"sync"

	"github.com/page-replacement-cow/pkg/models"
)

// Optimal implements the Optimal (Belady's) page replacement algorithm
// This is used for comparison purposes in simulations
// It requires future knowledge of page references
type Optimal struct {
	name            string
	futureAccesses  map[uint64][]int // Page ID -> list of future access indices
	currentIndex    int
	mu              sync.RWMutex
}

// NewOptimal creates a new Optimal algorithm instance
func NewOptimal() *Optimal {
	return &Optimal{
		name:           "Optimal",
		futureAccesses: make(map[uint64][]int),
		currentIndex:   0,
	}
}

// SetFutureAccesses sets the future access pattern for optimal prediction
func (opt *Optimal) SetFutureAccesses(accesses []uint64) {
	opt.mu.Lock()
	defer opt.mu.Unlock()

	opt.futureAccesses = make(map[uint64][]int)
	for i, pageID := range accesses {
		opt.futureAccesses[pageID] = append(opt.futureAccesses[pageID], i)
	}
	opt.currentIndex = 0
}

// SelectVictim selects the page that will not be used for the longest time
func (opt *Optimal) SelectVictim(frames []*models.Frame) (*models.Frame, error) {
	opt.mu.RLock()
	defer opt.mu.RUnlock()

	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames available for eviction")
	}

	var victim *models.Frame
	var farthestDistance int = -1

	for _, frame := range frames {
		// Skip free or pinned frames
		if frame.IsFree() || frame.IsPinned() {
			continue
		}

		pageID := frame.GetPageID()
		distance := opt.getNextAccessDistance(pageID)

		if distance > farthestDistance {
			victim = frame
			farthestDistance = distance
		}
	}

	if victim == nil {
		return nil, fmt.Errorf("no evictable frame found")
	}

	return victim, nil
}

// getNextAccessDistance returns the distance to next access
// Returns very large number if page will never be accessed again
func (opt *Optimal) getNextAccessDistance(pageID uint64) int {
	accessList, exists := opt.futureAccesses[pageID]
	if !exists {
		return 1000000 // Never accessed again
	}

	// Find next access after current index
	for _, accessIndex := range accessList {
		if accessIndex > opt.currentIndex {
			return accessIndex - opt.currentIndex
		}
	}

	// No future accesses
	return 1000000
}

// OnPageAccess is called when a page is accessed
func (opt *Optimal) OnPageAccess(frame *models.Frame, write bool) {
	opt.mu.Lock()
	defer opt.mu.Unlock()

	// Advance current index
	opt.currentIndex++
}

// OnPageFault is called when a page fault occurs
func (opt *Optimal) OnPageFault(frame *models.Frame) {
	opt.mu.Lock()
	defer opt.mu.Unlock()

	// Advance current index
	opt.currentIndex++
}

// OnPageEviction is called when a page is evicted
func (opt *Optimal) OnPageEviction(frame *models.Frame) {
	// No special handling needed
}

// GetName returns the name of the algorithm
func (opt *Optimal) GetName() string {
	return opt.name
}

// Reset resets the algorithm state
func (opt *Optimal) Reset() {
	opt.mu.Lock()
	defer opt.mu.Unlock()

	opt.currentIndex = 0
	opt.futureAccesses = make(map[uint64][]int)
}
