package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// 目标：signup 注册过程中“边成功边写入账号池”，避免 stats 长时间等待。
//
// 账号池：REDIS_STARTUP_COOKIE_POOL_KEY（沿用 startup_cookie_pool 的 key 结构）
// - {prefix}:ids (SET)
// - {prefix}:data (HASH) id -> json(account)  (account 包含 cookies 字段)

func startupAccountStreamEnabled() bool {
	// 只有启用写入账号池时才需要 stream
	if !getEnvBool("SAVE_STARTUP_COOKIES_TO_REDIS", false) {
		return false
	}
	// 默认开启：你现在的场景就是“2000 很久才结束，stats 一直等”
	return getEnvBool("DGEMAIL_STREAM_WRITE_STARTUP_POOL", true)
}

type startupAccountStream struct {
	ch       chan map[string]interface{}
	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func (s *startupAccountStream) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.wg.Wait()
	})
}

func (s *startupAccountStream) Enqueue(acc map[string]interface{}) {
	if acc == nil {
		return
	}
	select {
	case s.ch <- acc:
	default:
		// 队列满：宁可丢弃也不阻塞注册流程
		log.Printf("[stream] enqueue dropped: queue full")
	}
}

var (
	startupStream     *startupAccountStream
	startupStreamOnce sync.Once
)

func getStartupAccountStream() *startupAccountStream {
	startupStreamOnce.Do(func() {
		if !startupAccountStreamEnabled() {
			startupStream = nil
			return
		}
		buf := getEnvInt("DGEMAIL_STREAM_QUEUE_SIZE", 2000)
		if buf <= 0 {
			buf = 2000
		}
		startupStream = &startupAccountStream{
			ch:     make(chan map[string]interface{}, buf),
			stopCh: make(chan struct{}),
		}
		startupStream.wg.Add(1)
		go func() {
			defer startupStream.wg.Done()
			startupAccountStreamLoop(startupStream.ch, startupStream.stopCh)
		}()
		log.Printf("[stream] startup account stream enabled (queue=%d)", buf)
	})
	return startupStream
}

func startupAccountStreamLoop(ch <-chan map[string]interface{}, stop <-chan struct{}) {
	rdb, err := newRedisClient()
	if err != nil {
		log.Printf("[stream] redis init failed: %v (stream disabled)", err)
		return
	}
	defer rdb.Close()

	flushEvery := getEnvInt("DGEMAIL_STREAM_FLUSH_SEC", 2)
	if flushEvery <= 0 {
		flushEvery = 2
	}
	batchMax := getEnvInt("DGEMAIL_STREAM_BATCH", 80)
	if batchMax <= 0 {
		batchMax = 80
	}

	ticker := time.NewTicker(time.Duration(flushEvery) * time.Second)
	defer ticker.Stop()

	var pending []map[string]interface{}
	flush := func() {
		if len(pending) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), redisLoadTimeout())
		defer cancel()
		n, e := writeStartupAccountsToRedisWithClient(ctx, rdb, pending)
		if e != nil {
			log.Printf("[stream] flush failed: wrote=%d err=%v", n, e)
		} else {
			log.Printf("[stream] flush ok: wrote=%d", n)
		}
		pending = pending[:0]
	}

	for {
		select {
		case <-stop:
			flush()
			return
		case <-ticker.C:
			flush()
		case acc := <-ch:
			if acc == nil {
				continue
			}
			pending = append(pending, acc)
			if len(pending) >= batchMax {
				flush()
			}
		}
	}
}

// writeStartupAccountsToRedisWithClient 将账号 JSON 批量写入账号池（按分片前缀分组后 pipeline）
func writeStartupAccountsToRedisWithClient(ctx context.Context, rdb *redis.Client, accounts []map[string]interface{}) (int, error) {
	if len(accounts) == 0 {
		return 0, nil
	}

	basePrefix := normalizePoolBase(getEnvStr("REDIS_STARTUP_COOKIE_POOL_KEY", "tiktok:startup_cookie_pool"))
	if basePrefix == "" {
		basePrefix = "tiktok:startup_cookie_pool"
	}
	shards := getCookiePoolShards()
	if shards <= 0 {
		shards = 1
	}
	// 如果当前 prefix 已经是明确的 :idx（例如 poll 模式选择 shard），则视为“锁定分库”，不做自动分片
	if _, _, ok := splitPoolShardSuffix(strings.TrimSpace(os.Getenv("REDIS_STARTUP_COOKIE_POOL_KEY"))); ok {
		_ = os.Setenv("DGEMAIL_POOL_SHARD_LOCKED", "1")
	}
	useAuto := autoShardEnabled()

	// 按 prefix 分组，避免不同 shard 混在同一个 pipeline
	group := map[string][]struct {
		id  string
		val string
	}{}

	for _, acc := range accounts {
		if acc == nil {
			continue
		}
		rawCookies, ok := acc["cookies"]
		if !ok || strings.TrimSpace(fmt.Sprintf("%v", rawCookies)) == "" {
			continue
		}
		id := strings.TrimSpace(deviceIDFromAny(acc))
		if id == "" {
			b, _ := json.Marshal(acc)
			return 0, fmt.Errorf("startup account missing device_id: %s", string(b))
		}
		valBytes, _ := json.Marshal(acc)
		val := string(valBytes)

		shardIdx := 0
		if useAuto {
			shardKey := strings.TrimSpace(deviceIDFromAny(acc))
			if shardKey == "" {
				shardKey = id
			}
			shardIdx = shardIndexByKey(shardKey, shards)
		}
		prefix := poolPrefixForShard(basePrefix, shardIdx)
		group[prefix] = append(group[prefix], struct {
			id  string
			val string
		}{id: id, val: val})
	}

	wrote := 0
	for prefix, items := range group {
		if len(items) == 0 {
			continue
		}
		idsKey := prefix + ":ids"
		dataKey := prefix + ":data"
		useKey := prefix + ":use"

		pipe := rdb.TxPipeline()
		for _, it := range items {
			pipe.SAdd(ctx, idsKey, it.id)
			pipe.HSet(ctx, dataKey, it.id, it.val)
			pipe.ZAddNX(ctx, useKey, redis.Z{Member: it.id, Score: 0})
		}
		if _, err := pipe.Exec(ctx); err != nil {
			return wrote, fmt.Errorf("redis write startup accounts (prefix=%s): %w", prefix, err)
		}
		wrote += len(items)
	}
	return wrote, nil
}

// 从注册 goroutine 里直接写会导致创建很多 redis client；这里提供 enqueue 入口。
func enqueueStartupAccountForRedis(acc map[string]interface{}) {
	s := getStartupAccountStream()
	if s == nil {
		return
	}
	s.Enqueue(acc)
}


