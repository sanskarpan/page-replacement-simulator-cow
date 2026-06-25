package memory

import (
	"fmt"
	"sync"

	"github.com/page-replacement-cow/pkg/models"
)

// FrameTable manages physical memory frames
type FrameTable struct {
	Frames    []*models.Frame
	TotalSize int32
	mu        sync.RWMutex
}

// NewFrameTable creates a new frame table
func NewFrameTable(numFrames int32) *FrameTable {
	frames := make([]*models.Frame, numFrames)
	for i := int32(0); i < numFrames; i++ {
		frames[i] = models.NewFrame(i)
	}

	return &FrameTable{
		Frames:    frames,
		TotalSize: numFrames,
	}
}

// GetFrame returns a frame by ID
func (ft *FrameTable) GetFrame(frameID int32) (*models.Frame, error) {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	if frameID < 0 || frameID >= ft.TotalSize {
		return nil, fmt.Errorf("invalid frame ID: %d", frameID)
	}

	return ft.Frames[frameID], nil
}

// AllocateFrame finds and allocates a free frame
func (ft *FrameTable) AllocateFrame(pageID uint64, processID string) (*models.Frame, error) {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	// Find a free frame
	for _, frame := range ft.Frames {
		if frame.IsFree() {
			frame.Allocate(pageID, processID)
			return frame, nil
		}
	}

	return nil, fmt.Errorf("no free frames available")
}

// ReleaseFrame releases a frame
func (ft *FrameTable) ReleaseFrame(frameID int32) error {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	if frameID < 0 || frameID >= ft.TotalSize {
		return fmt.Errorf("invalid frame ID: %d", frameID)
	}

	ft.Frames[frameID].Release()
	return nil
}

// GetAllFrames returns all frames
func (ft *FrameTable) GetAllFrames() []*models.Frame {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	frames := make([]*models.Frame, len(ft.Frames))
	copy(frames, ft.Frames)
	return frames
}

// GetFreeFrames returns all free frames
func (ft *FrameTable) GetFreeFrames() []*models.Frame {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	freeFrames := make([]*models.Frame, 0)
	for _, frame := range ft.Frames {
		if frame.IsFree() {
			freeFrames = append(freeFrames, frame)
		}
	}
	return freeFrames
}

// GetUsedFrames returns all used frames
func (ft *FrameTable) GetUsedFrames() []*models.Frame {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	usedFrames := make([]*models.Frame, 0)
	for _, frame := range ft.Frames {
		if !frame.IsFree() {
			usedFrames = append(usedFrames, frame)
		}
	}
	return usedFrames
}

// GetFreeFrameCount returns the number of free frames
func (ft *FrameTable) GetFreeFrameCount() int32 {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	count := int32(0)
	for _, frame := range ft.Frames {
		if frame.IsFree() {
			count++
		}
	}
	return count
}

// GetUsedFrameCount returns the number of used frames
func (ft *FrameTable) GetUsedFrameCount() int32 {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	count := int32(0)
	for _, frame := range ft.Frames {
		if !frame.IsFree() {
			count++
		}
	}
	return count
}

// FindFrameByPage finds the frame containing a specific page
func (ft *FrameTable) FindFrameByPage(pageID uint64) (*models.Frame, error) {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	for _, frame := range ft.Frames {
		if !frame.IsFree() && frame.GetPageID() == pageID {
			return frame, nil
		}
	}

	return nil, fmt.Errorf("frame not found for page %d", pageID)
}

// GetStats returns statistics about the frame table
func (ft *FrameTable) GetStats() FrameTableStats {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	stats := FrameTableStats{
		TotalFrames:  int32(len(ft.Frames)),
		FreeFrames:   0,
		UsedFrames:   0,
		PinnedFrames: 0,
		DirtyFrames:  0,
	}

	for _, frame := range ft.Frames {
		if frame.IsFree() {
			stats.FreeFrames++
		} else {
			stats.UsedFrames++
			if frame.IsPinned() {
				stats.PinnedFrames++
			}
			if frame.IsModified() {
				stats.DirtyFrames++
			}
		}
	}

	return stats
}

// FrameTableStats contains statistics about the frame table
type FrameTableStats struct {
	TotalFrames  int32
	FreeFrames   int32
	UsedFrames   int32
	PinnedFrames int32
	DirtyFrames  int32
}

// Clear releases all frames
func (ft *FrameTable) Clear() {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	for _, frame := range ft.Frames {
		frame.Release()
	}
}
