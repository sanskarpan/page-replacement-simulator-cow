package unit

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
)

func TestEmitEventNoGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()

	mm := memory.NewMemoryManager(64, 16, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	var received int64
	mm.SetEventCallback(func(event string, data map[string]interface{}) {
		atomic.AddInt64(&received, 1)
	})

	proc, _ := pm.CreateProcess("LeakTest", 1, 200)
	for i := uint64(0); i < 100; i++ {
		_ = pm.AccessMemory(proc.ID, i, false)
	}

	// Allow background consumer to drain.
	time.Sleep(50 * time.Millisecond)

	after := runtime.NumGoroutine()
	// Allow for a small fixed number of background goroutines (the consumer goroutine).
	if after-before > 5 {
		t.Errorf("goroutine count grew by %d (before=%d, after=%d) — possible leak",
			after-before, before, after)
	}
	t.Logf("events received: %d, goroutine delta: %d", atomic.LoadInt64(&received), after-before)
}
