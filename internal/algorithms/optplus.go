package algorithms

import (
	"fmt"
	"sync"

	"github.com/page-replacement-cow/pkg/models"
)

type OptPlus struct {
	name           string
	futureAccesses map[uint64][]int
	futureWrites   map[uint64][]bool
	currentIndex   int
	mu             sync.RWMutex
}

func NewOptPlus() *OptPlus {
	return &OptPlus{
		name:           "OPT+",
		futureAccesses: make(map[uint64][]int),
		futureWrites:   make(map[uint64][]bool),
		currentIndex:   0,
	}
}

func (op *OptPlus) SetFutureAccesses(accesses []uint64) {
	op.mu.Lock()
	defer op.mu.Unlock()

	op.futureAccesses = make(map[uint64][]int)
	op.futureWrites = make(map[uint64][]bool)
	for i, pageID := range accesses {
		op.futureAccesses[pageID] = append(op.futureAccesses[pageID], i)
		op.futureWrites[pageID] = append(op.futureWrites[pageID], false)
	}
	op.currentIndex = 0
}

func (op *OptPlus) SetFutureAccessesWithWrites(accesses []uint64, writes []bool) {
	op.mu.Lock()
	defer op.mu.Unlock()

	op.futureAccesses = make(map[uint64][]int)
	op.futureWrites = make(map[uint64][]bool)
	for i, pageID := range accesses {
		op.futureAccesses[pageID] = append(op.futureAccesses[pageID], i)
		write := false
		if i < len(writes) {
			write = writes[i]
		}
		op.futureWrites[pageID] = append(op.futureWrites[pageID], write)
	}
	op.currentIndex = 0
}

func (op *OptPlus) SelectVictim(frames []*models.Frame) (*models.Frame, error) {
	op.mu.RLock()
	defer op.mu.RUnlock()

	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames available for eviction")
	}

	var victim *models.Frame
	bestScore := int64(-1 << 62)

	for _, frame := range frames {
		if frame.IsFree() || frame.IsPinned() {
			continue
		}

		pageID := frame.GetPageID()
		score := op.computeCostScore(pageID, frame.IsModified())

		if score > bestScore {
			victim = frame
			bestScore = score
		}
	}

	if victim == nil {
		return nil, fmt.Errorf("no evictable frame found")
	}

	return victim, nil
}

func (op *OptPlus) computeCostScore(pageID uint64, isDirty bool) int64 {
	distance := op.getNextAccessDistance(pageID)
	writePenalty := int64(0)
	if isDirty {
		writePenalty = 100
	}

	if distance >= 1000000 {
		return 1000000 + int64(distance) - writePenalty
	}

	return int64(distance) - writePenalty
}

func (op *OptPlus) getNextAccessDistance(pageID uint64) int {
	accessList, exists := op.futureAccesses[pageID]
	if !exists || len(accessList) == 0 {
		return 1000000
	}

	for _, accessIndex := range accessList {
		if accessIndex > op.currentIndex {
			return accessIndex - op.currentIndex
		}
	}

	return 1000000
}

func (op *OptPlus) nextWriteIsRead(pageID uint64) bool {
	accessList, exists := op.futureAccesses[pageID]
	if !exists || len(accessList) == 0 {
		return true
	}

	writeList, wExists := op.futureWrites[pageID]

	for i, accessIndex := range accessList {
		if accessIndex > op.currentIndex {
			if wExists && i < len(writeList) && writeList[i] {
				return false
			}
			return true
		}
	}

	return true
}

func (op *OptPlus) OnPageAccess(frame *models.Frame, write bool) {
	op.mu.Lock()
	defer op.mu.Unlock()
	op.currentIndex++
}

func (op *OptPlus) OnPageFault(frame *models.Frame) {
	// currentIndex is advanced only in OnPageAccess to avoid double-increment
}

func (op *OptPlus) OnPageEviction(frame *models.Frame) {}

func (op *OptPlus) GetName() string { return op.name }

func (op *OptPlus) Reset() {
	op.mu.Lock()
	defer op.mu.Unlock()
	op.currentIndex = 0
	op.futureAccesses = make(map[uint64][]int)
	op.futureWrites = make(map[uint64][]bool)
}