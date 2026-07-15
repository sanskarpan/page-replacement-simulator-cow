package unit

import (
	"sync"
	"testing"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
)

func TestManagerConcurrentAccessAndRemove(t *testing.T) {
	mm := memory.NewMemoryManager(32, 8, algorithms.AlgorithmLRU)
	mm.EnableClustering(true)
	pm := process.NewProcessManager(mm)

	proc, err := pm.CreateProcess("RaceTest", 1, 100)
	if err != nil {
		t.Fatalf("create process: %v", err)
	}
	// Prime TLB: first access loads pages into memory and TLB.
	for i := uint64(0); i < 5; i++ {
		if err := pm.AccessMemory(proc.ID, i, false); err != nil {
			t.Fatalf("prime access %d: %v", i, err)
		}
	}

	var wg sync.WaitGroup
	// Goroutine 1: concurrent write accesses on already-TLB-cached pages.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = pm.AccessMemory(proc.ID, uint64(i%5), true)
		}
	}()
	// Goroutine 2: concurrent reads on same process.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = pm.AccessMemory(proc.ID, uint64(i%5), false)
		}
	}()
	wg.Wait()
	// If race detector fires, the test fails automatically.
}

func TestManagerTLBHitWriteWithConcurrentRemove(t *testing.T) {
	mm := memory.NewMemoryManager(32, 8, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	proc, err := pm.CreateProcess("RemoveRace", 1, 100)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := uint64(0); i < 5; i++ {
		_ = pm.AccessMemory(proc.ID, i, false)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			_ = pm.AccessMemory(proc.ID, uint64(i%5), true)
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = pm.TerminateProcess(proc.ID)
	}()
	wg.Wait()
}
