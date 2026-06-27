package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
	"github.com/page-replacement-cow/internal/simulator"
)

func main() {
	// Parse flags
	numFrames := flag.Int("frames", 64, "Number of physical memory frames")
	tlbSize := flag.Int("tlb", 16, "TLB size")
	algo := flag.String("algorithm", "LRU", "Page replacement algorithm (LRU, CLOCK, LFU, FIFO, Optimal, Random, ARC, CAR, WSClock, PFF, OPT+)")
	scenario := flag.String("scenario", "mixed", "Simulation scenario to run")
	flag.Parse()

	// Parse algorithm
	var algType algorithms.AlgorithmType
	switch *algo {
	case "LRU":
		algType = algorithms.AlgorithmLRU
	case "CLOCK":
		algType = algorithms.AlgorithmCLOCK
	case "LFU":
		algType = algorithms.AlgorithmLFU
	case "FIFO":
		algType = algorithms.AlgorithmFIFO
	case "Optimal":
		algType = algorithms.AlgorithmOptimal
	case "Random":
		algType = algorithms.AlgorithmRandom
	case "ARC":
		algType = algorithms.AlgorithmARC
	case "CAR":
		algType = algorithms.AlgorithmCAR
	case "WSClock":
		algType = algorithms.AlgorithmWSClock
	case "PFF":
		algType = algorithms.AlgorithmPFF
	case "OPT+":
		algType = algorithms.AlgorithmOPTPlus
	default:
		log.Fatalf("Invalid algorithm: %s", *algo)
	}

	// Create system
	mm := memory.NewMemoryManager(int32(*numFrames), *tlbSize, algType)
	pm := process.NewProcessManager(mm)
	sim := simulator.NewSimulator(pm)

	// Print header
	fmt.Println("========================================")
	fmt.Println("Page Replacement Simulator + CoW")
	fmt.Println("========================================")
	fmt.Printf("Frames: %d\n", *numFrames)
	fmt.Printf("TLB Size: %d\n", *tlbSize)
	fmt.Printf("Algorithm: %s\n", *algo)
	fmt.Printf("Scenario: %s\n", *scenario)
	fmt.Println("========================================")
	fmt.Println()

	// Run scenario
	fmt.Printf("Running scenario: %s\n", *scenario)
	fmt.Printf("Description: %s\n\n", sim.GetScenarioDescription(*scenario))

	result, err := sim.RunScenario(*scenario)
	if err != nil {
		log.Fatalf("Scenario failed: %v", err)
	}

	// Print results
	fmt.Println("\n========================================")
	fmt.Println("Simulation Results")
	fmt.Println("========================================")
	fmt.Printf("Duration: %v\n", result.Duration)
	fmt.Printf("Success: %v\n", result.Success)

	if result.Metrics != nil {
		m := result.Metrics

		fmt.Println("\nMetrics:")
		fmt.Printf("  Total Accesses: %d\n", m.TotalAccesses)
		fmt.Printf("  Page Faults: %d\n", m.PageFaults)
		fmt.Printf("  Page Hits: %d\n", m.PageHits)
		fmt.Printf("  Evictions: %d\n", m.Evictions)
		fmt.Printf("  Page Fault Rate: %.2f%%\n", m.PageFaultRate*100)
		fmt.Printf("  Page Hit Rate: %.2f%%\n", m.PageHitRate*100)

		fmt.Println("\nMemory:")
		fmt.Printf("  Total Frames: %d\n", m.TotalFrames)
		fmt.Printf("  Used Frames: %d\n", m.UsedFrames)
		fmt.Printf("  Free Frames: %d\n", m.FreeFrames)
		fmt.Printf("  Memory Usage: %.1f%%\n", float64(m.UsedFrames)/float64(m.TotalFrames)*100)

		fmt.Println("\nPages:")
		fmt.Printf("  Pages in Memory: %d\n", m.PagesInMemory)
		fmt.Printf("  Shared Pages: %d\n", m.SharedPages)
		fmt.Printf("  Dirty Pages: %d\n", m.DirtyPages)

		if m.CoWCopies > 0 {
			fmt.Println("\nCopy-on-Write:")
			fmt.Printf("  Copies Created: %d\n", m.CoWCopies)
			fmt.Printf("  Copies Avoided: %d\n", m.CoWSaves)
			fmt.Printf("  Shared Reads: %d\n", m.SharedPageReads)
		}

		fmt.Println("\nProcesses:")
		fmt.Printf("  Active: %d\n", m.ActiveProcesses)
		fmt.Printf("  Total: %d\n", m.TotalProcesses)
	}

	fmt.Println("========================================")
}
