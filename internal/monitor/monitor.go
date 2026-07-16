package monitor

import (
	"sync"
	"time"

	"github.com/page-replacement-cow/internal/memory"
	"github.com/page-replacement-cow/internal/process"
	"github.com/page-replacement-cow/pkg/models"
)

// Monitor provides real-time monitoring of the system
type Monitor struct {
	processManager *process.ProcessManager
	memoryManager  *memory.MemoryManager

	// Historical data
	history      []HistoricalSnapshot
	maxHistory   int
	historyMu    sync.RWMutex

	// Event stream
	events       []Event
	maxEvents    int
	eventsMu     sync.RWMutex
	eventSubs    map[int]chan Event
	nextSubID    int
	eventSubsMu  sync.Mutex

	// Thrashing detection
	thrashThreshold float64      // fault rate above which thrashing is declared (0–1)
	thrashWindow    int          // number of snapshots in the detection window
	isThrashing     bool
	thrashedAt      time.Time
	thrashMu        sync.RWMutex
}

// HistoricalSnapshot represents a point-in-time snapshot
type HistoricalSnapshot struct {
	Timestamp time.Time
	Metrics   *models.MetricsSnapshot
}

// Event represents a system event
type Event struct {
	Timestamp time.Time
	Type      string
	Data      map[string]interface{}
}

// NewMonitor creates a new monitor
func NewMonitor(pm *process.ProcessManager, mm *memory.MemoryManager) *Monitor {
	mon := &Monitor{
		processManager:  pm,
		memoryManager:   mm,
		history:         make([]HistoricalSnapshot, 0),
		maxHistory:      1000,
		events:          make([]Event, 0),
		maxEvents:       500,
		eventSubs:       make(map[int]chan Event),
		nextSubID:       1,
		thrashThreshold: 0.8,
		thrashWindow:    5,
	}

	// Set up event callback
	mm.SetEventCallback(mon.handleEvent)

	return mon
}

// handleEvent handles events from the memory manager
func (mon *Monitor) handleEvent(eventType string, data map[string]interface{}) {
	event := Event{
		Timestamp: time.Now(),
		Type:      eventType,
		Data:      data,
	}

	// Add to event history
	mon.eventsMu.Lock()
	mon.events = append(mon.events, event)
	if len(mon.events) > mon.maxEvents {
		mon.events = mon.events[1:]
	}
	mon.eventsMu.Unlock()

	// Broadcast to subscribers
	mon.eventSubsMu.Lock()
	for _, ch := range mon.eventSubs {
		select {
		case ch <- event:
		default:
			// Channel full, skip
		}
	}
	mon.eventSubsMu.Unlock()
}

// CaptureSnapshot captures a snapshot of current metrics
func (mon *Monitor) CaptureSnapshot() {
	snapshot := HistoricalSnapshot{
		Timestamp: time.Now(),
		Metrics:   mon.memoryManager.GetMetrics(),
	}

	mon.historyMu.Lock()
	mon.history = append(mon.history, snapshot)
	if len(mon.history) > mon.maxHistory {
		mon.history = mon.history[1:]
	}
	mon.historyMu.Unlock()

	mon.detectThrashing()
}

// detectThrashing computes the incremental fault rate over the last thrashWindow
// snapshots and fires a "thrashing_detected" event on the rising edge.
func (mon *Monitor) detectThrashing() {
	mon.historyMu.RLock()
	n := len(mon.history)
	if n < 2 {
		mon.historyMu.RUnlock()
		return
	}
	wStart := n - mon.thrashWindow
	if wStart < 0 {
		wStart = 0
	}
	oldFaults := mon.history[wStart].Metrics.PageFaults
	newFaults := mon.history[n-1].Metrics.PageFaults
	oldAccesses := mon.history[wStart].Metrics.TotalAccesses
	newAccesses := mon.history[n-1].Metrics.TotalAccesses
	mon.historyMu.RUnlock()

	deltaAccesses := newAccesses - oldAccesses
	if deltaAccesses == 0 {
		return
	}
	windowFaultRate := float64(newFaults-oldFaults) / float64(deltaAccesses)

	mon.thrashMu.Lock()
	wasThrshing := mon.isThrashing
	if windowFaultRate >= mon.thrashThreshold {
		if !mon.isThrashing {
			mon.isThrashing = true
			mon.thrashedAt = time.Now()
		}
	} else {
		mon.isThrashing = false
	}
	thrashing := mon.isThrashing
	mon.thrashMu.Unlock()

	// Emit only on the rising edge (false → true transition).
	if thrashing && !wasThrshing {
		mon.handleEvent("thrashing_detected", map[string]interface{}{
			"window_fault_rate": windowFaultRate,
			"threshold":         mon.thrashThreshold,
			"window_snapshots":  mon.thrashWindow,
		})
	}
}

// ThrashingInfo contains a point-in-time view of the thrashing detector state.
type ThrashingInfo struct {
	IsThrashing     bool      `json:"is_thrashing"`
	WindowFaultRate float64   `json:"window_fault_rate"`
	Threshold       float64   `json:"threshold"`
	WindowSize      int       `json:"window_size"`
	DetectedAt      time.Time `json:"detected_at,omitempty"`
}

// GetThrashingInfo returns current thrashing detector state.
func (mon *Monitor) GetThrashingInfo() ThrashingInfo {
	mon.thrashMu.RLock()
	thrashing := mon.isThrashing
	detectedAt := mon.thrashedAt
	mon.thrashMu.RUnlock()

	// Compute current window fault rate for the response.
	mon.historyMu.RLock()
	n := len(mon.history)
	var windowFaultRate float64
	if n >= 2 {
		wStart := n - mon.thrashWindow
		if wStart < 0 {
			wStart = 0
		}
		deltaA := mon.history[n-1].Metrics.TotalAccesses - mon.history[wStart].Metrics.TotalAccesses
		if deltaA > 0 {
			deltaF := mon.history[n-1].Metrics.PageFaults - mon.history[wStart].Metrics.PageFaults
			windowFaultRate = float64(deltaF) / float64(deltaA)
		}
	}
	mon.historyMu.RUnlock()

	return ThrashingInfo{
		IsThrashing:     thrashing,
		WindowFaultRate: windowFaultRate,
		Threshold:       mon.thrashThreshold,
		WindowSize:      mon.thrashWindow,
		DetectedAt:      detectedAt,
	}
}

// IsThrashing returns whether thrashing is currently detected.
func (mon *Monitor) IsThrashing() bool {
	mon.thrashMu.RLock()
	defer mon.thrashMu.RUnlock()
	return mon.isThrashing
}

// GetHistory returns historical snapshots
func (mon *Monitor) GetHistory(last int) []HistoricalSnapshot {
	mon.historyMu.RLock()
	defer mon.historyMu.RUnlock()

	if last <= 0 || last > len(mon.history) {
		last = len(mon.history)
	}

	start := len(mon.history) - last
	if start < 0 {
		start = 0
	}

	history := make([]HistoricalSnapshot, last)
	copy(history, mon.history[start:])
	return history
}

// GetEvents returns recent events
func (mon *Monitor) GetEvents(last int) []Event {
	mon.eventsMu.RLock()
	defer mon.eventsMu.RUnlock()

	if last <= 0 || last > len(mon.events) {
		last = len(mon.events)
	}

	start := len(mon.events) - last
	if start < 0 {
		start = 0
	}

	events := make([]Event, last)
	copy(events, mon.events[start:])
	return events
}

// SubscribeEvents subscribes to event stream
func (mon *Monitor) SubscribeEvents(bufferSize int) (int, <-chan Event) {
	mon.eventSubsMu.Lock()
	defer mon.eventSubsMu.Unlock()

	subID := mon.nextSubID
	mon.nextSubID++

	ch := make(chan Event, bufferSize)
	mon.eventSubs[subID] = ch

	return subID, ch
}

// UnsubscribeEvents unsubscribes from event stream
func (mon *Monitor) UnsubscribeEvents(subID int) {
	mon.eventSubsMu.Lock()
	defer mon.eventSubsMu.Unlock()

	if ch, exists := mon.eventSubs[subID]; exists {
		close(ch)
		delete(mon.eventSubs, subID)
	}
}

// GetSystemStatus returns current system status
func (mon *Monitor) GetSystemStatus() SystemStatus {
	metrics := mon.memoryManager.GetMetrics()
	processes := mon.processManager.GetAllProcesses()

	return SystemStatus{
		Timestamp:      time.Now(),
		Metrics:        metrics,
		ProcessCount:   len(processes),
		AlgorithmName:  mon.memoryManager.GetAlgorithm().GetName(),
		Uptime:         metrics.Uptime,
	}
}

// GetProcessDetails returns detailed information about all processes
func (mon *Monitor) GetProcessDetails() []ProcessDetail {
	processes := mon.processManager.GetAllProcesses()
	details := make([]ProcessDetail, len(processes))

	for i, proc := range processes {
		pageTable, _ := mon.memoryManager.GetPageTable(proc.ID)
		var stats memory.PageTableStats
		if pageTable != nil {
			stats = pageTable.GetStats()
		}

		details[i] = ProcessDetail{
			ID:             proc.ID,
			Name:           proc.Name,
			State:          proc.GetStateString(),
			Priority:       proc.Priority,
			PageFaults:     proc.PageFaults.Load(),
			PageHits:       proc.PageHits.Load(),
			MemoryAccesses: proc.MemoryAccesses.Load(),
			CoWCopies:      proc.CoWCopies.Load(),
			PageFaultRate:  proc.GetPageFaultRate(),
			PageHitRate:    proc.GetPageHitRate(),
			TotalPages:     stats.TotalPages,
			PresentPages:   stats.PresentPages,
			SharedPages:    stats.SharedPages,
			DirtyPages:     stats.DirtyPages,
		}
	}

	return details
}

// GetFrameDetails returns detailed information about frames
func (mon *Monitor) GetFrameDetails() []FrameDetail {
	frameTable := mon.memoryManager.GetFrameTable()
	frames := frameTable.GetAllFrames()

	details := make([]FrameDetail, len(frames))

	for i, frame := range frames {
		details[i] = FrameDetail{
			ID:          frame.ID,
			Free:        frame.IsFree(),
			Pinned:      frame.IsPinned(),
			Modified:    frame.IsModified(),
			PageID:      frame.GetPageID(),
			ProcessID:   frame.GetProcessID(),
			LoadedAt:    frame.GetLoadedTime(),
			LastAccess:  frame.GetLastAccessTime(),
			AccessCount: frame.GetAccessCount(),
			Age:         frame.GetAge(),
		}
	}

	return details
}

// SystemStatus contains system status information
type SystemStatus struct {
	Timestamp     time.Time
	Metrics       *models.MetricsSnapshot
	ProcessCount  int
	AlgorithmName string
	Uptime        time.Duration
}

// ProcessDetail contains detailed process information
type ProcessDetail struct {
	ID             string
	Name           string
	State          string
	Priority       int32
	PageFaults     int64
	PageHits       int64
	MemoryAccesses int64
	CoWCopies      int64
	PageFaultRate  float64
	PageHitRate    float64
	TotalPages     int
	PresentPages   int
	SharedPages    int
	DirtyPages     int
}

// FrameDetail contains detailed frame information
type FrameDetail struct {
	ID          int32
	Free        bool
	Pinned      bool
	Modified    bool
	PageID      uint64
	ProcessID   string
	LoadedAt    time.Time
	LastAccess  time.Time
	AccessCount int64
	Age         int64
}

// StartPeriodicCapture starts periodic snapshot capture
func (mon *Monitor) StartPeriodicCapture(interval time.Duration) chan struct{} {
	stopCh := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				mon.CaptureSnapshot()
			case <-stopCh:
				return
			}
		}
	}()

	return stopCh
}

// ClearHistory clears historical data
func (mon *Monitor) ClearHistory() {
	mon.historyMu.Lock()
	mon.history = make([]HistoricalSnapshot, 0)
	mon.historyMu.Unlock()

	mon.eventsMu.Lock()
	mon.events = make([]Event, 0)
	mon.eventsMu.Unlock()
}
