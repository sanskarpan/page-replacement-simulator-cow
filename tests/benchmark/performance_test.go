package benchmark

import (
	"testing"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
)

// BenchmarkMemoryAccess benchmarks basic memory access
func BenchmarkMemoryAccess(b *testing.B) {
	mm := memory.NewMemoryManager(128, 16, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("Benchmark", 1, 10000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		page := uint64(i % 100)
		pm.AccessMemory(proc.ID, page, false)
	}
}

// BenchmarkLRU benchmarks LRU algorithm
func BenchmarkLRU(b *testing.B) {
	mm := memory.NewMemoryManager(128, 16, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("Benchmark", 1, 10000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		page := uint64(i % 200)
		pm.AccessMemory(proc.ID, page, false)
	}
}

// BenchmarkCLOCK benchmarks CLOCK algorithm
func BenchmarkCLOCK(b *testing.B) {
	mm := memory.NewMemoryManager(128, 16, algorithms.AlgorithmCLOCK)
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("Benchmark", 1, 10000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		page := uint64(i % 200)
		pm.AccessMemory(proc.ID, page, false)
	}
}

// BenchmarkLFU benchmarks LFU algorithm
func BenchmarkLFU(b *testing.B) {
	mm := memory.NewMemoryManager(128, 16, algorithms.AlgorithmLFU)
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("Benchmark", 1, 10000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		page := uint64(i % 200)
		pm.AccessMemory(proc.ID, page, false)
	}
}

// BenchmarkFIFO benchmarks FIFO algorithm
func BenchmarkFIFO(b *testing.B) {
	mm := memory.NewMemoryManager(128, 16, algorithms.AlgorithmFIFO)
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("Benchmark", 1, 10000)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		page := uint64(i % 200)
		pm.AccessMemory(proc.ID, page, false)
	}
}

// BenchmarkTLBLookup benchmarks TLB lookup
func BenchmarkTLBLookup(b *testing.B) {
	mm := memory.NewMemoryManager(128, 64, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	proc, _ := pm.CreateProcess("Benchmark", 1, 10000)

	// Warm up TLB
	for i := uint64(0); i < 50; i++ {
		pm.AccessMemory(proc.ID, i, false)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		page := uint64(i % 50) // Access pages in TLB
		pm.AccessMemory(proc.ID, page, false)
	}
}

// BenchmarkCopyOnWrite benchmarks CoW operations
func BenchmarkCopyOnWrite(b *testing.B) {
	mm := memory.NewMemoryManager(256, 16, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	// Create parent and load pages
	parent, _ := pm.CreateProcess("Parent", 1, 10000)
	for i := uint64(0); i < 100; i++ {
		pm.AccessMemory(parent.ID, i, false)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Fork child
		child, _ := pm.ForkProcess(parent.ID)

		// Write to trigger CoW
		for j := uint64(0); j < 10; j++ {
			pm.AccessMemory(child.ID, j, true)
		}

		// Clean up
		pm.TerminateProcess(child.ID)
	}
}
