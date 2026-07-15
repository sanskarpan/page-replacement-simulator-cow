package unit

import (
	"sync"
	"testing"

	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/pkg/models"
)

func TestAllocateFrameOnNodeReturnsNonNil(t *testing.T) {
	nm := memory.NewNumaManager()
	nm.AddNode(models.NewNumaNode(0, "N0", 100, 4))

	frame, err := nm.AllocateFrameOnNode(0, 42, "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if frame == nil {
		t.Error("AllocateFrameOnNode returned nil frame on success")
	}
}

func TestAllocateFrameOnNodeConcurrentNoRace(t *testing.T) {
	nm := memory.NewNumaManager()
	nm.AddNode(models.NewNumaNode(0, "N0", 100, 64))

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = nm.AllocateFrameOnNode(0, uint64(i), "p")
		}()
	}
	wg.Wait()
}

func TestAllocateFrameOnNodeNoFramesError(t *testing.T) {
	nm := memory.NewNumaManager()
	nm.AddNode(models.NewNumaNode(0, "N0", 100, 0))

	_, err := nm.AllocateFrameOnNode(0, 1, "p")
	if err == nil {
		t.Error("expected error when node has no frames")
	}
}
