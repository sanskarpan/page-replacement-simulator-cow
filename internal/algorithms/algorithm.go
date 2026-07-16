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

// ValidAlgorithmNames lists every recognised algorithm name, in the canonical order
// used for help text and error messages.
var ValidAlgorithmNames = []string{
	"LRU", "CLOCK", "LFU", "FIFO", "Optimal", "Random",
	"ARC", "CAR", "WSClock", "PFF", "OPT+", "NRU",
}

// ParseAlgorithmType converts a human-readable algorithm name to its AlgorithmType.
// Returns (type, true) on success, (0, false) when the name is unrecognised.
// This is the single authoritative mapping; all callers (CLI, API, simulator) should
// use this function instead of duplicating the switch.
func ParseAlgorithmType(name string) (AlgorithmType, bool) {
	switch name {
	case "LRU":
		return AlgorithmLRU, true
	case "CLOCK":
		return AlgorithmCLOCK, true
	case "LFU":
		return AlgorithmLFU, true
	case "FIFO":
		return AlgorithmFIFO, true
	case "Optimal":
		return AlgorithmOptimal, true
	case "Random":
		return AlgorithmRandom, true
	case "ARC":
		return AlgorithmARC, true
	case "CAR":
		return AlgorithmCAR, true
	case "WSClock":
		return AlgorithmWSClock, true
	case "PFF":
		return AlgorithmPFF, true
	case "OPT+":
		return AlgorithmOPTPlus, true
	case "NRU":
		return AlgorithmNRU, true
	default:
		return 0, false
	}
}

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
