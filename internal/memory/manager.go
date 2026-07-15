package memory

import (
	"fmt"
	"sync"
	"time"

	"github.com/page-replacement-cow/internal/algorithms"
	"github.com/page-replacement-cow/internal/cow"
	"github.com/page-replacement-cow/pkg/models"
)

type eventMsg struct {
	event string
	data  map[string]interface{}
}

type MemoryManager struct {
	frameTable    *FrameTable
	numFrames     int32
	pageTables    map[string]*PageTable
	multiLevelPT  map[string]*MultiLevelPageTable
	tlb           *TLB
	algorithm     algorithms.PageReplacementAlgorithm
	cowManager    *cow.CopyOnWrite
	metrics       *models.Metrics
	processes     map[string]*models.Process
	mu            sync.RWMutex
	eventCallback func(event string, data map[string]interface{})
	eventCh       chan eventMsg

	numaManager          *NumaManager
	compressionManager   *CompressionManager
	clusterManager       *PageClusterManager
	recentAccesses       map[string][]uint64
	numaEnabled          bool
	compressionEnabled   bool
	clusteringEnabled    bool
}

func NewMemoryManager(numFrames int32, tlbSize int, algType algorithms.AlgorithmType) *MemoryManager {
	mm := &MemoryManager{
		frameTable:         NewFrameTable(numFrames),
		numFrames:          numFrames,
		pageTables:         make(map[string]*PageTable),
		multiLevelPT:       make(map[string]*MultiLevelPageTable),
		tlb:                NewTLB(tlbSize),
		cowManager:         cow.NewCopyOnWrite(),
		metrics:            models.NewMetrics(numFrames),
		processes:          make(map[string]*models.Process),
		numaManager:        NewNumaManager(),
		compressionManager: NewCompressionManager(0.8),
		clusterManager:     NewPageClusterManager(4, 16),
		recentAccesses:     make(map[string][]uint64),
		numaEnabled:        false,
		compressionEnabled: false,
		clusteringEnabled:  false,
	}

	mm.SetAlgorithm(algType)

	// Pass channel by value so eventWorker never reads the mm.eventCh field.
	eventCh := make(chan eventMsg, 256)
	mm.eventCh = eventCh
	go mm.eventWorker(eventCh)

	return mm
}

func (mm *MemoryManager) EnableNuma(enabled bool) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	mm.numaEnabled = enabled
	if enabled && len(mm.numaManager.GetNodes()) == 0 {
		mm.numaManager.AddNode(models.NewNumaNode(0, "Node-0", 100, mm.numFrames/2))
		mm.numaManager.AddNode(models.NewNumaNode(1, "Node-1", 150, mm.numFrames/2))
	}
}

func (mm *MemoryManager) EnableCompression(enabled bool) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	mm.compressionEnabled = enabled
}

func (mm *MemoryManager) EnableClustering(enabled bool) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	mm.clusteringEnabled = enabled
}

func (mm *MemoryManager) GetNumaManager() *NumaManager   { return mm.numaManager }
func (mm *MemoryManager) GetCompressionManager() *CompressionManager { return mm.compressionManager }
func (mm *MemoryManager) GetClusterManager() *PageClusterManager { return mm.clusterManager }

func (mm *MemoryManager) SetAlgorithm(algType algorithms.AlgorithmType) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	switch algType {
	case algorithms.AlgorithmLRU:
		mm.algorithm = algorithms.NewLRU()
	case algorithms.AlgorithmCLOCK:
		mm.algorithm = algorithms.NewCLOCK()
	case algorithms.AlgorithmLFU:
		mm.algorithm = algorithms.NewLFU()
	case algorithms.AlgorithmFIFO:
		mm.algorithm = algorithms.NewFIFO()
	case algorithms.AlgorithmOptimal:
		mm.algorithm = algorithms.NewOptimal()
	case algorithms.AlgorithmRandom:
		mm.algorithm = algorithms.NewRandom()
	case algorithms.AlgorithmARC:
		mm.algorithm = algorithms.NewARC(mm.numFrames)
	case algorithms.AlgorithmCAR:
		mm.algorithm = algorithms.NewCAR(mm.numFrames)
	case algorithms.AlgorithmWSClock:
		mm.algorithm = algorithms.NewWSClock(1000)
	case algorithms.AlgorithmPFF:
		mm.algorithm = algorithms.NewPFF(5000, 0.1, 10.0, 4, mm.numFrames, mm.numFrames/2)
	case algorithms.AlgorithmOPTPlus:
		mm.algorithm = algorithms.NewOptPlus()
	default:
		mm.algorithm = algorithms.NewLRU()
	}
}

func (mm *MemoryManager) CreateProcess(process *models.Process) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if _, exists := mm.processes[process.ID]; exists {
		return fmt.Errorf("process %s already exists", process.ID)
	}

	mm.processes[process.ID] = process
	mm.pageTables[process.ID] = NewPageTable(process.ID)
	mm.multiLevelPT[process.ID] = NewMultiLevelPageTable(process.ID)
	mm.recentAccesses[process.ID] = make([]uint64, 0, 16)
	mm.metrics.TotalProcesses.Add(1)
	mm.metrics.ActiveProcesses.Add(1)

	mm.emitEvent("process_created", map[string]interface{}{
		"process_id": process.ID,
		"name":       process.Name,
	})

	return nil
}

func (mm *MemoryManager) RemoveProcess(processID string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	pageTable, exists := mm.pageTables[processID]
	if !exists {
		return fmt.Errorf("process %s not found", processID)
	}

	pages := pageTable.GetAllPages()
	for _, page := range pages {
		if page.IsPresent() {
			frameNum := page.GetFrame()
			if frameNum >= 0 {
				mm.frameTable.ReleaseFrame(frameNum)
			}
		}
		if page.IsShared() {
			mm.cowManager.UnsharePage(page.ID, processID)
		}
	}

	mm.cowManager.RemoveProcess(processID)
	mm.tlb.InvalidateProcess(processID)
	mm.clusterManager.ClearClusters(processID)

	delete(mm.pageTables, processID)
	delete(mm.multiLevelPT, processID)
	delete(mm.processes, processID)
	delete(mm.recentAccesses, processID)

	mm.metrics.ActiveProcesses.Add(-1)

	mm.emitEvent("process_removed", map[string]interface{}{
		"process_id": processID,
	})

	return nil
}

func (mm *MemoryManager) AccessMemory(processID string, virtualPage uint64, write bool) error {
	startTime := time.Now()

	mm.mu.RLock()
	process, exists := mm.processes[processID]
	mm.mu.RUnlock()
	if !exists {
		return fmt.Errorf("process %s not found", processID)
	}

	process.RecordMemoryAccess()

	// Read-only TLB fast path: no shared-map writes needed, so no lock required.
	if !write {
		if frameNum, hit := mm.tlb.Lookup(processID, virtualPage); hit {
			frame, _ := mm.frameTable.GetFrame(frameNum)
			if frame != nil {
				mm.algorithm.OnPageAccess(frame, false)
				process.RecordPageHit()
				mm.metrics.RecordPageHit()
				mm.emitEvent("memory_access", map[string]interface{}{
					"process_id":   processID,
					"virtual_page": virtualPage,
					"write":        false,
					"hit":          true,
					"latency_ns":   time.Since(startTime).Nanoseconds(),
				})
				return nil
			}
		}
	}

	// Write-lock section: handles TLB misses, page faults, and write CoW.
	// Guards pageTables, recentAccesses, and pageTable.Entries (BUG-02, BUG-03, BUG-04 fix).
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// trackAccess under write lock so recentAccesses map is fully guarded (BUG-03 fix).
	// Prefetch hint only needs updating on the write-locked path since tryPrefetch
	// is only ever called here, not on the read-only TLB fast path.
	if mm.clusteringEnabled {
		mm.trackAccess(processID, virtualPage)
	}

	pageTable, exists := mm.pageTables[processID]
	if !exists {
		return fmt.Errorf("page table not found for process %s", processID)
	}

	page := pageTable.GetOrCreatePage(virtualPage)

	if page.IsPresent() {
		frameNum := page.GetFrame()
		frame, _ := mm.frameTable.GetFrame(frameNum)
		if frame != nil {
			mm.algorithm.OnPageAccess(frame, write)
			mm.tlb.Insert(processID, virtualPage, frameNum)
			process.RecordPageHit()
			mm.metrics.RecordPageHit()

			if write && page.IsShared() {
				if err := mm.handleCoW(processID, page, frame); err != nil {
					return err
				}
			}

			mm.emitEvent("memory_access", map[string]interface{}{
				"process_id":   processID,
				"virtual_page": virtualPage,
				"write":        write,
				"hit":          true,
				"latency_ns":   time.Since(startTime).Nanoseconds(),
			})

			return nil
		}
	}

	if err := mm.handlePageFault(processID, page, write); err != nil {
		return err
	}

	process.RecordPageFault()
	mm.metrics.RecordPageFault()

	mm.emitEvent("page_fault", map[string]interface{}{
		"process_id":   processID,
		"virtual_page": virtualPage,
		"write":        write,
		"latency_ns":   time.Since(startTime).Nanoseconds(),
	})

	if mm.clusteringEnabled {
		mm.tryPrefetch(processID)
	}

	return nil
}

func (mm *MemoryManager) trackAccess(processID string, virtualPage uint64) {
	accesses, ok := mm.recentAccesses[processID]
	if !ok {
		return
	}
	accesses = append(accesses, virtualPage)
	if len(accesses) > 16 {
		accesses = accesses[1:]
	}
	mm.recentAccesses[processID] = accesses

	if len(accesses) >= 3 {
		tail := accesses[len(accesses)-3:]
		mm.clusterManager.DetectSequential(processID, tail)
	}
}

func (mm *MemoryManager) tryPrefetch(processID string) {
	pages := mm.recentAccesses[processID]
	if len(pages) == 0 {
		return
	}
	lastPage := pages[len(pages)-1]
	prefetchPages := mm.clusterManager.GetPrefetchPages(lastPage)
	if len(prefetchPages) == 0 {
		return
	}

	for i := 0; i < 2; i++ {
		if i >= len(prefetchPages) {
			break
		}
		prefetchPage := prefetchPages[i]
		pageTable, ok := mm.pageTables[processID]
		if !ok {
			return
		}
		p := pageTable.GetOrCreatePage(prefetchPage)

		frame, err := mm.frameTable.AllocateFrame(p.ID, processID)
		if err != nil {
			return
		}
		if mm.numaEnabled {
			frame.NumaNodeID = mm.selectLocalNode(processID)
		}
		p.SetFrame(frame.ID)
		mm.algorithm.OnPageFault(frame)
		mm.tlb.Insert(processID, p.ID, frame.ID)
	}
}

func (mm *MemoryManager) handlePageFault(processID string, page *models.Page, write bool) error {
	if mm.compressionEnabled {
		cp := mm.compressionManager.DecompressPage(page.ID)
		if cp != nil {
			frame, err := mm.frameTable.AllocateFrame(page.ID, processID)
			if err != nil {
				if evErr := mm.atomicEvictAndAlloc(page, processID); evErr != nil {
					return fmt.Errorf("handlePageFault (compressed): %w", evErr)
				}
			} else {
				page.SetFrame(frame.ID)
				mm.algorithm.OnPageFault(frame)
				mm.tlb.Insert(processID, page.ID, frame.ID)
			}
			return nil
		}
	}

	frame, err := mm.frameTable.AllocateFrame(page.ID, processID)
	if err != nil {
		if evErr := mm.atomicEvictAndAlloc(page, processID); evErr != nil {
			return fmt.Errorf("handlePageFault: %w", evErr)
		}
	} else {
		if mm.numaEnabled {
			frame.NumaNodeID = mm.selectLocalNode(processID)
		}
		page.SetFrame(frame.ID)
		mm.algorithm.OnPageFault(frame)
		mm.tlb.Insert(processID, page.ID, frame.ID)
	}

	if write && page.IsShared() {
		f, err := mm.frameTable.GetFrame(page.GetFrame())
		if err == nil && f != nil {
			return mm.handleCoW(processID, page, f)
		}
	}

	return nil
}

func (mm *MemoryManager) atomicEvictAndAlloc(page *models.Page, processID string) error {
	usedFrames := mm.frameTable.GetUsedFrames()
	victim, err := mm.algorithm.SelectVictim(usedFrames)
	if err != nil {
		return fmt.Errorf("no evictable frame: %w", err)
	}

	if err := mm.evictPage(victim); err != nil {
		return fmt.Errorf("eviction failed: %w", err)
	}

	frame := victim
	frame.Allocate(page.ID, processID)
	if mm.numaEnabled {
		frame.NumaNodeID = mm.selectLocalNode(processID)
	}
	page.SetFrame(frame.ID)
	mm.algorithm.OnPageFault(frame)
	mm.tlb.Insert(processID, page.ID, frame.ID)
	return nil
}

func (mm *MemoryManager) handleCoW(processID string, page *models.Page, frame *models.Frame) error {
	needsCopy, newPageID, err := mm.cowManager.HandleWrite(page.ID, processID, page)
	if err != nil {
		return err
	}

	if !needsCopy {
		page.Shared.Store(false)
		page.ReadOnly.Store(false)
		return nil
	}

	newPage, err := mm.cowManager.CopyPage(page, newPageID, processID)
	if err != nil {
		return err
	}

	newFrame, err := mm.frameTable.AllocateFrame(newPageID, processID)
	if err != nil {
		usedFrames := mm.frameTable.GetUsedFrames()
		victim, err := mm.algorithm.SelectVictim(usedFrames)
		if err != nil {
			return err
		}
		if err := mm.evictPage(victim); err != nil {
			return err
		}
		newFrame = victim
		newFrame.Allocate(newPageID, processID)
	}

	if mm.numaEnabled {
		newFrame.NumaNodeID = mm.selectLocalNode(processID)
	}

	// Register the new CoW frame with the replacement algorithm so ARC/CAR
	// T1/T2 lists stay coherent (BUG-06 fix).
	mm.algorithm.OnPageFault(newFrame)

	newPage.SetFrame(newFrame.ID)

	pageTable := mm.pageTables[processID]
	pageTable.Entries[page.ID] = newPage

	mm.tlb.Invalidate(processID, page.ID)
	// Use the original virtual page ID so post-CoW accesses hit the TLB
	// (BUG-12 fix: was newPageID which is the synthetic CoW ID ≥1,000,000).
	mm.tlb.Insert(processID, page.ID, newFrame.ID)

	mm.metrics.RecordCoWCopy()
	process := mm.processes[processID]
	if process != nil {
		process.RecordCoWCopy()
	}

	mm.emitEvent("cow_copy", map[string]interface{}{
		"process_id":     processID,
		"original_page":  page.ID,
		"new_page":       newPageID,
		"original_frame": frame.ID,
		"new_frame":      newFrame.ID,
	})

	return nil
}

func (mm *MemoryManager) evictPage(frame *models.Frame) error {
	if frame == nil {
		return fmt.Errorf("frame is nil")
	}

	pageID := frame.GetPageID()
	processID := frame.GetProcessID()

	if mm.compressionEnabled && frame.IsModified() {
		dummyData := make([]byte, 4096)
		mm.compressionManager.CompressPage(pageID, dummyData)
	}

	pageTable, exists := mm.pageTables[processID]
	if exists {
		// O(1) direct lookup instead of O(n) GetAllPages scan (BUG-15 fix).
		page, err := pageTable.GetPage(pageID)
		if err == nil && page != nil && page.IsPresent() {
			page.SetFrame(-1)
		}
	}

	mm.tlb.Invalidate(processID, pageID)
	mm.algorithm.OnPageEviction(frame)
	mm.metrics.RecordEviction(frame.IsModified())

	mm.emitEvent("page_eviction", map[string]interface{}{
		"process_id": processID,
		"page_id":    pageID,
		"frame_id":   frame.ID,
		"dirty":      frame.IsModified(),
	})

	frame.Release()

	return nil
}

func (mm *MemoryManager) ForkProcess(parentID, childID string, childProcess *models.Process) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	parentTable, exists := mm.pageTables[parentID]
	if !exists {
		return fmt.Errorf("parent process %s not found", parentID)
	}

	childTable := parentTable.Clone(childID)
	mm.pageTables[childID] = childTable
	mm.multiLevelPT[childID] = NewMultiLevelPageTable(childID)
	mm.recentAccesses[childID] = make([]uint64, 0, 16)
	mm.processes[childID] = childProcess

	parentPages := parentTable.GetPresentPages()
	childPages := childTable.GetPresentPages()

	if err := mm.cowManager.ForkProcess(parentID, childID, parentPages); err != nil {
		return err
	}

	for _, page := range append(parentPages, childPages...) {
		page.MakeShared()
	}

	mm.metrics.TotalProcesses.Add(1)
	mm.metrics.ActiveProcesses.Add(1)

	mm.emitEvent("process_forked", map[string]interface{}{
		"parent_id": parentID,
		"child_id":  childID,
		"pages":     len(parentPages),
	})

	return nil
}

func (mm *MemoryManager) GetMetrics() *models.MetricsSnapshot {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	frameStats := mm.frameTable.GetStats()

	var pagesInMemory, sharedPages, dirtyPages int32
	for _, pageTable := range mm.pageTables {
		stats := pageTable.GetStats()
		pagesInMemory += int32(stats.PresentPages)
		sharedPages += int32(stats.SharedPages)
		dirtyPages += int32(stats.DirtyPages)
	}

	return mm.metrics.GetSnapshotWithStats(
		frameStats.UsedFrames, frameStats.FreeFrames, frameStats.PinnedFrames,
		pagesInMemory, sharedPages, dirtyPages,
	)
}

func (mm *MemoryManager) GetProcess(processID string) (*models.Process, error) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	process, exists := mm.processes[processID]
	if !exists {
		return nil, fmt.Errorf("process %s not found", processID)
	}

	return process, nil
}

func (mm *MemoryManager) GetAllProcesses() []*models.Process {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	processes := make([]*models.Process, 0, len(mm.processes))
	for _, process := range mm.processes {
		processes = append(processes, process)
	}
	return processes
}

func (mm *MemoryManager) GetPageTable(processID string) (*PageTable, error) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	pt, exists := mm.pageTables[processID]
	if !exists {
		return nil, fmt.Errorf("page table not found for process %s", processID)
	}

	return pt, nil
}

func (mm *MemoryManager) GetMultiLevelPageTable(processID string) (*MultiLevelPageTable, error) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	mpt, exists := mm.multiLevelPT[processID]
	if !exists {
		return nil, fmt.Errorf("multi-level page table not found for process %s", processID)
	}

	return mpt, nil
}

func (mm *MemoryManager) GetFrameTable() *FrameTable         { return mm.frameTable }
func (mm *MemoryManager) GetTLB() *TLB                       { return mm.tlb }
func (mm *MemoryManager) GetCoWManager() *cow.CopyOnWrite   { return mm.cowManager }

func (mm *MemoryManager) GetAlgorithm() algorithms.PageReplacementAlgorithm {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.algorithm
}

func (mm *MemoryManager) SetEventCallback(callback func(event string, data map[string]interface{})) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	mm.eventCallback = callback
}

func (mm *MemoryManager) emitEvent(event string, data map[string]interface{}) {
	if mm.eventCallback == nil {
		return
	}
	select {
	case mm.eventCh <- eventMsg{event: event, data: data}:
	default:
		// Channel full: drop rather than blocking the caller (BUG-09 fix).
	}
}

func (mm *MemoryManager) eventWorker(ch chan eventMsg) {
	// ch is captured by value so this goroutine never reads mm.eventCh field.
	for msg := range ch {
		cb := mm.eventCallback
		if cb != nil {
			cb(msg.event, msg.data)
		}
	}
}

func (mm *MemoryManager) SetFutureAccesses(accesses []uint64) {
	mm.mu.RLock()
	alg := mm.algorithm
	mm.mu.RUnlock()

	if opt, ok := alg.(*algorithms.Optimal); ok {
		opt.SetFutureAccesses(accesses)
	}
	if optp, ok := alg.(*algorithms.OptPlus); ok {
		optp.SetFutureAccesses(accesses)
	}
}

func (mm *MemoryManager) Reset() {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	mm.frameTable.Clear()
	mm.tlb.Clear()
	mm.cowManager.Reset()
	mm.compressionManager.Reset()
	mm.clusterManager.ClearClusters("")
	mm.pageTables = make(map[string]*PageTable)
	mm.multiLevelPT = make(map[string]*MultiLevelPageTable)
	mm.processes = make(map[string]*models.Process)
	mm.recentAccesses = make(map[string][]uint64)
	mm.metrics.Reset()
	mm.algorithm.Reset()

	mm.emitEvent("system_reset", map[string]interface{}{})
}

func (mm *MemoryManager) selectLocalNode(processID string) int32 {
	return int32(hashString(processID) % 2)
}

func hashString(s string) uint64 {
	var h uint64
	for i := 0; i < len(s); i++ {
		h = h*31 + uint64(s[i])
	}
	return h
}
