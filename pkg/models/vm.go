package models

type PageSize int32

const (
	PageSizeRegular PageSize = iota
	PageSizeHuge
	PageSizeGigantic
)

func (ps PageSize) GetSizeBytes() int64 {
	switch ps {
	case PageSizeHuge:
		return 2 * 1024 * 1024
	case PageSizeGigantic:
		return 1 * 1024 * 1024 * 1024
	default:
		return 4 * 1024
	}
}

func (ps PageSize) GetRegularPageCount() int {
	switch ps {
	case PageSizeHuge:
		return 512
	case PageSizeGigantic:
		return 262144
	default:
		return 1
	}
}

func (ps PageSize) String() string {
	switch ps {
	case PageSizeHuge:
		return "2MB"
	case PageSizeGigantic:
		return "1GB"
	default:
		return "4KB"
	}
}

type NumaNode struct {
	ID             int32
	Name           string
	AccessCostNs   int64
	LocalFrames    int32
	TotalFrames    int32
}

func NewNumaNode(id int32, name string, accessCostNs int64, frameCount int32) *NumaNode {
	return &NumaNode{
		ID:           id,
		Name:         name,
		AccessCostNs: accessCostNs,
		LocalFrames:  frameCount,
		TotalFrames:  frameCount,
	}
}

type CompressionStats struct {
	UncompressedBytes int64
	CompressedBytes   int64
	CompressionRatio  float64
	PagesCompressed   int64
	PagesDecompressed int64
}

type PageCluster struct {
	Pages       []*Page
	AnchorPage  uint64
	ClusterSize int
	Sequential  bool
	ProcessID   string // owner process
}

type WorkingSetEntry struct {
	PageID    uint64
	LastUsed  int64
	InWindow  bool
}