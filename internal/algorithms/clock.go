package algorithms

import (
	"fmt"
	"sync"

	"github.com/page-replacement-cow/pkg/models"
)

// CLOCK implements the CLOCK (Second Chance) page replacement algorithm
type CLOCK struct {
	name     string
	hand     int32 // Clock hand position
	mu       sync.RWMutex
}

// NewCLOCK creates a new CLOCK algorithm instance
func NewCLOCK() *CLOCK {
	return &CLOCK{
		name: "CLOCK",
		hand: 0,
	}
}

// SelectVictim selects a victim using the CLOCK algorithm
func (c *CLOCK) SelectVictim(frames []*models.Frame) (*models.Frame, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames available for eviction")
	}

	numFrames := int32(len(frames))
	iterations := 0
	maxIterations := numFrames * 2 // Prevent infinite loop

	for iterations < int(maxIterations) {
		// Wrap around
		if c.hand >= numFrames {
			c.hand = 0
		}

		frame := frames[c.hand]

		// Skip free or pinned frames
		if !frame.IsFree() && !frame.IsPinned() {
			// Check reference bit
			if frame.GetReferenceBit() {
				// Clear reference bit and give second chance
				frame.ClearReferenceBit()
			} else {
				// Found victim
				victim := frame
				c.hand = (c.hand + 1) % numFrames
				return victim, nil
			}
		}

		c.hand = (c.hand + 1) % numFrames
		iterations++

		// If we've wrapped around completely, just take the current frame
		if iterations >= int(maxIterations) {
			for i := int32(0); i < numFrames; i++ {
				if !frames[i].IsFree() && !frames[i].IsPinned() {
					c.hand = (i + 1) % numFrames
					return frames[i], nil
				}
			}
			return nil, fmt.Errorf("no evictable frame found")
		}
	}

	// Unreachable: the inner loop always returns above once all reference bits
	// have been cleared (worst case: two full sweeps). Required by the compiler.
	return nil, fmt.Errorf("no evictable frame found")
}

// OnPageAccess is called when a page is accessed
func (c *CLOCK) OnPageAccess(frame *models.Frame, write bool) {
	// Set reference bit
	frame.Access(write)
}

// OnPageFault is called when a page fault occurs
func (c *CLOCK) OnPageFault(frame *models.Frame) {
	// Set reference bit on page load
	if frame != nil {
		frame.Access(false)
	}
}

// OnPageEviction is called when a page is evicted
func (c *CLOCK) OnPageEviction(frame *models.Frame) {
	// Clear reference bit
	if frame != nil {
		frame.ClearReferenceBit()
	}
}

// GetName returns the name of the algorithm
func (c *CLOCK) GetName() string {
	return c.name
}

// Reset resets the algorithm state
func (c *CLOCK) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hand = 0
}

// GetHandPosition returns the current hand position (for visualization)
func (c *CLOCK) GetHandPosition() int32 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hand
}
