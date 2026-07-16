package algorithms

import (
	"container/list"
	"fmt"
	"sync"

	"github.com/page-replacement-cow/pkg/models"
)

type arcEntry struct {
	pageID    uint64
	frameID   int32
	key       int64
}

type ARC struct {
	name string
	mu   sync.RWMutex

	t1     *list.List
	t2     *list.List
	b1     *list.List
	b2     *list.List

	t1Size int
	t2Size int
	b1Size int
	b2Size int

	p      int
	c      int
	index  int64
}

func NewARC(numFrames int32) *ARC {
	return &ARC{
		name:   "ARC",
		t1:     list.New(),
		t2:     list.New(),
		b1:     list.New(),
		b2:     list.New(),
		c:      int(numFrames),
		p:      0,
		index:  0,
	}
}

func (a *ARC) getIndex() int64 {
	a.index++
	return a.index
}

func (a *ARC) SelectVictim(frames []*models.Frame) (*models.Frame, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	victimFrame, victimEntry := a.findVictim(frames)
	if victimFrame == nil {
		return nil, fmt.Errorf("no evictable frame found")
	}

	pageID := victimFrame.GetPageID()
	if victimEntry != nil {
		if a.inT2(victimEntry) {
			a.moveToB2(victimEntry)
		} else {
			a.moveToB1(victimEntry)
		}
	} else {
		// Clean up any stale T1/T2 entry for this pageID before recording B1 eviction.
		for e := a.t1.Front(); e != nil; e = e.Next() {
			if entry, ok := e.Value.(*arcEntry); ok && entry.pageID == pageID {
				a.t1.Remove(e)
				a.t1Size--
				break
			}
		}
		for e := a.t2.Front(); e != nil; e = e.Next() {
			if entry, ok := e.Value.(*arcEntry); ok && entry.pageID == pageID {
				a.t2.Remove(e)
				a.t2Size--
				break
			}
		}
		a.b1.PushFront(&arcEntry{pageID: pageID, frameID: victimFrame.ID, key: a.getIndex()})
		a.b1Size++
		if a.b1Size > a.c {
			a.b1.Remove(a.b1.Back())
			a.b1Size--
		}
	}

	return victimFrame, nil
}

func (a *ARC) findVictim(frames []*models.Frame) (*models.Frame, *arcEntry) {
	if a.t1.Len() > 0 {
		for e := a.t1.Back(); e != nil; e = e.Prev() {
			entry := e.Value.(*arcEntry)
			for _, f := range frames {
				if f.GetPageID() == entry.pageID && !f.IsFree() && !f.IsPinned() {
					return f, entry
				}
			}
		}
	}
	if a.t2.Len() > 0 {
		for e := a.t2.Back(); e != nil; e = e.Prev() {
			entry := e.Value.(*arcEntry)
			for _, f := range frames {
				if f.GetPageID() == entry.pageID && !f.IsFree() && !f.IsPinned() {
					return f, entry
				}
			}
		}
	}
	for _, f := range frames {
		if !f.IsFree() && !f.IsPinned() {
			return f, nil
		}
	}
	return nil, nil
}

func (a *ARC) OnPageAccess(frame *models.Frame, write bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	pageID := frame.GetPageID()
	entry := a.findEntryInT1(pageID)
	if entry != nil {
		val := entry.Value.(*arcEntry)
		a.t1.Remove(entry)
		a.t1Size--
		a.t2.PushFront(val)
		a.t2Size++
		frame.Access(write)
		return
	}

	entry = a.findEntryInT2(pageID)
	if entry != nil {
		a.t2.MoveToFront(entry)
		frame.Access(write)
		return
	}

	entry = a.findEntryInB1(pageID)
	if entry != nil {
		val := entry.Value.(*arcEntry)
		delta := 1
		if a.b1Size >= a.b2Size && a.b2Size > 0 {
			delta = a.b1Size / a.b2Size
		}
		if delta < 1 {
			delta = 1
		}
		a.p = min(a.p+delta, a.c)
		a.b1.Remove(entry)
		a.b1Size--
		a.adapt()
		a.t2.PushFront(val)
		a.t2Size++
		frame.Access(write)
		return
	}

	entry = a.findEntryInB2(pageID)
	if entry != nil {
		val := entry.Value.(*arcEntry)
		delta := 1
		if a.b2Size >= a.b1Size && a.b1Size > 0 {
			delta = a.b2Size / a.b1Size
		}
		if delta < 1 {
			delta = 1
		}
		a.p = max(a.p-delta, 0)
		a.b2.Remove(entry)
		a.b2Size--
		a.adapt()
		a.t2.PushFront(val)
		a.t2Size++
		frame.Access(write)
		return
	}

	val := &arcEntry{pageID: pageID, frameID: frame.ID, key: a.getIndex()}
	a.t1.PushFront(val)
	a.t1Size++
	a.adapt()
	frame.Access(write)
}

func (a *ARC) OnPageFault(frame *models.Frame) {
	a.mu.Lock()
	defer a.mu.Unlock()

	pageID := frame.GetPageID()

	b1Entry := a.findEntryInB1(pageID)
	if b1Entry != nil {
		entry := b1Entry.Value.(*arcEntry)
		a.b1.Remove(b1Entry)
		a.b1Size--
		a.t2.PushFront(entry)
		a.t2Size++
		frame.Access(false)
		return
	}

	b2Entry := a.findEntryInB2(pageID)
	if b2Entry != nil {
		entry := b2Entry.Value.(*arcEntry)
		a.b2.Remove(b2Entry)
		a.b2Size--
		a.t2.PushFront(entry)
		a.t2Size++
		frame.Access(false)
		return
	}

	a.t1.PushFront(&arcEntry{pageID: pageID, frameID: frame.ID, key: a.getIndex()})
	a.t1Size++
	a.adapt()
	frame.Access(false)
}

func (a *ARC) adapt() {
	for a.t1Size > a.p {
		if a.t1.Len() == 0 {
			break
		}
		entry := a.t1.Back().Value.(*arcEntry)
		a.t1.Remove(a.t1.Back())
		a.t1Size--
		a.b1.PushFront(entry)
		a.b1Size++
		if a.b1Size > a.c {
			a.b1.Remove(a.b1.Back())
			a.b1Size--
		}
	}

	total := a.t1Size + a.t2Size
	for total > a.c {
		if a.t2.Len() > 0 && a.t2Size > 0 {
			entry := a.t2.Back().Value.(*arcEntry)
			a.t2.Remove(a.t2.Back())
			a.t2Size--
			a.b2.PushFront(entry)
			a.b2Size++
			if a.b2Size > a.c {
				a.b2.Remove(a.b2.Back())
				a.b2Size--
			}
		}
		total = a.t1Size + a.t2Size
		if total <= a.c {
			break
		}
	}
}

func (a *ARC) OnPageEviction(frame *models.Frame) {}

func (a *ARC) GetName() string { return a.name }

func (a *ARC) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.t1 = list.New()
	a.t2 = list.New()
	a.b1 = list.New()
	a.b2 = list.New()
	a.t1Size, a.t2Size, a.b1Size, a.b2Size = 0, 0, 0, 0
	a.p = 0
	a.index = 0
}

func (a *ARC) findEntryInT1(pageID uint64) *list.Element {
	for e := a.t1.Front(); e != nil; e = e.Next() {
		if e.Value.(*arcEntry).pageID == pageID {
			return e
		}
	}
	return nil
}

func (a *ARC) findEntryInT2(pageID uint64) *list.Element {
	for e := a.t2.Front(); e != nil; e = e.Next() {
		if e.Value.(*arcEntry).pageID == pageID {
			return e
		}
	}
	return nil
}

func (a *ARC) findEntryInB1(pageID uint64) *list.Element {
	for e := a.b1.Front(); e != nil; e = e.Next() {
		if e.Value.(*arcEntry).pageID == pageID {
			return e
		}
	}
	return nil
}

func (a *ARC) findEntryInB2(pageID uint64) *list.Element {
	for e := a.b2.Front(); e != nil; e = e.Next() {
		if e.Value.(*arcEntry).pageID == pageID {
			return e
		}
	}
	return nil
}

func (a *ARC) inT2(entry *arcEntry) bool {
	for e := a.t2.Front(); e != nil; e = e.Next() {
		v := e.Value.(*arcEntry)
		if v.key == entry.key {
			return true
		}
	}
	return false
}

func (a *ARC) moveToB1(entry *arcEntry) {
	if a.t1.Len() > 0 {
		for e := a.t1.Front(); e != nil; e = e.Next() {
			if e.Value.(*arcEntry).key == entry.key {
				a.t1.Remove(e)
				a.t1Size--
				break
			}
		}
	}
	a.b1.PushFront(entry)
	a.b1Size++
	if a.b1Size > a.c {
		a.b1.Remove(a.b1.Back())
		a.b1Size--
	}
}

func (a *ARC) moveToB2(entry *arcEntry) {
	if a.t2.Len() > 0 {
		for e := a.t2.Front(); e != nil; e = e.Next() {
			if e.Value.(*arcEntry).key == entry.key {
				a.t2.Remove(e)
				a.t2Size--
				break
			}
		}
	}
	a.b2.PushFront(entry)
	a.b2Size++
	if a.b2Size > a.c {
		a.b2.Remove(a.b2.Back())
		a.b2Size--
	}
}