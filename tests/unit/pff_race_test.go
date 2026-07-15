package unit

import (
	"sync"
	"testing"
	"time"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/pkg/models"
)

func TestPFFGetFaultRaceConcurrent(t *testing.T) {
	pff := algorithms.NewPFF(1000, 0.1, 10.0, 2, 32, 8)
	frames := make([]*models.Frame, 10)
	for i := range frames {
		frames[i] = models.NewFrame(int32(i))
		frames[i].Allocate(uint64(i), "p")
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if j%3 == 0 {
					pff.OnPageFault(frames[j%10])
				}
				_ = pff.GetFaultRate()
				_ = pff.GetStats()
				time.Sleep(time.Microsecond)
			}
		}(i)
	}
	wg.Wait()
}
