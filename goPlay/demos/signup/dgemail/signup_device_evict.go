package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// 需求：
// - signup 项目：同一个设备成功注册次数 > 3 则从设备池删除
// - 连续失败次数 > 10 也从设备池删除
//
// 说明：
// - 设备池：REDIS_DEVICE_POOL_KEY（当前进程选中的 shard）
// - 计数存储在同前缀下的 HASH：:signup_succ / :signup_fail_streak

func signupDeviceEvictEnabled() bool {
	return getEnvBool("SIGNUP_DEVICE_EVICT_ENABLED", true)
}

func signupDeviceMaxSuccess() int {
	n := getEnvInt("SIGNUP_DEVICE_MAX_SUCCESS", 3)
	if n < 0 {
		return 0
	}
	return n
}

func signupDeviceMaxConsecFail() int {
	n := getEnvInt("SIGNUP_DEVICE_MAX_CONSEC_FAIL", 10)
	if n < 0 {
		return 0
	}
	return n
}

var (
	signupRedisOnce sync.Once
	signupRedisCli  *redis.Client
	signupRedisErr  error
)

func getSignupRedisClient() (*redis.Client, error) {
	signupRedisOnce.Do(func() {
		rdb, err := newRedisClient()
		if err != nil {
			signupRedisErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := rdb.Ping(ctx).Err(); err != nil {
			_ = rdb.Close()
			signupRedisErr = err
			return
		}
		signupRedisCli = rdb
	})
	return signupRedisCli, signupRedisErr
}

func devicePoolPrefix() string {
	// dgemail 读取设备池：按当前进程 env 的 REDIS_DEVICE_POOL_KEY（可能含 :idx）
	p := strings.TrimSpace(getEnvStr("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool"))
	if p == "" {
		p = "tiktok:device_pool"
	}
	return p
}

func signupDeviceUsageUpdate(deviceID string, success bool) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return
	}
	if !shouldLoadDevicesFromRedis() {
		// 只有 Redis 设备池模式才有“从池子删除”的意义
		return
	}
	if !signupDeviceEvictEnabled() {
		return
	}

	rdb, err := getSignupRedisClient()
	if err != nil {
		return
	}

	prefix := devicePoolPrefix()
	succKey := prefix + ":signup_succ"
	streakKey := prefix + ":signup_fail_streak"

	maxSucc := int64(signupDeviceMaxSuccess())
	maxFail := int64(signupDeviceMaxConsecFail())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if success {
		pipe := rdb.TxPipeline()
		succCmd := pipe.HIncrBy(ctx, succKey, deviceID, 1)
		pipe.HSet(ctx, streakKey, deviceID, 0)
		if _, err := pipe.Exec(ctx); err != nil {
			return
		}
		if maxSucc > 0 && succCmd.Val() > maxSucc {
			_ = evictDeviceFromPool(ctx, rdb, prefix, deviceID, fmt.Sprintf("success>%d", maxSucc))
		}
		return
	}

	pipe := rdb.TxPipeline()
	failCmd := pipe.HIncrBy(ctx, streakKey, deviceID, 1)
	if _, err := pipe.Exec(ctx); err != nil {
		return
	}
	if maxFail > 0 && failCmd.Val() > maxFail {
		_ = evictDeviceFromPool(ctx, rdb, prefix, deviceID, fmt.Sprintf("consec_fail>%d", maxFail))
	}
}

func evictDeviceFromPool(ctx context.Context, rdb *redis.Client, prefix, deviceID, reason string) error {
	prefix = strings.TrimSpace(prefix)
	deviceID = strings.TrimSpace(deviceID)
	if prefix == "" || deviceID == "" {
		return nil
	}

	idsKey := prefix + ":ids"
	dataKey := prefix + ":data"
	useKey := prefix + ":use"
	failKey := prefix + ":fail"
	playKey := prefix + ":play"
	attemptKey := prefix + ":attempt"
	inKey := prefix + ":in"
	succKey := prefix + ":signup_succ"
	streakKey := prefix + ":signup_fail_streak"

	pipe := rdb.TxPipeline()
	pipe.SRem(ctx, idsKey, deviceID)
	pipe.HDel(ctx, dataKey, deviceID)
	pipe.ZRem(ctx, useKey, deviceID)
	pipe.ZRem(ctx, failKey, deviceID)
	pipe.ZRem(ctx, playKey, deviceID)
	pipe.ZRem(ctx, attemptKey, deviceID)
	pipe.ZRem(ctx, inKey, deviceID)
	pipe.HDel(ctx, succKey, deviceID)
	pipe.HDel(ctx, streakKey, deviceID)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	log.Printf("[device] evicted from pool prefix=%s device_id=%s reason=%s", prefix, deviceID, reason)
	return nil
}


