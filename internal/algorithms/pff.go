package algorithms

import (
	"fmt"
	"sync"
	"time"

	"github.com/page-replacement-cow/pkg/models"
)

type PFF struct {
	name            string
	mu              sync.RWMutex

	faultTimes      []time.Time
	windowDuration  time.Duration
	minFaultRate    float64
	maxFaultRate    float64
	targetResident  int32
	minResident     int32
	maxResident     int32
	adjustInterval  int32
	accessSinceAdjust int32
}

func NewPFF(windowMs int64, minRate, maxRate float64, minRes, maxRes, targetRes int32) *PFF {
	return &PFF{
		name:            "PFF",
		faultTimes:      make([]time.Time, 0, 256),
		windowDuration:  time.Duration(windowMs) * time.Millisecond,
		minFaultRate:    minRate,
		maxFaultRate:    maxRate,
		targetResident:  targetRes,
		minResident:     minRes,
		maxResident:     maxRes,
		adjustInterval:  10,
		accessSinceAdjust: 0,
	}
}

func (p *PFF) GetTargetResidentSet() int32 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.targetResident
}

func (p *PFF) GetFaultRate() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.computeFaultRate()
}

func (p *PFF) SelectVictim(frames []*models.Frame) (*models.Frame, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames available for eviction")
	}

	usedAndUnpinned := make([]*models.Frame, 0)
	for _, f := range frames {
		if !f.IsFree() && !f.IsPinned() {
			usedAndUnpinned = append(usedAndUnpinned, f)
		}
	}

	if len(usedAndUnpinned) == 0 {
		return nil, fmt.Errorf("no evictable frame found")
	}

	residentCount := int32(len(usedAndUnpinned))
	if p.accessSinceAdjust >= p.adjustInterval {
		p.adjustResidentSize(residentCount)
		p.accessSinceAdjust = 0
	}
	p.accessSinceAdjust++

	var victim *models.Frame
	var oldestTime time.Time
	first := true

	for _, f := range usedAndUnpinned {
		lastAccess := f.GetLastAccessTime()
		if first || lastAccess.Before(oldestTime) {
			victim = f
			oldestTime = lastAccess
			first = false
		}
	}

	return victim, nil
}

func (p *PFF) adjustResidentSize(currentResident int32) {
	faultRate := p.computeFaultRate()

	if faultRate > p.maxFaultRate && p.targetResident < p.maxResident {
		p.targetResident = min(p.targetResident+1, p.maxResident)
	}

	if faultRate < p.minFaultRate && p.targetResident > p.minResident {
		p.targetResident = max(p.targetResident-1, p.minResident)
	}

	if currentResident > p.targetResident+1 {
		p.targetResident = currentResident - 1
	}
}

func (p *PFF) computeFaultRate() float64 {
	now := time.Now()
	cutoff := now.Add(-p.windowDuration)

	valid := make([]time.Time, 0, len(p.faultTimes))
	for _, t := range p.faultTimes {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	p.faultTimes = valid

	windowSecs := p.windowDuration.Seconds()
	if windowSecs <= 0 {
		return 0
	}

	return float64(len(valid)) / windowSecs
}

func (p *PFF) recordFault() {
	now := time.Now()
	p.faultTimes = append(p.faultTimes, now)
	if len(p.faultTimes) > 512 {
		p.faultTimes = p.faultTimes[1:]
	}
}

func (p *PFF) OnPageAccess(frame *models.Frame, write bool) {
	frame.Access(write)
}

func (p *PFF) OnPageFault(frame *models.Frame) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.recordFault()
	if frame != nil {
		frame.Access(false)
	}
}

func (p *PFF) OnPageEviction(frame *models.Frame) {}

func (p *PFF) GetName() string { return p.name }

func (p *PFF) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.faultTimes = p.faultTimes[:0]
	p.accessSinceAdjust = 0
}

func (p *PFF) GetStats() PFFStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return PFFStats{
		TargetResident: p.targetResident,
		FaultRate:      p.computeFaultRate(),
		RecentFaults:   len(p.faultTimes),
	}
}

type PFFStats struct {
	TargetResident int32
	FaultRate      float64
	RecentFaults   int
}