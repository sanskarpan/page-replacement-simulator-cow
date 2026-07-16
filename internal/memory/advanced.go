package memory

import (
	"fmt"
	"strconv"
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
			nodeCopy := *n
			return &nodeCopy, nil
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

// AllocateFrameOnNode allocates a simulated frame on a specific NUMA node by
// decrementing the node's LocalFrames counter. The returned frame is NOT
// registered in any FrameTable; callers that need FrameTable tracking should
// use MemoryManager.allocateFrameForPage instead.
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
	// Key: processID + "\x00" + anchorPage — prevents cross-process collisions
	// when different processes happen to share the same anchor virtual page number.
	clusters        map[string]*models.PageCluster
	minClusterSize  int
	maxClusterSize  int
	sequentialBoost int
	mu              sync.RWMutex
}

func NewPageClusterManager(minSize, maxSize int) *PageClusterManager {
	return &PageClusterManager{
		clusters:        make(map[string]*models.PageCluster),
		minClusterSize:  minSize,
		maxClusterSize:  maxSize,
		sequentialBoost: 8,
	}
}

func clusterKey(processID string, anchorPage uint64) string {
	return processID + "\x00" + strconv.FormatUint(anchorPage, 10)
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
	pcm.clusters[clusterKey(processID, pages[0])] = cluster

	return cluster
}

func (pcm *PageClusterManager) GetPrefetchPages(processID string, anchorPage uint64) []uint64 {
	pcm.mu.RLock()
	defer pcm.mu.RUnlock()

	cluster, exists := pcm.clusters[clusterKey(processID, anchorPage)]
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
		pcm.clusters = make(map[string]*models.PageCluster)
		return
	}
	for key, cluster := range pcm.clusters {
		if cluster.ProcessID == processID {
			delete(pcm.clusters, key)
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
	// Guard against duplicate insertion: ShouldCompress + CompressPage is a
	// TOCTOU pair — another caller could have compressed the same page between
	// our ShouldCompress check and this lock acquisition.
	if _, exists := cm.compressedPages[pageID]; exists {
		cm.mu.Unlock()
		return nil
	}
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

// RestoreCompressed re-inserts a previously decompressed page back into the
// compression store. Used by handlePageFault when frame allocation fails after
// DecompressPage has already deleted the compressed entry, preventing silent
// data loss.
func (cm *CompressionManager) RestoreCompressed(cp *CompressedPage) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if _, exists := cm.compressedPages[cp.PageID]; exists {
		return // already restored or re-compressed concurrently
	}
	cm.compressedPages[cp.PageID] = cp
	cm.totalOriginal += cp.OriginalSize
	cm.totalCompressed += cp.CompressedSize
	// Do NOT increment pagesCompressed — this is not a new compression event;
	// it undoes a failed decompression. Decrementing pagesDecompressed keeps
	// "net successful decompressions" correct so callers can compute
	// currentCompressed = pagesCompressed - pagesDecompressed.
	cm.pagesDecompressed--
	if cm.totalOriginal > 0 {
		cm.compressionRatio = float64(cm.totalCompressed) / float64(cm.totalOriginal)
	}
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