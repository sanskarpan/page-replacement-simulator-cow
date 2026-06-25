package main

import (
	"fmt"
	"log"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
)

func main() {
	fmt.Println("=== Page Replacement Simulator - Basic Usage ===")
	fmt.Println()

	// Create memory manager with 32 frames, 8-entry TLB, using LRU algorithm
	mm := memory.NewMemoryManager(32, 8, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	// Create a process
	proc, err := pm.CreateProcess("ExampleProcess", 1, 100)
	if err != nil {
		log.Fatalf("Failed to create process: %v", err)
	}

	fmt.Printf("Created process: %s\n\n", proc.ID)

	// Perform memory accesses
	fmt.Println("Performing memory accesses...")

	// Sequential access
	for i := uint64(0); i < 10; i++ {
		err := pm.AccessMemory(proc.ID, i, false)
		if err != nil {
			log.Printf("Failed to access page %d: %v", i, err)
		} else {
			fmt.Printf("  Accessed page %d (read)\n", i)
		}
	}

	// Write access
	err = pm.AccessMemory(proc.ID, 5, true)
	if err != nil {
		log.Printf("Failed to write page 5: %v", err)
	} else {
		fmt.Println("  Wrote to page 5")
	}

	// Get metrics
	fmt.Println("\n=== System Metrics ===")
	metrics := mm.GetMetrics()

	fmt.Printf("Total Accesses: %d\n", metrics.TotalAccesses)
	fmt.Printf("Page Faults: %d\n", metrics.PageFaults)
	fmt.Printf("Page Hits: %d\n", metrics.PageHits)
	fmt.Printf("Page Fault Rate: %.2f%%\n", metrics.PageFaultRate*100)
	fmt.Printf("Page Hit Rate: %.2f%%\n", metrics.PageHitRate*100)
	fmt.Printf("Used Frames: %d/%d\n", metrics.UsedFrames, metrics.TotalFrames)
	fmt.Printf("Free Frames: %d\n", metrics.FreeFrames)

	// Get TLB stats
	tlb := mm.GetTLB()
	tlbStats := tlb.GetStats()

	fmt.Println("\n=== TLB Statistics ===")
	fmt.Printf("TLB Hits: %d\n", tlbStats.Hits)
	fmt.Printf("TLB Misses: %d\n", tlbStats.Misses)
	fmt.Printf("TLB Hit Rate: %.2f%%\n", tlbStats.HitRate*100)

	// Get process details
	fmt.Println("\n=== Process Statistics ===")
	fmt.Printf("Process ID: %s\n", proc.ID)
	fmt.Printf("Process Name: %s\n", proc.Name)
	fmt.Printf("Memory Accesses: %d\n", proc.MemoryAccesses.Load())
	fmt.Printf("Page Faults: %d\n", proc.PageFaults.Load())
	fmt.Printf("Page Hits: %d\n", proc.PageHits.Load())
	fmt.Printf("Fault Rate: %.2f%%\n", proc.GetPageFaultRate()*100)
	fmt.Printf("Hit Rate: %.2f%%\n", proc.GetPageHitRate()*100)

	fmt.Println("\n=== Example Completed ===")
}
