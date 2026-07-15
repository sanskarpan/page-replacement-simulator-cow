package unit

import (
	"sync"
	"testing"

	"github.com/page-replacement-cow/internal/cow"
	"github.com/page-replacement-cow/pkg/models"
)

func TestCowHandleWriteConcurrentNoDoubleCopy(t *testing.T) {
	cowMgr := cow.NewCopyOnWrite()

	page := models.NewPage(42, "P1")
	page.Shared.Store(true)

	_ = cowMgr.SharePage(42, 1, []string{"P1", "P2"})

	var copies int64
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, pid := range []string{"P1", "P2"} {
		pid := pid
		wg.Add(1)
		go func() {
			defer wg.Done()
			needsCopy, _, _ := cowMgr.HandleWrite(42, pid, page)
			if needsCopy {
				mu.Lock()
				copies++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// At most one copy should be created: the second writer should have
	// seen refCount=1 after the first decrement and claimed the original.
	if copies > 1 {
		t.Errorf("Expected at most 1 CoW copy, got %d — TOCTOU race", copies)
	}
	// Also verify no frame was leaked: refCount should be 0 (entry cleaned up).
	if cowMgr.GetRefCount(42) != 0 {
		t.Errorf("Expected refCount=0 after both writes, got %d", cowMgr.GetRefCount(42))
	}
}
