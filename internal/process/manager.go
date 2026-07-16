// Package process manages the lifecycle of simulated OS processes: creation,
// termination, forking (with Copy-on-Write semantics), and memory-access
// dispatch to the MemoryManager.
package process

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/pkg/models"
)

// ProcessManager manages processes and coordinates with memory manager
type ProcessManager struct {
	processes     map[string]*models.Process
	memoryManager *memory.MemoryManager
	nextPID       atomic.Uint64
	mu            sync.RWMutex
}

// NewProcessManager creates a new process manager
func NewProcessManager(memoryManager *memory.MemoryManager) *ProcessManager {
	pm := &ProcessManager{
		processes:     make(map[string]*models.Process),
		memoryManager: memoryManager,
	}
	pm.nextPID.Store(1)
	return pm
}

// CreateProcess creates a new process
func (pm *ProcessManager) CreateProcess(name string, priority int32, virtualPages uint64) (*models.Process, error) {
	pid := fmt.Sprintf("P%d", pm.nextPID.Add(1))

	process := models.NewProcess(pid, name, priority, virtualPages)

	pm.mu.Lock()
	pm.processes[pid] = process
	pm.mu.Unlock()

	// Register with memory manager
	if err := pm.memoryManager.CreateProcess(process); err != nil {
		pm.mu.Lock()
		delete(pm.processes, pid)
		pm.mu.Unlock()
		return nil, err
	}

	return process, nil
}

// TerminateProcess terminates a process
func (pm *ProcessManager) TerminateProcess(pid string) error {
	pm.mu.Lock()
	_, exists := pm.processes[pid]
	if !exists {
		pm.mu.Unlock()
		return fmt.Errorf("process %s not found", pid)
	}

	// Mark terminated and remove from map while still holding pm.mu.
	pm.processes[pid].SetState(models.ProcessTerminated)
	delete(pm.processes, pid)
	pm.mu.Unlock()

	// Call into memory manager WITHOUT holding pm.mu to prevent pm.mu → mm.mu deadlock.
	if err := pm.memoryManager.RemoveProcess(pid); err != nil {
		// Process is already removed from our map; log but don't re-insert.
		return fmt.Errorf("TerminateProcess %s: memory cleanup failed: %w", pid, err)
	}
	return nil
}

// ForkProcess forks a process (creates child with CoW)
func (pm *ProcessManager) ForkProcess(parentPID string) (*models.Process, error) {
	pm.mu.RLock()
	parent, exists := pm.processes[parentPID]
	pm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("parent process %s not found", parentPID)
	}

	// Create child process
	childPID := fmt.Sprintf("P%d", pm.nextPID.Add(1))
	child := models.NewProcess(
		childPID,
		fmt.Sprintf("%s-fork", parent.Name),
		parent.Priority,
		parent.VirtualPages,
	)
	child.ParentID = parentPID

	pm.mu.Lock()
	pm.processes[childPID] = child

	// Add child to parent's children list
	parent.AddChild(childPID)
	pm.mu.Unlock()

	// Register fork with memory manager (sets up CoW)
	if err := pm.memoryManager.ForkProcess(parentPID, childPID, child); err != nil {
		pm.mu.Lock()
		delete(pm.processes, childPID)
		pm.mu.Unlock()
		return nil, err
	}

	return child, nil
}

// GetProcess returns a process by ID
func (pm *ProcessManager) GetProcess(pid string) (*models.Process, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	process, exists := pm.processes[pid]
	if !exists {
		return nil, fmt.Errorf("process %s not found", pid)
	}

	return process, nil
}

// GetAllProcesses returns all processes
func (pm *ProcessManager) GetAllProcesses() []*models.Process {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	processes := make([]*models.Process, 0, len(pm.processes))
	for _, process := range pm.processes {
		processes = append(processes, process)
	}
	return processes
}

// AccessMemory performs a memory access for a process
func (pm *ProcessManager) AccessMemory(pid string, virtualPage uint64, write bool) error {
	pm.mu.RLock()
	process, exists := pm.processes[pid]
	pm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("process %s not found", pid)
	}

	// Update process state
	if process.GetState() == models.ProcessReady {
		process.SetState(models.ProcessRunning)
	}

	// Perform memory access through memory manager
	return pm.memoryManager.AccessMemory(pid, virtualPage, write)
}

// GetMemoryManager returns the memory manager
func (pm *ProcessManager) GetMemoryManager() *memory.MemoryManager {
	return pm.memoryManager
}

// Reset removes all processes
func (pm *ProcessManager) Reset() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Terminate all processes
	for pid := range pm.processes {
		pm.memoryManager.RemoveProcess(pid)
	}

	pm.processes = make(map[string]*models.Process)
	pm.nextPID.Store(1)
}

// GetProcessCount returns the number of active processes
func (pm *ProcessManager) GetProcessCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.processes)
}
