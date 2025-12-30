package main

import (
	"net/http"
	"sync"
)

// BatchRequest 批量请求结构
type BatchRequest struct {
	DeviceMap map[string]string
	Proxy     string
	Client    *http.Client
}

// BatchResult 批量结果
type BatchResult struct {
	Seed     string
	SeedType int
	Token    string
	Err      error
}

// BatchProcessor 批量处理器
type BatchProcessor struct {
	batchSize int
	requests  chan BatchRequest
	results   chan BatchResult
	wg        sync.WaitGroup
}

// NewBatchProcessor 创建批量处理器
func NewBatchProcessor(batchSize int) *BatchProcessor {
	return &BatchProcessor{
		batchSize: batchSize,
		requests:  make(chan BatchRequest, batchSize*10),
		results:   make(chan BatchResult, batchSize*10),
	}
}

// ProcessBatch 批量处理请求
func (bp *BatchProcessor) ProcessBatch(requests []BatchRequest) []BatchResult {
	if len(requests) == 0 {
		return nil
	}

	// 并行处理所有请求
	results := make([]BatchResult, len(requests))
	var wg sync.WaitGroup

	for i, req := range requests {
		wg.Add(1)
		go func(idx int, r BatchRequest) {
			defer wg.Done()
			
			// 并行获取seed和token
			seedChan := GetSeedAsync(r.DeviceMap, r.Client)
			tokenChan := GetTokenAsync(r.DeviceMap, r.Client)
			
			// 等待结果
			seedResult := <-seedChan
			tokenResult := <-tokenChan
			
			results[idx] = BatchResult{
				Seed:     seedResult.Seed,
				SeedType: seedResult.SeedType,
				Token:    tokenResult.Token,
				Err:      seedResult.Err,
			}
		}(i, req)
	}

	wg.Wait()
	return results
}

