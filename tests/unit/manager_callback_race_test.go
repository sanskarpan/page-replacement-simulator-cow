package unit

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
)

// TestEventCallbackConcurrentSetAndFire verifies there is no data race when
// SetEventCallback is called concurrently with operations that emit events
// (which read mm.eventCallback in eventWorker). Previously eventWorker read the
// field without any lock while SetEventCallback wrote it under mm.mu.Lock().
func TestEventCallbackConcurrentSetAndFire(t *testing.T) {
	mm := memory.NewMemoryManager(32, 16, algorithms.AlgorithmLRU)
	pm := process.NewProcessManager(mm)

	var (
		callCount int64
		wg        sync.WaitGroup
	)

	proc, _ := pm.CreateProcess("CB", 1, 200)
	for i := uint64(0); i < 10; i++ {
		_ = pm.AccessMemory(proc.ID, i, false)
	}

	// Rapidly toggle the callback while events are being emitted.
	for round := 0; round < 5; round++ {
		wg.Add(2)

		go func() {
			defer wg.Done()
			mm.SetEventCallback(func(event string, data map[string]interface{}) {
				atomic.AddInt64(&callCount, 1)
			})
		}()

		go func() {
			defer wg.Done()
			for i := uint64(0); i < 20; i++ {
				_ = pm.AccessMemory(proc.ID, i%10, false)
			}
		}()
	}

	wg.Wait()

	// Also swap to nil to test the nil-callback fast path races.
	mm.SetEventCallback(nil)
	for i := uint64(0); i < 20; i++ {
		_ = pm.AccessMemory(proc.ID, i%10, false)
	}

	time.Sleep(20 * time.Millisecond)
	t.Logf("callback race test passed: invocations=%d", atomic.LoadInt64(&callCount))
}
