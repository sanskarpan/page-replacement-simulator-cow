package algorithms

import (
	"fmt"
	"sync"

	"github.com/page-replacement-cow/pkg/models"
)

type carEntry struct {
	pageID    uint64
	frameID   int32
	refBit    bool
	key       int64
}

type CAR struct {
	name  string
	mu    sync.RWMutex

	t1       []*carEntry
	t2       []*carEntry
	b1       []uint64
	b2       []uint64

	t1Hand  int
	t2Hand  int
	t1Count int
	t2Count int

	p       int
	c       int

	index   int64
}

func NewCAR(numFrames int32) *CAR {
	cap := int(numFrames) * 4
	return &CAR{
		name:    "CAR",
		t1:      make([]*carEntry, 0, cap),
		t2:      make([]*carEntry, 0, cap),
		b1:      make([]uint64, 0, cap),
		b2:      make([]uint64, 0, cap),
		c:       int(numFrames),
		p:       int(numFrames) / 2,
		index:   0,
	}
}

func (c *CAR) SelectVictim(frames []*models.Frame) (*models.Frame, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.t1) > 0 && c.t1Count > 0 {
		for i := 0; i < len(c.t1)*2; i++ {
			c.t1Hand = c.t1Hand % len(c.t1)
			if c.t1Hand < 0 {
				c.t1Hand = 0
			}
			entry := c.t1[c.t1Hand]
			c.t1Hand++

			frame := c.findFrameByPageID(frames, entry.pageID)
			if frame == nil || frame.IsFree() || frame.IsPinned() {
				continue
			}

			if entry.refBit {
				entry.refBit = false
				c.t1Hand--
				c.moveT1ToT2(entry)
				continue
			}

			c.removeT1(entry)
			return frame, nil
		}
	}

	if len(c.t2) > 0 && c.t2Count > 0 {
		for i := 0; i < len(c.t2)*2; i++ {
			c.t2Hand = c.t2Hand % len(c.t2)
			if c.t2Hand < 0 {
				c.t2Hand = 0
			}
			entry := c.t2[c.t2Hand]
			c.t2Hand++

			frame := c.findFrameByPageID(frames, entry.pageID)
			if frame == nil || frame.IsFree() || frame.IsPinned() {
				continue
			}

			if entry.refBit {
				entry.refBit = false
				continue
			}

			c.removeT2(entry)
			return frame, nil
		}
	}

	for _, f := range frames {
		if !f.IsFree() && !f.IsPinned() {
			return f, nil
		}
	}
	return nil, fmt.Errorf("no evictable frame found")
}

func (c *CAR) OnPageAccess(frame *models.Frame, write bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	frame.Access(write)

	pageID := frame.GetPageID()
	if entry := c.findInT1(pageID); entry != nil {
		entry.refBit = true
		return
	}
	if entry := c.findInT2(pageID); entry != nil {
		entry.refBit = true
		return
	}

	entry := &carEntry{pageID: pageID, frameID: frame.ID, refBit: true, key: c.nextIndex()}
	if c.isInB1(pageID) {
		delta := 1
		if len(c.b1) >= len(c.b2) {
			delta = max(1, len(c.b1)/max(1, len(c.b2)))
		}
		c.p = min(c.p+delta, c.c)
		c.removeFromB1(pageID)
		c.adapt()
		c.pushT2(entry)
	} else if c.isInB2(pageID) {
		delta := 1
		if len(c.b2) >= len(c.b1) {
			delta = max(1, len(c.b2)/max(1, len(c.b1)))
		}
		c.p = max(c.p-delta, 0)
		c.removeFromB2(pageID)
		c.adapt()
		c.pushT2(entry)
	} else {
		c.pushT1(entry)
		c.adapt()
	}
}

func (c *CAR) OnPageFault(frame *models.Frame) {
	c.mu.Lock()
	defer c.mu.Unlock()
	frame.Access(false)

	pageID := frame.GetPageID()
	entry := &carEntry{pageID: pageID, frameID: frame.ID, refBit: true, key: c.nextIndex()}

	if c.isInB1(pageID) {
		c.removeFromB1(pageID)
		c.pushT2(entry)
	} else if c.isInB2(pageID) {
		c.removeFromB2(pageID)
		c.pushT2(entry)
	} else {
		c.pushT1(entry)
	}
	c.adapt()
}

func (c *CAR) adapt() {
	for c.t1Count > c.p {
		if len(c.t1) == 0 {
			break
		}
		c.moveT1ToB1()
	}

	for c.t1Count+c.t2Count > c.c {
		if c.t2Count > 0 {
			c.moveT2ToB2()
		} else if c.t1Count > 0 {
			c.moveT1ToB1()
		} else {
			break
		}
	}
}

func (c *CAR) pushT1(entry *carEntry) {
	c.t1 = append(c.t1, entry)
	c.t1Count++
}

func (c *CAR) pushT2(entry *carEntry) {
	c.t2 = append(c.t2, entry)
	c.t2Count++
}

func (c *CAR) moveT1ToT2(entry *carEntry) {
	c.removeT1(entry)
	c.t2 = append(c.t2, entry)
	c.t2Count++
}

func (c *CAR) moveT1ToB1() {
	for i := 0; i < len(c.t1); i++ {
		if c.t1[i] != nil {
			c.b1 = append(c.b1, c.t1[i].pageID)
			if len(c.b1) > c.c*2 {
				c.b1 = c.b1[1:]
			}
			c.t1 = append(c.t1[:i], c.t1[i+1:]...)
			c.t1Count--
			return
		}
	}
}

func (c *CAR) moveT2ToB2() {
	for i := 0; i < len(c.t2); i++ {
		if c.t2[i] != nil {
			c.b2 = append(c.b2, c.t2[i].pageID)
			if len(c.b2) > c.c*2 {
				c.b2 = c.b2[1:]
			}
			c.t2 = append(c.t2[:i], c.t2[i+1:]...)
			c.t2Count--
			return
		}
	}
}

func (c *CAR) removeT1(entry *carEntry) {
	for i, e := range c.t1 {
		if e.key == entry.key {
			c.t1 = append(c.t1[:i], c.t1[i+1:]...)
			c.t1Count--
			return
		}
	}
}

func (c *CAR) removeT2(entry *carEntry) {
	for i, e := range c.t2 {
		if e.key == entry.key {
			c.t2 = append(c.t2[:i], c.t2[i+1:]...)
			c.t2Count--
			return
		}
	}
}

func (c *CAR) findInT1(pageID uint64) *carEntry {
	for _, e := range c.t1 {
		if e.pageID == pageID {
			return e
		}
	}
	return nil
}

func (c *CAR) findInT2(pageID uint64) *carEntry {
	for _, e := range c.t2 {
		if e.pageID == pageID {
			return e
		}
	}
	return nil
}

func (c *CAR) isInB1(pageID uint64) bool {
	for _, pid := range c.b1 {
		if pid == pageID {
			return true
		}
	}
	return false
}

func (c *CAR) isInB2(pageID uint64) bool {
	for _, pid := range c.b2 {
		if pid == pageID {
			return true
		}
	}
	return false
}

func (c *CAR) removeFromB1(pageID uint64) {
	for i, pid := range c.b1 {
		if pid == pageID {
			c.b1 = append(c.b1[:i], c.b1[i+1:]...)
			return
		}
	}
}

func (c *CAR) removeFromB2(pageID uint64) {
	for i, pid := range c.b2 {
		if pid == pageID {
			c.b2 = append(c.b2[:i], c.b2[i+1:]...)
			return
		}
	}
}

func (c *CAR) findFrameByPageID(frames []*models.Frame, pageID uint64) *models.Frame {
	for _, f := range frames {
		if f.GetPageID() == pageID {
			return f
		}
	}
	return nil
}

func (c *CAR) nextIndex() int64 {
	c.index++
	return c.index
}

func (c *CAR) OnPageEviction(frame *models.Frame) {}

func (c *CAR) GetName() string { return c.name }

func (c *CAR) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t1 = make([]*carEntry, 0, c.c*4)
	c.t2 = make([]*carEntry, 0, c.c*4)
	c.b1 = make([]uint64, 0, c.c*4)
	c.b2 = make([]uint64, 0, c.c*4)
	c.t1Count, c.t2Count = 0, 0
	c.t1Hand, c.t2Hand = 0, 0
	c.index = 0
	c.p = c.c / 2
}