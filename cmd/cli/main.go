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
	numFrames := flag.Int("frames", 64, "Number of physical memory frames")
	tlbSize := flag.Int("tlb", 16, "TLB size")
	algo := flag.String("algorithm", "LRU", "Page replacement algorithm (LRU, CLOCK, LFU, FIFO, Optimal, Random, ARC, CAR, WSClock, PFF, OPT+)")
	scenario := flag.String("scenario", "mixed", "Simulation scenario (sequential, random, locality, looping, mixed, fork_cow, thrashing)")
	compare := flag.Bool("compare", false, "Compare all algorithms on the chosen scenario and rank them")
	flag.Parse()

	if *compare {
		runComparison(*scenario, int32(*numFrames), *tlbSize)
		return
	}

	runSingle(*algo, *scenario, int32(*numFrames), *tlbSize)
}

func runSingle(algo, scenario string, numFrames int32, tlbSize int) {
	var algType algorithms.AlgorithmType
	switch algo {
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
		log.Fatalf("Invalid algorithm: %s", algo)
	}

	mm := memory.NewMemoryManager(numFrames, int(tlbSize), algType)
	defer mm.Close()
	pm := process.NewProcessManager(mm)
	sim := simulator.NewSimulator(pm)

	fmt.Println("========================================")
	fmt.Println("Page Replacement Simulator + CoW")
	fmt.Println("========================================")
	fmt.Printf("Frames: %d | TLB: %d | Algorithm: %s | Scenario: %s\n",
		numFrames, tlbSize, algo, scenario)
	fmt.Println("========================================")

	result, err := sim.RunScenario(scenario)
	if err != nil {
		log.Fatalf("Scenario failed: %v", err)
	}

	fmt.Println("\n========================================")
	fmt.Println("Simulation Results")
	fmt.Println("========================================")
	fmt.Printf("Duration: %v\n", result.Duration)

	if m := result.Metrics; m != nil {
		fmt.Printf("\n  Accesses : %d\n", m.TotalAccesses)
		fmt.Printf("  Faults   : %d  (%.2f%%)\n", m.PageFaults, m.PageFaultRate*100)
		fmt.Printf("  Hits     : %d  (%.2f%%)\n", m.PageHits, m.PageHitRate*100)
		fmt.Printf("  Evictions: %d\n", m.Evictions)
		fmt.Printf("\n  Frames   : %d used / %d total\n", m.UsedFrames, m.TotalFrames)
		if m.CoWCopies > 0 {
			fmt.Printf("  CoW copies created: %d  avoided: %d\n", m.CoWCopies, m.CoWSaves)
		}
	}
	fmt.Println("========================================")
}

func runComparison(scenario string, numFrames int32, tlbSize int) {
	// Seed once from the system clock and reuse across all algorithms so the
	// comparison is fair (same random workload for every algorithm).
	mm0 := memory.NewMemoryManager(numFrames, int(tlbSize), algorithms.AlgorithmLRU)
	pm0 := process.NewProcessManager(mm0)
	seedSim := simulator.NewSimulator(pm0)
	mm0.Close()

	fmt.Println("=======================================================================")
	fmt.Printf("Algorithm Comparison: scenario=%s  frames=%d  tlb=%d\n",
		scenario, numFrames, tlbSize)
	fmt.Println("=======================================================================")
	fmt.Println("Running all 11 algorithms with the same workload seed...")

	results, err := seedSim.CompareAlgorithms(scenario, numFrames, int(tlbSize))
	if err != nil {
		log.Fatalf("Comparison failed: %v", err)
	}

	if len(results) == 0 {
		fmt.Println("No results (all algorithms errored).")
		return
	}

	// Print ranked table.
	fmt.Println()
	fmt.Printf("%-4s  %-10s  %10s  %10s  %8s  %9s  %7s  %s\n",
		"Rank", "Algorithm", "FaultRate%", "HitRate%", "Faults", "Evictions", "CoW", "Time")
	fmt.Println("----  ----------  ----------  ----------  --------  ---------  -------  --------")

	best := results[0].FaultRate
	for _, r := range results {
		marker := ""
		if r.FaultRate == best {
			marker = " ★"
		}
		fmt.Printf("#%-3d  %-10s  %9.2f%%  %9.2f%%  %8d  %9d  %7d  %v%s\n",
			r.Rank,
			r.Algorithm,
			r.FaultRate*100,
			r.HitRate*100,
			r.PageFaults,
			r.Evictions,
			r.CoWCopies,
			r.Duration.Round(1*1000*1000), // round to ms
			marker,
		)
	}

	fmt.Println("=======================================================================")
	fmt.Printf("Winner: %s (fault rate %.2f%% on '%s' scenario)\n",
		results[0].Algorithm, results[0].FaultRate*100, scenario)
}
