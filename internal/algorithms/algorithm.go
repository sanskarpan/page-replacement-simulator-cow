// Package algorithms implements 12 page-replacement algorithms: LRU, CLOCK,
// LFU, FIFO, Optimal, Random, ARC, CAR, WSClock, PFF, OPT+, and NRU.
// All implementations satisfy the Algorithm interface so the MemoryManager
// can swap them at runtime.
package algorithms

import (
	"github.com/page-replacement-cow/pkg/models"
)

type PageReplacementAlgorithm interface {
	SelectVictim(frames []*models.Frame) (*models.Frame, error)
	OnPageAccess(frame *models.Frame, write bool)
	OnPageFault(frame *models.Frame)
	OnPageEviction(frame *models.Frame)
	GetName() string
	Reset()
}

type AlgorithmType int

const (
	AlgorithmLRU AlgorithmType = iota
	AlgorithmCLOCK
	AlgorithmLFU
	AlgorithmFIFO
	AlgorithmOptimal
	AlgorithmRandom
	AlgorithmARC
	AlgorithmCAR
	AlgorithmWSClock
	AlgorithmPFF
	AlgorithmOPTPlus
	AlgorithmNRU
)

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
	case AlgorithmARC:
		return "ARC"
	case AlgorithmCAR:
		return "CAR"
	case AlgorithmWSClock:
		return "WSClock"
	case AlgorithmPFF:
		return "PFF"
	case AlgorithmOPTPlus:
		return "OPT+"
	case AlgorithmNRU:
		return "NRU"
	default:
		return "Unknown"
	}
}
