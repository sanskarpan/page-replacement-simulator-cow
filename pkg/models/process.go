package models

import (
	"sync"
	"sync/atomic"
	"time"
)

// ProcessState represents the state of a process
type ProcessState int32

const (
	ProcessReady ProcessState = iota
	ProcessRunning
	ProcessBlocked
	ProcessTerminated
)

// Process represents a process in the system
type Process struct {
	ID          string
	Name        string
	Priority    int32
	// json:"-" on atomic fields prevents the encoder from emitting raw
	// atomic struct internals; use marshalProcess (in pkg/api) or Load()
	// accessors when serializing.
	State       atomic.Int32 `json:"-"`

	// Virtual memory
	VirtualPages   uint64 // Number of virtual pages
	PageTableSize  uint64 // Size of page table
	WorkingSetSize int32  // Current working set size

	// Statistics
	PageFaults     atomic.Int64 `json:"-"`
	PageHits       atomic.Int64 `json:"-"`
	MemoryAccesses atomic.Int64 `json:"-"`
	CoWCopies      atomic.Int64 `json:"-"` // Number of CoW copies made

	// Timing
	CreatedAt time.Time
	CPUTime   atomic.Int64 `json:"-"` // Nanoseconds

	// Parent process (for fork/CoW)
	ParentID string
	Children []string

	mu sync.RWMutex
}

// NewProcess creates a new process
func NewProcess(id, name string, priority int32, virtualPages uint64) *Process {
	p := &Process{
		ID:            id,
		Name:          name,
		Priority:      priority,
		VirtualPages:  virtualPages,
		PageTableSize: virtualPages,
		CreatedAt:     time.Now(),
		Children:      make([]string, 0),
	}
	p.State.Store(int32(ProcessReady))
	p.PageFaults.Store(0)
	p.PageHits.Store(0)
	p.MemoryAccesses.Store(0)
	p.CoWCopies.Store(0)
	p.CPUTime.Store(0)
	p.WorkingSetSize = 0
	return p
}

// RecordPageFault records a page fault
func (p *Process) RecordPageFault() {
	p.PageFaults.Add(1)
}

// RecordPageHit records a page hit
func (p *Process) RecordPageHit() {
	p.PageHits.Add(1)
}

// RecordMemoryAccess records a memory access
func (p *Process) RecordMemoryAccess() {
	p.MemoryAccesses.Add(1)
}

// RecordCoWCopy records a copy-on-write operation
func (p *Process) RecordCoWCopy() {
	p.CoWCopies.Add(1)
}

// GetPageFaultRate returns the page fault rate (0.0 to 1.0)
func (p *Process) GetPageFaultRate() float64 {
	accesses := p.MemoryAccesses.Load()
	if accesses == 0 {
		return 0.0
	}
	faults := p.PageFaults.Load()
	return float64(faults) / float64(accesses)
}

// GetPageHitRate returns the page hit rate (0.0 to 1.0)
func (p *Process) GetPageHitRate() float64 {
	accesses := p.MemoryAccesses.Load()
	if accesses == 0 {
		return 0.0
	}
	hits := p.PageHits.Load()
	return float64(hits) / float64(accesses)
}

// GetState returns the current process state
func (p *Process) GetState() ProcessState {
	return ProcessState(p.State.Load())
}

// SetState sets the process state
func (p *Process) SetState(state ProcessState) {
	p.State.Store(int32(state))
}

// AddChild adds a child process (for fork)
func (p *Process) AddChild(childID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Children = append(p.Children, childID)
}

// RemoveChild removes a child process ID (for fork rollback)
func (p *Process) RemoveChild(childID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, id := range p.Children {
		if id == childID {
			p.Children = append(p.Children[:i], p.Children[i+1:]...)
			return
		}
	}
}

// GetChildren returns all child process IDs
func (p *Process) GetChildren() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	children := make([]string, len(p.Children))
	copy(children, p.Children)
	return children
}

// GetStateString returns a string representation of the state
func (p *Process) GetStateString() string {
	state := ProcessState(p.State.Load())
	switch state {
	case ProcessReady:
		return "Ready"
	case ProcessRunning:
		return "Running"
	case ProcessBlocked:
		return "Blocked"
	case ProcessTerminated:
		return "Terminated"
	default:
		return "Unknown"
	}
}

// UpdateWorkingSetSize updates the working set size
func (p *Process) UpdateWorkingSetSize(size int32) {
	atomic.StoreInt32(&p.WorkingSetSize, size)
}

// GetWorkingSetSize returns the current working set size
func (p *Process) GetWorkingSetSize() int32 {
	return atomic.LoadInt32(&p.WorkingSetSize)
}
