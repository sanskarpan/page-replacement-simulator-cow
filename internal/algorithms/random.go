package algorithms

import (
	"fmt"
	"math/rand"
	"sync"

	"github.com/page-replacement-cow/pkg/models"
)

type Random struct {
	name string
	rng  *rand.Rand
	mu   sync.RWMutex
}

func NewRandom() *Random {
	return &Random{
		name: "Random",
		rng:  rand.New(rand.NewSource(rand.Int63())),
	}
}

func (r *Random) SelectVictim(frames []*models.Frame) (*models.Frame, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames available for eviction")
	}

	evictable := make([]*models.Frame, 0, len(frames))
	for _, frame := range frames {
		if !frame.IsFree() && !frame.IsPinned() {
			evictable = append(evictable, frame)
		}
	}

	if len(evictable) == 0 {
		return nil, fmt.Errorf("no evictable frame found")
	}

	idx := r.rng.Intn(len(evictable))
	return evictable[idx], nil
}

func (r *Random) OnPageAccess(frame *models.Frame, write bool) {
	frame.Access(write)
}

func (r *Random) OnPageFault(frame *models.Frame) {
	if frame != nil {
		frame.Access(false)
	}
}

func (r *Random) OnPageEviction(frame *models.Frame) {
}

func (r *Random) GetName() string {
	return r.name
}

func (r *Random) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
}