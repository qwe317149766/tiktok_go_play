package main

import (
	"sync"
	"sync/atomic"
	"time"
)

// BatchProcessor 批处理器 - 用于批量处理任务
type BatchProcessor struct {
	batchSize     int           // 批处理大小
	flushInterval time.Duration // 刷新间隔
	mu            sync.Mutex
	pending       []interface{}
	lastFlush     time.Time
}

// NewBatchProcessor 创建新的批处理器
func NewBatchProcessor(batchSize int, flushInterval time.Duration) *BatchProcessor {
	return &BatchProcessor{
		batchSize:     batchSize,
		flushInterval: flushInterval,
		pending:       make([]interface{}, 0, batchSize),
		lastFlush:     time.Now(),
	}
}

// Add 添加项目到批处理队列
func (bp *BatchProcessor) Add(item interface{}) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	bp.pending = append(bp.pending, item)

	// 如果达到批处理大小，立即刷新
	if len(bp.pending) >= bp.batchSize {
		bp.flush()
	} else {
		// 检查是否需要定时刷新
		if time.Since(bp.lastFlush) >= bp.flushInterval {
			bp.flush()
		}
	}
}

// flush 刷新批处理队列（需要在持有锁的情况下调用）
func (bp *BatchProcessor) flush() {
	if len(bp.pending) == 0 {
		return
	}

	// 这里可以添加实际的批处理逻辑
	// 例如：批量写入数据库、批量发送请求等
	// 目前只是清空队列
	bp.pending = bp.pending[:0]
	bp.lastFlush = time.Now()
}

// Flush 手动刷新批处理队列
func (bp *BatchProcessor) Flush() {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.flush()
}

// GetPendingCount 获取待处理数量
func (bp *BatchProcessor) GetPendingCount() int {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	return len(bp.pending)
}

// BatchStats 批处理统计
type BatchStats struct {
	TotalBatches  int64 // 总批次数
	TotalItems    int64 // 总项目数
	LastBatchTime time.Time
	mu            sync.RWMutex
}

var globalBatchStats = &BatchStats{}

// RecordBatch 记录批处理
func RecordBatch(itemCount int) {
	globalBatchStats.mu.Lock()
	defer globalBatchStats.mu.Unlock()
	atomic.AddInt64(&globalBatchStats.TotalBatches, 1)
	atomic.AddInt64(&globalBatchStats.TotalItems, int64(itemCount))
	globalBatchStats.LastBatchTime = time.Now()
}

// GetBatchStats 获取批处理统计
func GetBatchStats() (batches int64, items int64, lastTime time.Time) {
	globalBatchStats.mu.RLock()
	defer globalBatchStats.mu.RUnlock()
	return atomic.LoadInt64(&globalBatchStats.TotalBatches),
		atomic.LoadInt64(&globalBatchStats.TotalItems),
		globalBatchStats.LastBatchTime
}
