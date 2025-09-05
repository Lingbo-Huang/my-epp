package flow_control

import (
	"container/heap"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Lingbo-Huang/my-epp/pkg/types"
)

// QueueSystem 队列系统实现
type QueueSystem struct {
	// 队列配置
	config *QueueConfig

	// 不同策略的队列
	fifoQueue     []*types.ProcessingRequest
	priorityQueue *PriorityQueue

	// 队列锁
	queueLock sync.RWMutex

	// 统计信息
	stats *QueueStats
}

// QueueConfig 队列配置
type QueueConfig struct {
	MaxSize        int                 `json:"max_size"`
	MaxWaitTime    time.Duration       `json:"max_wait_time"`
	Strategy       types.QueueStrategy `json:"strategy"`
	PriorityLevels int                 `json:"priority_levels"`

	// 处理配置
	BatchSize       int           `json:"batch_size"`
	ProcessInterval time.Duration `json:"process_interval"`

	// 超时配置
	EnableTimeout  bool          `json:"enable_timeout"`
	TimeoutCleanup time.Duration `json:"timeout_cleanup"`
}

// QueueStats 队列统计
type QueueStats struct {
	TotalEnqueued int64 `json:"total_enqueued"`
	TotalDequeued int64 `json:"total_dequeued"`
	TotalTimedOut int64 `json:"total_timed_out"`
	TotalDropped  int64 `json:"total_dropped"`

	CurrentSize     int           `json:"current_size"`
	AverageWaitTime time.Duration `json:"average_wait_time"`
	MaxWaitTime     time.Duration `json:"max_wait_time"`

	LastEnqueueTime time.Time `json:"last_enqueue_time"`
	LastDequeueTime time.Time `json:"last_dequeue_time"`
}

// PriorityQueue 优先级队列实现
type PriorityQueue []*PriorityItem

// PriorityItem 优先级队列项
type PriorityItem struct {
	Request     *types.ProcessingRequest `json:"request"`
	Priority    int                      `json:"priority"`
	EnqueueTime time.Time                `json:"enqueue_time"`
	Index       int                      `json:"index"`
}

// NewQueueSystem 创建新的队列系统
func NewQueueSystem(config *QueueConfig) *QueueSystem {
	if config == nil {
		config = &QueueConfig{
			MaxSize:         1000,
			MaxWaitTime:     60 * time.Second,
			Strategy:        types.QueueStrategyFIFO,
			PriorityLevels:  10,
			BatchSize:       10,
			ProcessInterval: 100 * time.Millisecond,
			EnableTimeout:   true,
			TimeoutCleanup:  30 * time.Second,
		}
	}

	qs := &QueueSystem{
		config:        config,
		fifoQueue:     make([]*types.ProcessingRequest, 0),
		priorityQueue: &PriorityQueue{},
		stats:         &QueueStats{},
	}

	// 初始化优先级队列
	heap.Init(qs.priorityQueue)

	// 启动后台清理任务
	if config.EnableTimeout {
		go qs.cleanupTimeoutRequests()
	}

	log.Printf("队列系统初始化完成 - 策略: %s, 最大大小: %d",
		config.Strategy.String(), config.MaxSize)

	return qs
}

// Enqueue 将请求加入队列
func (qs *QueueSystem) Enqueue(req *types.ProcessingRequest) error {
	qs.queueLock.Lock()
	defer qs.queueLock.Unlock()

	// 检查队列是否已满
	currentSize := qs.getCurrentSize()
	if currentSize >= qs.config.MaxSize {
		qs.stats.TotalDropped++
		return fmt.Errorf("队列已满，当前大小: %d", currentSize)
	}

	// 设置入队时间
	req.QueuedTime = time.Now()

	// 根据策略入队
	switch qs.config.Strategy {
	case types.QueueStrategyFIFO, types.QueueStrategyLIFO:
		qs.fifoQueue = append(qs.fifoQueue, req)
	case types.QueueStrategyPriority:
		item := &PriorityItem{
			Request:     req,
			Priority:    req.Priority,
			EnqueueTime: req.QueuedTime,
		}
		heap.Push(qs.priorityQueue, item)
	case types.QueueStrategyShortest:
		// 简化实现：按预估处理时间排序
		qs.insertByShortest(req)
	default:
		qs.fifoQueue = append(qs.fifoQueue, req)
	}

	// 更新统计
	qs.stats.TotalEnqueued++
	qs.stats.CurrentSize = qs.getCurrentSize()
	qs.stats.LastEnqueueTime = time.Now()

	log.Printf("请求入队成功 - ID: %s, 队列大小: %d, 策略: %s",
		req.ID, qs.stats.CurrentSize, qs.config.Strategy.String())

	return nil
}

// Dequeue 从队列中取出请求
func (qs *QueueSystem) Dequeue() (*types.ProcessingRequest, error) {
	qs.queueLock.Lock()
	defer qs.queueLock.Unlock()

	// 检查队列是否为空
	if qs.getCurrentSize() == 0 {
		return nil, fmt.Errorf("队列为空")
	}

	var req *types.ProcessingRequest

	// 根据策略出队
	switch qs.config.Strategy {
	case types.QueueStrategyFIFO:
		req = qs.fifoQueue[0]
		qs.fifoQueue = qs.fifoQueue[1:]

	case types.QueueStrategyLIFO:
		lastIndex := len(qs.fifoQueue) - 1
		req = qs.fifoQueue[lastIndex]
		qs.fifoQueue = qs.fifoQueue[:lastIndex]

	case types.QueueStrategyPriority:
		if qs.priorityQueue.Len() == 0 {
			return nil, fmt.Errorf("优先级队列为空")
		}
		item := heap.Pop(qs.priorityQueue).(*PriorityItem)
		req = item.Request

	case types.QueueStrategyShortest:
		// 简化实现：取第一个（已按处理时间排序）
		req = qs.fifoQueue[0]
		qs.fifoQueue = qs.fifoQueue[1:]

	default:
		req = qs.fifoQueue[0]
		qs.fifoQueue = qs.fifoQueue[1:]
	}

	// 更新统计
	qs.stats.TotalDequeued++
	qs.stats.CurrentSize = qs.getCurrentSize()
	qs.stats.LastDequeueTime = time.Now()

	// 计算等待时间
	waitTime := time.Since(req.QueuedTime)
	qs.updateWaitTimeStats(waitTime)

	log.Printf("请求出队成功 - ID: %s, 等待时间: %v, 队列剩余: %d",
		req.ID, waitTime, qs.stats.CurrentSize)

	return req, nil
}

// Size 获取队列大小
func (qs *QueueSystem) Size() int {
	qs.queueLock.RLock()
	defer qs.queueLock.RUnlock()

	return qs.getCurrentSize()
}

// EstimatedWaitTime 估算等待时间
func (qs *QueueSystem) EstimatedWaitTime() time.Duration {
	qs.queueLock.RLock()
	defer qs.queueLock.RUnlock()

	currentSize := qs.getCurrentSize()
	if currentSize == 0 {
		return 0
	}

	// 基于平均处理时间和队列长度估算
	avgProcessTime := qs.stats.AverageWaitTime
	if avgProcessTime == 0 {
		avgProcessTime = 1 * time.Second // 默认1秒
	}

	// 考虑队列策略的影响
	estimatedTime := time.Duration(currentSize) * avgProcessTime

	// 优先级队列的高优先级请求等待时间更短
	if qs.config.Strategy == types.QueueStrategyPriority {
		estimatedTime = estimatedTime / 2 // 简化计算
	}

	return estimatedTime
}

// getCurrentSize 获取当前队列大小（内部使用，无锁）
func (qs *QueueSystem) getCurrentSize() int {
	switch qs.config.Strategy {
	case types.QueueStrategyPriority:
		return qs.priorityQueue.Len()
	default:
		return len(qs.fifoQueue)
	}
}

// insertByShortest 按最短处理时间插入
func (qs *QueueSystem) insertByShortest(req *types.ProcessingRequest) {
	// 简化实现：根据模型复杂度估算处理时间
	estimatedTime := qs.estimateProcessingTime(req)
	req.Metadata["estimated_time"] = estimatedTime

	// 插入到合适位置
	inserted := false
	for i, existingReq := range qs.fifoQueue {
		if existingTime, exists := existingReq.Metadata["estimated_time"]; exists {
			if estimatedTime < existingTime.(time.Duration) {
				// 插入到这个位置
				qs.fifoQueue = append(qs.fifoQueue[:i], append([]*types.ProcessingRequest{req}, qs.fifoQueue[i:]...)...)
				inserted = true
				break
			}
		}
	}

	if !inserted {
		qs.fifoQueue = append(qs.fifoQueue, req)
	}
}

// estimateProcessingTime 估算处理时间
func (qs *QueueSystem) estimateProcessingTime(req *types.ProcessingRequest) time.Duration {
	// 根据模型类型和LoRA需求估算
	baseTime := 500 * time.Millisecond

	if req.LoRARequired {
		baseTime *= 2 // LoRA请求需要更多时间
	}

	// 根据优先级调整
	if req.Priority > 5 {
		baseTime = baseTime * 80 / 100 // 高优先级请求预期更快处理
	}

	return baseTime
}

// updateWaitTimeStats 更新等待时间统计
func (qs *QueueSystem) updateWaitTimeStats(waitTime time.Duration) {
	// 简化的移动平均计算
	if qs.stats.AverageWaitTime == 0 {
		qs.stats.AverageWaitTime = waitTime
	} else {
		qs.stats.AverageWaitTime = (qs.stats.AverageWaitTime*9 + waitTime) / 10
	}

	// 更新最大等待时间
	if waitTime > qs.stats.MaxWaitTime {
		qs.stats.MaxWaitTime = waitTime
	}
}

// cleanupTimeoutRequests 清理超时请求
func (qs *QueueSystem) cleanupTimeoutRequests() {
	ticker := time.NewTicker(qs.config.TimeoutCleanup)
	defer ticker.Stop()

	for range ticker.C {
		qs.queueLock.Lock()

		now := time.Now()
		var cleanedCount int

		// 清理FIFO队列中的超时请求
		var validRequests []*types.ProcessingRequest
		for _, req := range qs.fifoQueue {
			if now.Sub(req.QueuedTime) > qs.config.MaxWaitTime {
				cleanedCount++
				qs.stats.TotalTimedOut++
			} else {
				validRequests = append(validRequests, req)
			}
		}
		qs.fifoQueue = validRequests

		// 清理优先级队列中的超时请求
		if qs.config.Strategy == types.QueueStrategyPriority {
			var validItems []*PriorityItem
			for qs.priorityQueue.Len() > 0 {
				item := heap.Pop(qs.priorityQueue).(*PriorityItem)
				if now.Sub(item.EnqueueTime) > qs.config.MaxWaitTime {
					cleanedCount++
					qs.stats.TotalTimedOut++
				} else {
					validItems = append(validItems, item)
				}
			}

			// 重新加入有效项
			for _, item := range validItems {
				heap.Push(qs.priorityQueue, item)
			}
		}

		qs.stats.CurrentSize = qs.getCurrentSize()
		qs.queueLock.Unlock()

		if cleanedCount > 0 {
			log.Printf("清理超时请求: %d 个, 队列剩余: %d", cleanedCount, qs.stats.CurrentSize)
		}
	}
}

// GetStats 获取队列统计信息
func (qs *QueueSystem) GetStats() *QueueStats {
	qs.queueLock.RLock()
	defer qs.queueLock.RUnlock()

	// 返回统计副本
	statsCopy := *qs.stats
	statsCopy.CurrentSize = qs.getCurrentSize()

	return &statsCopy
}

// Clear 清空队列
func (qs *QueueSystem) Clear() {
	qs.queueLock.Lock()
	defer qs.queueLock.Unlock()

	qs.fifoQueue = qs.fifoQueue[:0]
	*qs.priorityQueue = (*qs.priorityQueue)[:0]
	heap.Init(qs.priorityQueue)

	qs.stats.CurrentSize = 0

	log.Println("队列已清空")
}

// 优先级队列的heap.Interface实现
func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	// 优先级高的排在前面（数值越大优先级越高）
	if pq[i].Priority != pq[j].Priority {
		return pq[i].Priority > pq[j].Priority
	}
	// 优先级相同时，先入队的排在前面
	return pq[i].EnqueueTime.Before(pq[j].EnqueueTime)
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].Index = i
	pq[j].Index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*PriorityItem)
	item.Index = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.Index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}
