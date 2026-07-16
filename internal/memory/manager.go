// Package memory implements the virtual-memory subsystem: frame allocation,
// page-table management, TLB, Copy-on-Write, huge pages, NUMA placement,
// memory compression, and page-clustering.  The central type is MemoryManager.
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
	eventCallback   func(event string, data map[string]interface{})
	eventCallbackMu sync.RWMutex // separate lock so eventWorker never blocks under mm.mu
	eventCh         chan eventMsg
	closeOnce       sync.Once // guards Close() idempotency

	numaManager          *NumaManager
	compressionManager   *CompressionManager
	clusterManager       *PageClusterManager
	recentAccesses       map[string][]uint64
	numaEnabled          bool
	compressionEnabled   bool
	clusteringEnabled    bool

	hugePages           map[string]map[uint64]int32 // processID → hugePageIndex → frameID
	workingSetWindow    int                          // number of accesses in the working-set window
	workingSetAccesses  map[string][]uint64          // per-process access ring buffer
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

		hugePages:          make(map[string]map[uint64]int32),
		workingSetWindow:   10,
		workingSetAccesses: make(map[string][]uint64),
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
	case algorithms.AlgorithmNRU:
		mm.algorithm = algorithms.NewNRU()
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
	mm.hugePages[process.ID] = make(map[uint64]int32)
	mm.workingSetAccesses[process.ID] = make([]uint64, 0, mm.workingSetWindow*4)
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
	delete(mm.hugePages, processID)
	delete(mm.workingSetAccesses, processID)

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
	if !exists {
		mm.mu.RUnlock()
		return fmt.Errorf("process %s not found", processID)
	}

	process.RecordMemoryAccess()

	// Read-only TLB fast path: held under mm.mu.RLock so mm.algorithm,
	// the frame table, and page-table maps are all stable for this read.
	if !write {
		if frameNum, hit := mm.tlb.Lookup(processID, virtualPage); hit {
			frame, _ := mm.frameTable.GetFrame(frameNum)
			if frame != nil {
				mm.algorithm.OnPageAccess(frame, false)
				process.RecordPageHit()
				mm.metrics.RecordPageHit()
				mm.mu.RUnlock()
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
	mm.mu.RUnlock()

	// Write-lock section: handles TLB misses, page faults, and write CoW.
	// Guards pageTables, recentAccesses, and pageTable.Entries (BUG-02, BUG-03, BUG-04 fix).
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// Working-set and cluster tracking under write lock so maps are fully guarded (BUG-03 fix).
	mm.updateWorkingSet(processID, virtualPage)
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

	// PFF: proactively evict frames when resident set exceeds the algorithm's target.
	if pff, ok := mm.algorithm.(*algorithms.PFF); ok {
		mm.enforcePFFResident(pff.GetTargetResidentSet())
	}

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
	if len(pages) < 3 {
		return
	}
	// anchor = first page of the last 3-page sequential window detected by DetectSequential
	anchor := pages[len(pages)-3]
	prefetchPages := mm.clusterManager.GetPrefetchPages(processID, anchor)
	if len(prefetchPages) == 0 {
		return
	}

	pageTable, ok := mm.pageTables[processID]
	if !ok {
		return
	}

	// Prefetch up to 2 non-resident pages from the cluster list, skipping any
	// that are already in memory (possible when the last few accesses are also
	// in the prefetch range).
	const maxPrefetch = 2
	prefetched := 0
	for i := 0; i < len(prefetchPages) && prefetched < maxPrefetch; i++ {
		p := pageTable.GetOrCreatePage(prefetchPages[i])
		if p.IsPresent() {
			continue // already resident, keep scanning for non-resident ones
		}

		frame, err := mm.allocateFrameForPage(p.ID, processID)
		if err != nil {
			return // no free frames, skip prefetch rather than stalling
		}
		p.SetFrame(frame.ID)
		if mpt, ok := mm.multiLevelPT[processID]; ok {
			mpt.SetEntry(p.ID<<12, frame.ID, false)
		}
		mm.algorithm.OnPageFault(frame)
		mm.tlb.Insert(processID, p.ID, frame.ID)
		prefetched++
	}
}

func (mm *MemoryManager) handlePageFault(processID string, page *models.Page, write bool) error {
	if mm.compressionEnabled {
		cp := mm.compressionManager.DecompressPage(page.ID)
		if cp != nil {
			frame, err := mm.allocateFrameForPage(page.ID, processID)
			if err != nil {
				if evErr := mm.atomicEvictAndAlloc(page, processID); evErr != nil {
					// Restore compressed entry to prevent data loss when frame
					// allocation fails after DecompressPage deleted it.
					mm.compressionManager.RestoreCompressed(cp)
					return fmt.Errorf("handlePageFault (compressed): %w", evErr)
				}
			} else {
				page.SetFrame(frame.ID)
				if mpt, ok := mm.multiLevelPT[processID]; ok {
					mpt.SetEntry(page.ID<<12, frame.ID, false)
				}
				mm.algorithm.OnPageFault(frame)
				mm.tlb.Insert(processID, page.ID, frame.ID)
			}
			// A write to a shared compressed page must still trigger CoW so the
			// writer gets its own private copy rather than corrupting the shared frame.
			if write && page.IsShared() {
				f, err := mm.frameTable.GetFrame(page.GetFrame())
				if err == nil && f != nil {
					return mm.handleCoW(processID, page, f)
				}
			}
			return nil
		}
	}

	frame, err := mm.allocateFrameForPage(page.ID, processID)
	if err != nil {
		if evErr := mm.atomicEvictAndAlloc(page, processID); evErr != nil {
			return fmt.Errorf("handlePageFault: %w", evErr)
		}
	} else {
		page.SetFrame(frame.ID)
		if mpt, ok := mm.multiLevelPT[processID]; ok {
			mpt.SetEntry(page.ID<<12, frame.ID, false)
		}
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
		frame.SetNumaNodeID(mm.selectLocalNode(processID))
	}
	page.SetFrame(frame.ID)
	if mpt, ok := mm.multiLevelPT[processID]; ok {
		mpt.SetEntry(page.ID<<12, frame.ID, false)
	}
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
		newFrame.SetNumaNodeID(mm.selectLocalNode(processID))
	}

	// Register the new CoW frame with the replacement algorithm so ARC/CAR
	// T1/T2 lists stay coherent (BUG-06 fix).
	mm.algorithm.OnPageFault(newFrame)

	newPage.SetFrame(newFrame.ID)

	pageTable := mm.pageTables[processID]
	pageTable.ReplaceEntry(page.ID, newPage)

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
	if mpt, ok := mm.multiLevelPT[processID]; ok {
		mpt.InvalidateEntry(pageID << 12)
	}
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
	mm.hugePages[childID] = make(map[uint64]int32)
	mm.workingSetAccesses[childID] = make([]uint64, 0, mm.workingSetWindow*4)
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
	mm.eventCallbackMu.Lock()
	mm.eventCallback = callback
	mm.eventCallbackMu.Unlock()
}

// Close stops the background event worker. Safe to call multiple times.
// After Close, emitEvent calls are no-ops (sends to a closed channel are
// caught by recover). Managers created in tests should defer mm.Close().
func (mm *MemoryManager) Close() {
	mm.closeOnce.Do(func() { close(mm.eventCh) })
}

func (mm *MemoryManager) emitEvent(event string, data map[string]interface{}) {
	mm.eventCallbackMu.RLock()
	cb := mm.eventCallback
	mm.eventCallbackMu.RUnlock()
	if cb == nil {
		return
	}
	// recover catches the "send on closed channel" panic that can occur if
	// Close() is called concurrently with an in-flight emitEvent.
	defer func() { recover() }() //nolint:errcheck
	select {
	case mm.eventCh <- eventMsg{event: event, data: data}:
	default:
		mm.metrics.DroppedEvents.Add(1)
	}
}

func (mm *MemoryManager) eventWorker(ch chan eventMsg) {
	// ch is captured by value so this goroutine never reads mm.eventCh field.
	for msg := range ch {
		mm.eventCallbackMu.RLock()
		cb := mm.eventCallback
		mm.eventCallbackMu.RUnlock()
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
	mm.hugePages = make(map[string]map[uint64]int32)
	mm.workingSetAccesses = make(map[string][]uint64)
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

// allocateFrameForPage allocates a physical frame for pageID, using NUMA-local
// range selection when NUMA is enabled.
func (mm *MemoryManager) allocateFrameForPage(pageID uint64, processID string) (*models.Frame, error) {
	if mm.numaEnabled {
		nodeID := mm.selectLocalNode(processID)
		node0End := mm.numFrames / 2
		node1End := mm.numFrames
		var start, end int32
		if nodeID == 0 {
			start = 0
			end = node0End
		} else {
			start = node0End
			end = node1End
		}
		frame, err := mm.frameTable.AllocateFrameInRange(start, end, pageID, processID)
		if err == nil {
			frame.SetNumaNodeID(nodeID)
			return frame, nil
		}
		// NUMA node full — fall through to global allocation
	}
	return mm.frameTable.AllocateFrame(pageID, processID)
}

// MapHugePage allocates a 2MB huge-page mapping for a process and registers it
// in the multi-level page table at L2 granularity.
func (mm *MemoryManager) MapHugePage(processID string, hugePageIdx uint64) (int32, error) {
	// Indices >= 2^43 cause hugePageIdx<<L2Shift (L2Shift=21) to overflow uint64.
	if hugePageIdx > (1<<43)-1 {
		return -1, fmt.Errorf("huge page index %d out of range (max %d)", hugePageIdx, uint64(1<<43)-1)
	}

	mm.mu.Lock()
	defer mm.mu.Unlock()

	if _, ok := mm.pageTables[processID]; !ok {
		return -1, fmt.Errorf("process %s not found", processID)
	}

	// Synthetic page ID outside the normal virtual-page range so it does not
	// collide with regular 4KB page table entries.
	const hugePageBase = uint64(1) << 32
	syntheticID := hugePageBase + hugePageIdx

	frame, err := mm.allocateFrameForPage(syntheticID, processID)
	if err != nil {
		// No free frame — evict one first.
		usedFrames := mm.frameTable.GetUsedFrames()
		victim, verr := mm.algorithm.SelectVictim(usedFrames)
		if verr != nil {
			return -1, fmt.Errorf("no frames available for huge page: %w", verr)
		}
		if verr = mm.evictPage(victim); verr != nil {
			return -1, verr
		}
		frame = victim
		frame.Allocate(syntheticID, processID)
	}

	// Virtual address for huge page N is N × 2MB (L2Shift = 21 bits).
	virtualAddr := hugePageIdx << L2Shift
	if mpt, ok := mm.multiLevelPT[processID]; ok {
		mpt.SetEntry(virtualAddr, frame.ID, true)
	}

	mm.hugePages[processID][hugePageIdx] = frame.ID
	mm.algorithm.OnPageFault(frame)
	mm.metrics.RecordPageFault()

	mm.emitEvent("huge_page_mapped", map[string]interface{}{
		"process_id":      processID,
		"huge_page_index": hugePageIdx,
		"frame_id":        frame.ID,
		"virtual_addr":    virtualAddr,
	})

	return frame.ID, nil
}

// GetHugePages returns all huge-page mappings for a process.
func (mm *MemoryManager) GetHugePages(processID string) ([]map[string]interface{}, error) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	hp, ok := mm.hugePages[processID]
	if !ok {
		return nil, fmt.Errorf("process %s not found", processID)
	}

	result := make([]map[string]interface{}, 0, len(hp))
	for idx, frameID := range hp {
		result = append(result, map[string]interface{}{
			"huge_page_index": idx,
			"frame_id":        frameID,
			"virtual_addr":    idx << L2Shift,
			"size_bytes":      int64(1) << L2Shift,
		})
	}
	return result, nil
}

// updateWorkingSet records virtualPage in the per-process sliding window and
// updates process.WorkingSetSize with the count of unique pages in that window.
// Must be called under mm.mu.Lock().
func (mm *MemoryManager) updateWorkingSet(processID string, virtualPage uint64) {
	ws, ok := mm.workingSetAccesses[processID]
	if !ok {
		return
	}
	ws = append(ws, virtualPage)
	window := mm.workingSetWindow
	if len(ws) > window {
		ws = ws[len(ws)-window:]
	}
	mm.workingSetAccesses[processID] = ws

	// O(n²) uniqueness count avoids a heap allocation per access for the small
	// working-set windows typical in this simulator (window ≤ 64 by default).
	unique := 0
	for i, p := range ws {
		dup := false
		for j := 0; j < i; j++ {
			if ws[j] == p {
				dup = true
				break
			}
		}
		if !dup {
			unique++
		}
	}
	if proc := mm.processes[processID]; proc != nil {
		proc.UpdateWorkingSetSize(int32(unique))
	}
}

// GetWorkingSetInfo returns the current working-set size and window for a process.
func (mm *MemoryManager) GetWorkingSetInfo(processID string) (map[string]interface{}, error) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	proc, ok := mm.processes[processID]
	if !ok {
		return nil, fmt.Errorf("process %s not found", processID)
	}
	return map[string]interface{}{
		"process_id":       processID,
		"working_set_size": proc.GetWorkingSetSize(),
		"window_size":      mm.workingSetWindow,
	}, nil
}

// enforcePFFResident proactively evicts frames until used frames ≤ target.
// Must be called under mm.mu.Lock().
func (mm *MemoryManager) enforcePFFResident(target int32) {
	if target <= 0 {
		return
	}
	usedFrames := mm.frameTable.GetUsedFrames()
	for int32(len(usedFrames)) > target {
		victim, err := mm.algorithm.SelectVictim(usedFrames)
		if err != nil {
			break
		}
		if err := mm.evictPage(victim); err != nil {
			break
		}
		// Remove the evicted frame from the local slice to avoid an
		// extra GetUsedFrames() allocation on each loop iteration.
		for i, f := range usedFrames {
			if f.ID == victim.ID {
				usedFrames = append(usedFrames[:i], usedFrames[i+1:]...)
				break
			}
		}
	}
}
