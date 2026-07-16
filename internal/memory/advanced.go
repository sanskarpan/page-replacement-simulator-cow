package memory

import (
	"fmt"
	"sync"
	"time"

	"github.com/page-replacement-cow/pkg/models"
)

type NumaManager struct {
	nodes       []*models.NumaNode
	mu          sync.RWMutex
}

func NewNumaManager() *NumaManager {
	return &NumaManager{
		nodes: make([]*models.NumaNode, 0),
	}
}

func (nm *NumaManager) AddNode(node *models.NumaNode) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	nm.nodes = append(nm.nodes, node)
}

func (nm *NumaManager) GetNode(nodeID int32) (*models.NumaNode, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	for _, n := range nm.nodes {
		if n.ID == nodeID {
			return n, nil
		}
	}
	return nil, fmt.Errorf("NUMA node %d not found", nodeID)
}

func (nm *NumaManager) GetClosestNode(targetID int32) (*models.NumaNode, int64) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	if len(nm.nodes) == 0 {
		return nil, 0
	}

	target, err := nm.getNodeLocked(targetID)
	if err != nil {
		return nm.nodes[0], 0
	}

	// Start with the target node as the best option using its local access cost
	// as the baseline. A remote node only wins if it is genuinely cheaper.
	best := target
	bestCost := target.AccessCostNs

	for _, n := range nm.nodes {
		if n.ID == targetID {
			continue
		}
		cost := nm.estimateAccessCost(target, n)
		if cost < bestCost {
			best = n
			bestCost = cost
		}
	}

	return best, bestCost
}

func (nm *NumaManager) getNodeLocked(nodeID int32) (*models.NumaNode, error) {
	for _, n := range nm.nodes {
		if n.ID == nodeID {
			return n, nil
		}
	}
	return nil, fmt.Errorf("node not found")
}

func (nm *NumaManager) estimateAccessCost(from, to *models.NumaNode) int64 {
	if from.ID == to.ID {
		return to.AccessCostNs
	}
	return to.AccessCostNs * 2
}

func (nm *NumaManager) AllocateFrameOnNode(nodeID int32, pageID uint64, processID string) (*models.Frame, error) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	node, err := nm.getNodeLocked(nodeID)
	if err != nil {
		return nil, err
	}

	if node.LocalFrames <= 0 {
		return nil, fmt.Errorf("NUMA node %d has no free frames", nodeID)
	}

	node.LocalFrames--
	frame := models.NewFrame(int32(pageID))
	frame.AllocateNuma(pageID, processID, nodeID)
	return frame, nil
}

func (nm *NumaManager) GetNodes() []*models.NumaNode {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	result := make([]*models.NumaNode, len(nm.nodes))
	for i, n := range nm.nodes {
		nodeCopy := *n
		result[i] = &nodeCopy
	}
	return result
}

type PageClusterManager struct {
	clusters        map[uint64]*models.PageCluster
	minClusterSize  int
	maxClusterSize  int
	sequentialBoost int
	mu              sync.RWMutex
}

func NewPageClusterManager(minSize, maxSize int) *PageClusterManager {
	return &PageClusterManager{
		clusters:        make(map[uint64]*models.PageCluster),
		minClusterSize:  minSize,
		maxClusterSize:  maxSize,
		sequentialBoost: 8,
	}
}

func (pcm *PageClusterManager) DetectSequential(processID string, pages []uint64) *models.PageCluster {
	pcm.mu.Lock()
	defer pcm.mu.Unlock()

	if len(pages) < 3 {
		return nil
	}

	for i := 1; i < len(pages)-1; i++ {
		if pages[i]-pages[i-1] != 1 || pages[i+1]-pages[i] != 1 {
			return nil
		}
	}

	cluster := &models.PageCluster{
		AnchorPage:  pages[0],
		ClusterSize: pcm.maxClusterSize,
		Sequential:  true,
		Pages:       make([]*models.Page, 0, pcm.maxClusterSize),
		ProcessID:   processID,
	}
	pcm.clusters[pages[0]] = cluster

	return cluster
}

func (pcm *PageClusterManager) GetPrefetchPages(anchorPage uint64) []uint64 {
	pcm.mu.RLock()
	defer pcm.mu.RUnlock()

	cluster, exists := pcm.clusters[anchorPage]
	if !exists || !cluster.Sequential {
		return nil
	}

	pages := make([]uint64, cluster.ClusterSize)
	for i := 0; i < cluster.ClusterSize; i++ {
		pages[i] = anchorPage + uint64(i) + 1
	}
	return pages
}

func (pcm *PageClusterManager) ClearClusters(processID string) {
	pcm.mu.Lock()
	defer pcm.mu.Unlock()
	if processID == "" {
		pcm.clusters = make(map[uint64]*models.PageCluster)
		return
	}
	for anchor, cluster := range pcm.clusters {
		if cluster.ProcessID == processID {
			delete(pcm.clusters, anchor)
		}
	}
}

type CompressedPage struct {
	PageID         uint64
	OriginalSize   int64
	CompressedSize int64
	Data           []byte
	CreatedAt      time.Time
}

type CompressionManager struct {
	compressedPages map[uint64]*CompressedPage
	totalOriginal   int64
	totalCompressed int64
	pagesCompressed int64
	pagesDecompressed int64
	compressionRatio float64
	minRatio        float64
	mu              sync.RWMutex
}

func NewCompressionManager(minRatio float64) *CompressionManager {
	return &CompressionManager{
		compressedPages: make(map[uint64]*CompressedPage),
		minRatio:        minRatio,
	}
}

func (cm *CompressionManager) ShouldCompress(pageID uint64) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	_, exists := cm.compressedPages[pageID]
	return !exists
}

func (cm *CompressionManager) CompressPage(pageID uint64, data []byte) *CompressedPage {
	if len(data) == 0 {
		return nil
	}
	// Simulate 50% compression (representative of LZ-style on typical pages).
	compressedSize := int64(len(data)) / 2

	// Reject if the compressed/original ratio meets or exceeds the threshold
	// (i.e. not enough savings to justify compression overhead).
	if float64(compressedSize)/float64(len(data)) >= cm.minRatio {
		return nil
	}

	cp := &CompressedPage{
		PageID:         pageID,
		OriginalSize:   int64(len(data)),
		CompressedSize: compressedSize,
		Data:           make([]byte, compressedSize),
		CreatedAt:      time.Now(),
	}

	cm.mu.Lock()
	cm.compressedPages[pageID] = cp
	cm.totalOriginal += cp.OriginalSize
	cm.totalCompressed += cp.CompressedSize
	cm.pagesCompressed++
	if cm.totalOriginal > 0 {
		cm.compressionRatio = float64(cm.totalCompressed) / float64(cm.totalOriginal)
	}
	cm.mu.Unlock()

	return cp
}

func (cm *CompressionManager) DecompressPage(pageID uint64) *CompressedPage {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cp, exists := cm.compressedPages[pageID]
	if !exists {
		return nil
	}

	delete(cm.compressedPages, pageID)
	cm.totalOriginal -= cp.OriginalSize
	cm.totalCompressed -= cp.CompressedSize
	cm.pagesDecompressed++
	if cm.totalOriginal > 0 {
		cm.compressionRatio = float64(cm.totalCompressed) / float64(cm.totalOriginal)
	} else {
		cm.compressionRatio = 0
	}

	return cp
}

func (cm *CompressionManager) GetStats() models.CompressionStats {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return models.CompressionStats{
		UncompressedBytes: cm.totalOriginal,
		CompressedBytes:   cm.totalCompressed,
		CompressionRatio:  cm.compressionRatio,
		PagesCompressed:   cm.pagesCompressed,
		PagesDecompressed: cm.pagesDecompressed,
	}
}

func (cm *CompressionManager) Reset() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.compressedPages = make(map[uint64]*CompressedPage)
	cm.totalOriginal = 0
	cm.totalCompressed = 0
	cm.pagesCompressed = 0
	cm.pagesDecompressed = 0
	cm.compressionRatio = 0
}