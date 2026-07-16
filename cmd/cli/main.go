package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
	"github.com/page-replacement-cow/internal/simulator"
)

func main() {
	numFrames := flag.Int("frames", 64, "Number of physical memory frames")
	tlbSize := flag.Int("tlb", 16, "TLB size")
	algo := flag.String("algorithm", "LRU", "Page replacement algorithm (LRU, CLOCK, LFU, FIFO, Optimal, Random, ARC, CAR, WSClock, PFF, OPT+, NRU)")
	scenario := flag.String("scenario", "mixed", "Simulation scenario (sequential, random, locality, looping, mixed, fork_cow, thrashing)")
	compare := flag.Bool("compare", false, "Compare all algorithms on the chosen scenario and rank them")
	framesSweep := flag.String("frames-sweep", "", "Sweep frame counts for this algorithm (e.g. \"LRU\") and print the Belady curve")
	frameMin := flag.Int("frame-min", 2, "Minimum frame count for --frames-sweep")
	frameMax := flag.Int("frame-max", 128, "Maximum frame count for --frames-sweep")
	output := flag.String("output", "text", "Output format: text, json, csv")
	flag.Parse()

	if *framesSweep != "" {
		runFramesSweep(*framesSweep, *scenario, int32(*frameMin), int32(*frameMax), *tlbSize, *output)
		return
	}

	if *compare {
		runComparison(*scenario, int32(*numFrames), *tlbSize, *output)
		return
	}

	runSingle(*algo, *scenario, int32(*numFrames), *tlbSize, *output)
}

func parseAlgo(name string) algorithms.AlgorithmType {
	switch name {
	case "LRU":
		return algorithms.AlgorithmLRU
	case "CLOCK":
		return algorithms.AlgorithmCLOCK
	case "LFU":
		return algorithms.AlgorithmLFU
	case "FIFO":
		return algorithms.AlgorithmFIFO
	case "Optimal":
		return algorithms.AlgorithmOptimal
	case "Random":
		return algorithms.AlgorithmRandom
	case "ARC":
		return algorithms.AlgorithmARC
	case "CAR":
		return algorithms.AlgorithmCAR
	case "WSClock":
		return algorithms.AlgorithmWSClock
	case "PFF":
		return algorithms.AlgorithmPFF
	case "OPT+":
		return algorithms.AlgorithmOPTPlus
	case "NRU":
		return algorithms.AlgorithmNRU
	default:
		log.Fatalf("Invalid algorithm: %s", name)
		return algorithms.AlgorithmLRU
	}
}

func runSingle(algo, scenario string, numFrames int32, tlbSize int, outputFmt string) {
	algType := parseAlgo(algo)

	mm := memory.NewMemoryManager(numFrames, int(tlbSize), algType)
	defer mm.Close()
	pm := process.NewProcessManager(mm)
	sim := simulator.NewSimulator(pm)

	result, err := sim.RunScenario(scenario)
	if err != nil {
		log.Fatalf("Scenario failed: %v", err)
	}

	switch outputFmt {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			log.Fatalf("json encode: %v", err)
		}
	case "csv":
		w := csv.NewWriter(os.Stdout)
		_ = w.Write([]string{"scenario", "algorithm", "duration_ns", "faults", "hits", "fault_rate", "hit_rate", "evictions"})
		m := result.Metrics
		_ = w.Write([]string{
			scenario, algo,
			fmt.Sprintf("%d", result.Duration.Nanoseconds()),
			fmt.Sprintf("%d", m.PageFaults),
			fmt.Sprintf("%d", m.PageHits),
			fmt.Sprintf("%.6f", m.PageFaultRate),
			fmt.Sprintf("%.6f", m.PageHitRate),
			fmt.Sprintf("%d", m.Evictions),
		})
		w.Flush()
	default:
		fmt.Println("========================================")
		fmt.Println("Page Replacement Simulator + CoW")
		fmt.Println("========================================")
		fmt.Printf("Frames: %d | TLB: %d | Algorithm: %s | Scenario: %s\n",
			numFrames, tlbSize, algo, scenario)
		fmt.Println("========================================")
		fmt.Printf("\nDuration: %v\n", result.Duration)
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
}

func runComparison(scenario string, numFrames int32, tlbSize int, outputFmt string) {
	mm0 := memory.NewMemoryManager(numFrames, int(tlbSize), algorithms.AlgorithmLRU)
	pm0 := process.NewProcessManager(mm0)
	seedSim := simulator.NewSimulator(pm0)
	mm0.Close()

	results, err := seedSim.CompareAlgorithms(scenario, numFrames, int(tlbSize))
	if err != nil {
		log.Fatalf("Comparison failed: %v", err)
	}

	switch outputFmt {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			log.Fatalf("json encode: %v", err)
		}
	case "csv":
		w := csv.NewWriter(os.Stdout)
		_ = w.Write([]string{"rank", "algorithm", "fault_rate", "hit_rate", "faults", "hits", "evictions", "cow_copies", "duration_ns"})
		for _, r := range results {
			_ = w.Write([]string{
				fmt.Sprintf("%d", r.Rank),
				r.Algorithm,
				fmt.Sprintf("%.6f", r.FaultRate),
				fmt.Sprintf("%.6f", r.HitRate),
				fmt.Sprintf("%d", r.PageFaults),
				fmt.Sprintf("%d", r.PageHits),
				fmt.Sprintf("%d", r.Evictions),
				fmt.Sprintf("%d", r.CoWCopies),
				fmt.Sprintf("%d", r.Duration.Nanoseconds()),
			})
		}
		w.Flush()
	default:
		fmt.Println("=======================================================================")
		fmt.Printf("Algorithm Comparison: scenario=%s  frames=%d  tlb=%d\n",
			scenario, numFrames, tlbSize)
		fmt.Println("=======================================================================")
		fmt.Println("Running all 12 algorithms with the same workload seed...")

		if len(results) == 0 {
			fmt.Println("No results (all algorithms errored).")
			return
		}

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
				r.Duration.Round(1*1000*1000),
				marker,
			)
		}

		fmt.Println("=======================================================================")
		fmt.Printf("Winner: %s (fault rate %.2f%% on '%s' scenario)\n",
			results[0].Algorithm, results[0].FaultRate*100, scenario)
	}
}

func runFramesSweep(algo, scenario string, frameMin, frameMax int32, tlbSize int, outputFmt string) {
	if frameMin < 2 {
		frameMin = 2
	}
	if frameMax <= frameMin {
		frameMax = frameMin * 8
	}

	mm0 := memory.NewMemoryManager(frameMin, tlbSize, algorithms.AlgorithmLRU)
	pm0 := process.NewProcessManager(mm0)
	seedSim := simulator.NewSimulator(pm0)
	mm0.Close()

	frameCounts := geometricRange(frameMin, frameMax, 12)
	results, err := seedSim.CompareFrameCounts(scenario, algo, frameCounts, tlbSize)
	if err != nil {
		log.Fatalf("Frame count sweep failed: %v", err)
	}

	switch outputFmt {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			log.Fatalf("json encode: %v", err)
		}
	case "csv":
		w := csv.NewWriter(os.Stdout)
		_ = w.Write([]string{"num_frames", "algorithm", "fault_rate", "hit_rate", "faults", "hits", "evictions"})
		for _, r := range results {
			_ = w.Write([]string{
				fmt.Sprintf("%d", r.NumFrames),
				r.Algorithm,
				fmt.Sprintf("%.6f", r.FaultRate),
				fmt.Sprintf("%.6f", r.HitRate),
				fmt.Sprintf("%d", r.PageFaults),
				fmt.Sprintf("%d", r.PageHits),
				fmt.Sprintf("%d", r.Evictions),
			})
		}
		w.Flush()
	default:
		fmt.Println("================================================================")
		fmt.Printf("Belady Curve: algorithm=%s  scenario=%s  tlb=%d\n", algo, scenario, tlbSize)
		fmt.Println("================================================================")
		fmt.Printf("%-10s  %10s  %10s  %8s  %8s\n",
			"Frames", "FaultRate%", "HitRate%", "Faults", "Evictions")
		fmt.Println("----------  ----------  ----------  --------  ---------")
		for _, r := range results {
			fmt.Printf("%-10d  %9.2f%%  %9.2f%%  %8d  %9d\n",
				r.NumFrames, r.FaultRate*100, r.HitRate*100, r.PageFaults, r.Evictions)
		}
		fmt.Println("================================================================")
	}
}

// geometricRange returns up to maxPoints frame counts from min to max in a
// geometric (doubling) progression.
func geometricRange(min, max int32, maxPoints int) []int32 {
	result := []int32{min}
	curr := min
	for len(result) < maxPoints && curr < max {
		next := curr * 2
		if next > max {
			next = max
		}
		result = append(result, next)
		curr = next
	}
	return result
}
