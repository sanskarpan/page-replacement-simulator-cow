package algorithms

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/page-replacement-cow/pkg/models"
)

// NRU (Not Recently Used) classifies pages into 4 classes by their Referenced
// (R) and Modified (M) bits and evicts a uniform-random page from the lowest
// non-empty class. R bits are cleared every clearPeriod accesses to simulate
// a hardware clock interrupt aging out recently-used pages.
//
//	Class 0  R=0, M=0  — best candidate for eviction
//	Class 1  R=0, M=1
//	Class 2  R=1, M=0
//	Class 3  R=1, M=1  — worst candidate for eviction
type NRU struct {
	name        string
	mu          sync.Mutex // protects rng and accessCount; prevents TOCTOU on clearPeriod check-and-store
	accessCount int64      // always accessed under mu
	clearPeriod int64
	rng         *rand.Rand
}

func NewNRU() *NRU {
	return &NRU{
		name:        "NRU",
		clearPeriod: 50,
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (n *NRU) SelectVictim(frames []*models.Frame) (*models.Frame, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Periodic clock tick: clear all R bits to give cold pages a second chance.
	if n.accessCount >= n.clearPeriod {
		n.accessCount = 0
		for _, f := range frames {
			if !f.IsFree() {
				f.ClearReferenceBit()
			}
		}
	}

	var classes [4][]*models.Frame
	for _, f := range frames {
		if f.IsFree() || f.IsPinned() {
			continue
		}
		class := 0
		if f.GetReferenceBit() {
			class += 2
		}
		if f.IsModified() {
			class += 1
		}
		classes[class] = append(classes[class], f)
	}

	for cls := 0; cls < 4; cls++ {
		if len(classes[cls]) > 0 {
			return classes[cls][n.rng.Intn(len(classes[cls]))], nil
		}
	}
	return nil, fmt.Errorf("no evictable frame found")
}

func (n *NRU) OnPageAccess(frame *models.Frame, write bool) {
	frame.Access(write)
	n.mu.Lock()
	n.accessCount++
	n.mu.Unlock()
}

func (n *NRU) OnPageFault(frame *models.Frame) {
	if frame != nil {
		frame.Access(false)
		n.mu.Lock()
		n.accessCount++
		n.mu.Unlock()
	}
}

func (n *NRU) OnPageEviction(frame *models.Frame) {
	if frame != nil {
		frame.ClearReferenceBit()
	}
}

func (n *NRU) GetName() string { return n.name }

func (n *NRU) Reset() {
	n.mu.Lock()
	n.accessCount = 0
	n.mu.Unlock()
}
