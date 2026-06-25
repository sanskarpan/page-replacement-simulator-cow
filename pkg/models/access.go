package models

import "time"

// AccessType represents the type of memory access
type AccessType int

const (
	AccessRead AccessType = iota
	AccessWrite
	AccessExecute
)

// MemoryAccess represents a memory access operation
type MemoryAccess struct {
	ProcessID    string
	VirtualPage  uint64
	AccessType   AccessType
	Timestamp    time.Time
	PageFault    bool
	CoWTriggered bool
	LatencyNs    int64
}

// NewMemoryAccess creates a new memory access
func NewMemoryAccess(processID string, virtualPage uint64, accessType AccessType) *MemoryAccess {
	return &MemoryAccess{
		ProcessID:    processID,
		VirtualPage:  virtualPage,
		AccessType:   accessType,
		Timestamp:    time.Now(),
		PageFault:    false,
		CoWTriggered: false,
		LatencyNs:    0,
	}
}

// IsWrite returns true if this is a write access
func (a *MemoryAccess) IsWrite() bool {
	return a.AccessType == AccessWrite
}

// IsRead returns true if this is a read access
func (a *MemoryAccess) IsRead() bool {
	return a.AccessType == AccessRead
}

// GetAccessTypeString returns a string representation of the access type
func (a *MemoryAccess) GetAccessTypeString() string {
	switch a.AccessType {
	case AccessRead:
		return "Read"
	case AccessWrite:
		return "Write"
	case AccessExecute:
		return "Execute"
	default:
		return "Unknown"
	}
}

// AccessPattern represents a pattern of memory accesses
type AccessPattern int

const (
	PatternSequential AccessPattern = iota
	PatternRandom
	PatternLocality   // Temporal locality
	PatternLooping    // Loop pattern
	PatternMixed
)

// GetAccessPatternString returns a string representation of the access pattern
func GetAccessPatternString(pattern AccessPattern) string {
	switch pattern {
	case PatternSequential:
		return "Sequential"
	case PatternRandom:
		return "Random"
	case PatternLocality:
		return "Locality"
	case PatternLooping:
		return "Looping"
	case PatternMixed:
		return "Mixed"
	default:
		return "Unknown"
	}
}
