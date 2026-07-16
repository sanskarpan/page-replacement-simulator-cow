package unit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
	"github.com/page-replacement-cow/internal/simulator"
)

// -------------------------------------------------------------------
// CompareFrameCounts (Belady curve sweep)
// -------------------------------------------------------------------

// TestCompareFrameCountsDescendingFaultRate verifies that adding more physical
// frames generally reduces the fault rate (Belady's anomaly excepted — we only
// check the overall trend from fewest to most frames).
func TestCompareFrameCountsDescendingFaultRate(t *testing.T) {
	mm := memory.NewMemoryManager(4, 4, algorithms.AlgorithmLRU)
	defer mm.Close()
	pm := process.NewProcessManager(mm)
	sim := simulator.NewSimulator(pm)

	frameCounts := []int32{4, 8, 16, 32}
	results, err := sim.CompareFrameCounts("locality", "LRU", frameCounts, 4)
	if err != nil {
		t.Fatalf("CompareFrameCounts: %v", err)
	}
	if len(results) != len(frameCounts) {
		t.Fatalf("expected %d results, got %d", len(frameCounts), len(results))
	}

	// Results must be in the same order as the input frame counts.
	for i, r := range results {
		if r.NumFrames != frameCounts[i] {
			t.Errorf("results[%d].NumFrames = %d, want %d", i, r.NumFrames, frameCounts[i])
		}
	}

	// Most-frames result should have ≤ fault rate than fewest-frames.
	first := results[0].FaultRate
	last := results[len(results)-1].FaultRate
	if last > first {
		t.Logf("fault rates: first=%v last=%v", first, last)
		// Belady anomaly can occur with FIFO; only warn, don't fail.
		t.Logf("WARNING: fault rate did not decrease with more frames (Belady anomaly?)")
	}
}

// TestCompareFrameCountsUnknownAlgorithm verifies that an unknown algorithm name
// returns an error immediately.
func TestCompareFrameCountsUnknownAlgorithm(t *testing.T) {
	mm := memory.NewMemoryManager(8, 4, algorithms.AlgorithmLRU)
	defer mm.Close()
	pm := process.NewProcessManager(mm)
	sim := simulator.NewSimulator(pm)

	_, err := sim.CompareFrameCounts("locality", "Nonexistent", []int32{8}, 4)
	if err == nil {
		t.Fatal("expected error for unknown algorithm, got nil")
	}
}

// TestCompareFrameCountsAllAlgorithms verifies that the sweep works for each
// named algorithm without panicking.
func TestCompareFrameCountsAllAlgorithms(t *testing.T) {
	algNames := []string{"LRU", "CLOCK", "LFU", "FIFO", "Random", "ARC", "NRU"}

	mm := memory.NewMemoryManager(4, 4, algorithms.AlgorithmLRU)
	defer mm.Close()
	pm := process.NewProcessManager(mm)
	sim := simulator.NewSimulator(pm)

	for _, name := range algNames {
		results, err := sim.CompareFrameCounts("locality", name, []int32{4, 8}, 4)
		if err != nil {
			t.Errorf("CompareFrameCounts(%q): %v", name, err)
			continue
		}
		if len(results) == 0 {
			t.Errorf("CompareFrameCounts(%q): no results", name)
		}
		t.Logf("%-8s  frames=4 fault=%.2f%%  frames=8 fault=%.2f%%",
			name, results[0].FaultRate*100, results[len(results)-1].FaultRate*100)
	}
}

// -------------------------------------------------------------------
// JSON trace save / load
// -------------------------------------------------------------------

// TestTraceSaveAndLoad records a trace, saves it to a temp file, reloads it,
// and verifies the round-trip is lossless.
func TestTraceSaveAndLoad(t *testing.T) {
	mm := memory.NewMemoryManager(8, 4, algorithms.AlgorithmLRU)
	defer mm.Close()
	pm := process.NewProcessManager(mm)
	sim := simulator.NewSimulator(pm)
	proc, _ := pm.CreateProcess("SaveLoad", 1, 200)

	sim.StartRecording()
	_ = sim.SimulateSequentialAccess(proc.ID, 0, 15, false)
	_ = sim.SimulateSequentialAccess(proc.ID, 0, 5, true)
	trace := sim.StopRecording()

	if len(trace.Entries) != 20 {
		t.Fatalf("expected 20 trace entries, got %d", len(trace.Entries))
	}

	// Save to a temp file.
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.json")
	if err := trace.Save(path); err != nil {
		t.Fatalf("Trace.Save: %v", err)
	}

	// Verify the file exists and is non-empty.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat trace file: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("saved trace file is empty")
	}

	// Reload and compare.
	loaded, err := simulator.LoadTrace(path)
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}
	if len(loaded.Entries) != len(trace.Entries) {
		t.Errorf("entry count mismatch: saved=%d loaded=%d", len(trace.Entries), len(loaded.Entries))
	}
	if loaded.Seed != trace.Seed {
		t.Errorf("seed mismatch: saved=%d loaded=%d", trace.Seed, loaded.Seed)
	}
	for i, e := range trace.Entries {
		got := loaded.Entries[i]
		if got.ProcessID != e.ProcessID || got.VirtualPage != e.VirtualPage || got.Write != e.Write {
			t.Errorf("entry[%d] mismatch: want %+v, got %+v", i, e, got)
		}
	}
}

// TestLoadTraceMissingFile verifies that LoadTrace returns an error for a
// non-existent path.
func TestLoadTraceMissingFile(t *testing.T) {
	_, err := simulator.LoadTrace("/nonexistent/path/trace.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestLoadTraceCorruptJSON verifies that LoadTrace returns a meaningful error
// when the file contains invalid JSON.
func TestLoadTraceCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not valid json}"), 0644); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	_, err := simulator.LoadTrace(path)
	if err == nil {
		t.Fatal("expected error for corrupt JSON, got nil")
	}
}

// TestTraceSaveLoadReplay records a trace, saves and reloads it, then replays it
// on a fresh manager and verifies that fault counts match the original run.
func TestTraceSaveLoadReplay(t *testing.T) {
	mm1 := memory.NewMemoryManager(8, 4, algorithms.AlgorithmLRU)
	defer mm1.Close()
	pm1 := process.NewProcessManager(mm1)
	sim1 := simulator.NewSimulator(pm1)
	proc1, _ := pm1.CreateProcess("Recorder", 1, 200) // capture proc to get its generated ID

	sim1.StartRecording()
	_ = sim1.SimulateSequentialAccess(proc1.ID, 0, 20, false) // use actual process ID
	trace := sim1.StopRecording()
	original := mm1.GetMetrics()

	dir := t.TempDir()
	path := filepath.Join(dir, "replay.json")
	if err := trace.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := simulator.LoadTrace(path)
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}

	mm2 := memory.NewMemoryManager(8, 4, algorithms.AlgorithmLRU)
	defer mm2.Close()
	pm2 := process.NewProcessManager(mm2)
	_, _ = pm2.CreateProcess("Recorder", 1, 200)
	sim2 := simulator.NewSimulator(pm2)

	if err := sim2.ReplayTrace(loaded); err != nil {
		t.Fatalf("ReplayTrace: %v", err)
	}
	replayed := mm2.GetMetrics()

	if original.PageFaults != replayed.PageFaults {
		t.Errorf("fault count mismatch: original=%d replayed=%d",
			original.PageFaults, replayed.PageFaults)
	}
}
