package memory

import "sync/atomic"

type PageTableEntry struct {
	FrameNumber int32
	Present     atomic.Bool
	Dirty       atomic.Bool
	HugePage    atomic.Bool
	ReadOnly    atomic.Bool
	UserAccess  atomic.Bool
}

func NewPageTableEntry() *PageTableEntry {
	entry := &PageTableEntry{FrameNumber: -1}
	entry.Present.Store(false)
	entry.Dirty.Store(false)
	entry.HugePage.Store(false)
	entry.ReadOnly.Store(false)
	entry.UserAccess.Store(true)
	return entry
}

const (
	PTEntriesPerLevel = 512
	L4Shift           = 39
	L3Shift           = 30
	L2Shift           = 21
	L1Shift           = 12
)

type Level4Table struct {
	entries [PTEntriesPerLevel]*Level3Table
}

type Level3Table struct {
	entries [PTEntriesPerLevel]*Level2Table
}

type Level2Table struct {
	entries         [PTEntriesPerLevel]*Level1Table
	hugePageEntries [PTEntriesPerLevel]*PageTableEntry
	isHuge          [PTEntriesPerLevel]bool
}

type Level1Table struct {
	entries [PTEntriesPerLevel]*PageTableEntry
}

type MultiLevelPageTable struct {
	ProcessID string
	l4Table   *Level4Table
}

func NewMultiLevelPageTable(processID string) *MultiLevelPageTable {
	return &MultiLevelPageTable{
		ProcessID: processID,
		l4Table:   &Level4Table{},
	}
}

func (mlpt *MultiLevelPageTable) indices(virtualAddr uint64) (int, int, int, int) {
	l4 := int((virtualAddr >> L4Shift) & 0x1FF)
	l3 := int((virtualAddr >> L3Shift) & 0x1FF)
	l2 := int((virtualAddr >> L2Shift) & 0x1FF)
	l1 := int((virtualAddr >> L1Shift) & 0x1FF)
	return l4, l3, l2, l1
}

func (mlpt *MultiLevelPageTable) GetEntry(virtualAddr uint64) *PageTableEntry {
	l4, l3, l2, l1 := mlpt.indices(virtualAddr)

	l3Table := mlpt.l4Table.entries[l4]
	if l3Table == nil {
		return nil
	}

	l2Table := l3Table.entries[l3]
	if l2Table == nil {
		return nil
	}

	if l2Table.isHuge[l2] {
		return l2Table.hugePageEntries[l2]
	}

	l1Table := l2Table.entries[l2]
	if l1Table == nil {
		return nil
	}

	return l1Table.entries[l1]
}

func (mlpt *MultiLevelPageTable) SetEntry(virtualAddr uint64, frameNumber int32, huge bool) *PageTableEntry {
	l4, l3, l2, l1 := mlpt.indices(virtualAddr)

	if mlpt.l4Table.entries[l4] == nil {
		mlpt.l4Table.entries[l4] = &Level3Table{}
	}
	l3Table := mlpt.l4Table.entries[l4]

	if l3Table.entries[l3] == nil {
		l3Table.entries[l3] = &Level2Table{}
	}
	l2Table := l3Table.entries[l3]

	if huge {
		if l2Table.hugePageEntries[l2] == nil {
			l2Table.hugePageEntries[l2] = NewPageTableEntry()
		}
		entry := l2Table.hugePageEntries[l2]
		entry.FrameNumber = frameNumber
		entry.Present.Store(true)
		entry.HugePage.Store(true)
		l2Table.isHuge[l2] = true
		return entry
	}

	if l2Table.entries[l2] == nil {
		l2Table.entries[l2] = &Level1Table{}
	}
	l1Table := l2Table.entries[l2]

	if l1Table.entries[l1] == nil {
		l1Table.entries[l1] = NewPageTableEntry()
	}
	entry := l1Table.entries[l1]
	entry.FrameNumber = frameNumber
	entry.Present.Store(true)
	return entry
}

func (mlpt *MultiLevelPageTable) InvalidateEntry(virtualAddr uint64) {
	l4, l3, l2, l1 := mlpt.indices(virtualAddr)

	l3Table := mlpt.l4Table.entries[l4]
	if l3Table == nil {
		return
	}
	l2Table := l3Table.entries[l3]
	if l2Table == nil {
		return
	}

	if l2Table.isHuge[l2] {
		if l2Table.hugePageEntries[l2] != nil {
			l2Table.hugePageEntries[l2].Present.Store(false)
			l2Table.hugePageEntries[l2].FrameNumber = -1
		}
		return
	}

	l1Table := l2Table.entries[l2]
	if l1Table == nil {
		return
	}
	if l1Table.entries[l1] != nil {
		l1Table.entries[l1].Present.Store(false)
		l1Table.entries[l1].FrameNumber = -1
	}
}

func (mlpt *MultiLevelPageTable) WalkPages(fn func(virtualAddr uint64, entry *PageTableEntry, huge bool)) {
	for l4 := 0; l4 < PTEntriesPerLevel; l4++ {
		l3Table := mlpt.l4Table.entries[l4]
		if l3Table == nil {
			continue
		}
		for l3 := 0; l3 < PTEntriesPerLevel; l3++ {
			l2Table := l3Table.entries[l3]
			if l2Table == nil {
				continue
			}
			for l2 := 0; l2 < PTEntriesPerLevel; l2++ {
				if l2Table.isHuge[l2] && l2Table.hugePageEntries[l2] != nil && l2Table.hugePageEntries[l2].Present.Load() {
					addr := (uint64(l4) << L4Shift) | (uint64(l3) << L3Shift) | (uint64(l2) << L2Shift)
					fn(addr, l2Table.hugePageEntries[l2], true)
					continue
				}

				l1Table := l2Table.entries[l2]
				if l1Table == nil {
					continue
				}
				for l1 := 0; l1 < PTEntriesPerLevel; l1++ {
					entry := l1Table.entries[l1]
					if entry != nil && entry.Present.Load() {
						addr := (uint64(l4) << L4Shift) | (uint64(l3) << L3Shift) | (uint64(l2) << L2Shift) | (uint64(l1) << L1Shift)
						fn(addr, entry, false)
					}
				}
			}
		}
	}
}