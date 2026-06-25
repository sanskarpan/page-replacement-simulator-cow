package algorithms

import (
	"github.com/page-replacement-cow/pkg/models"
)

// PageReplacementAlgorithm defines the interface for page replacement algorithms
type PageReplacementAlgorithm interface {
	// SelectVictim selects a page to evict from the given list of frames
	SelectVictim(frames []*models.Frame) (*models.Frame, error)

	// OnPageAccess is called when a page is accessed
	OnPageAccess(frame *models.Frame, write bool)

	// OnPageFault is called when a page fault occurs
	OnPageFault(frame *models.Frame)

	// OnPageEviction is called when a page is evicted
	OnPageEviction(frame *models.Frame)

	// GetName returns the name of the algorithm
	GetName() string

	// Reset resets the algorithm state
	Reset()
}

// AlgorithmType represents the type of page replacement algorithm
type AlgorithmType int

const (
	AlgorithmLRU AlgorithmType = iota
	AlgorithmCLOCK
	AlgorithmLFU
	AlgorithmFIFO
	AlgorithmOptimal
	AlgorithmRandom
)

// GetAlgorithmName returns the name of an algorithm type
func GetAlgorithmName(algType AlgorithmType) string {
	switch algType {
	case AlgorithmLRU:
		return "LRU"
	case AlgorithmCLOCK:
		return "CLOCK"
	case AlgorithmLFU:
		return "LFU"
	case AlgorithmFIFO:
		return "FIFO"
	case AlgorithmOptimal:
		return "Optimal"
	case AlgorithmRandom:
		return "Random"
	default:
		return "Unknown"
	}
}
