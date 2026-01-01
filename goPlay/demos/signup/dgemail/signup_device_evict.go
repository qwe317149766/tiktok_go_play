package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// 需求变更：
// - signup 项目：设备注册成功一次后，立即使用独立线程异步删除。
// - 失败处理：保持原有逻辑（连续失败次数 > 10 则删除）。

var (
	// 定义删除通道，容量足够大以避免阻塞
	deviceEvictChan = make(chan string, 5000)
)

func init() {
	// 启动后台线程处理设备删除
	go func() {
		for deviceID := range deviceEvictChan {
			if deviceID == "" {
				continue
			}
			// 调用 db_pool.go 中的删除函数
			if err := deleteDeviceFromDBPool(deviceID); err != nil {
				log.Printf("[evict-error] 异步删除设备失败 device_id=%s: %v", deviceID, err)
			} else {
				log.Printf("[evict] 异步删除设备成功(注册成功后淘汰) device_id=%s", deviceID)
			}
		}
	}()
}

func signupDeviceEvictEnabled() bool {
	return getEnvBool("SIGNUP_DEVICE_EVICT_ENABLED", true)
}

func signupDeviceMaxConsecFail() int {
	n := getEnvInt("SIGNUP_DEVICE_MAX_CONSEC_FAIL", 10)
	if n < 0 {
		return 0
	}
	return n
}

func signupDeviceUsageUpdate(deviceID string, success bool) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return
	}
	if !signupDeviceEvictEnabled() {
		return
	}

	// 策略1：注册成功 -> 立即异步删除
	if success {
		select {
		case deviceEvictChan <- deviceID:
			// ok
		default:
			log.Printf("[evict-warn] 删除队列已满，强制丢弃(可能不一致) device_id=%s", deviceID)
		}
		return
	}

	// 策略2：注册失败 -> 记录 fail_count，若超过阈值则删除（保留同步逻辑或改为异步均可，这里暂保留同步以复用原有 DB 逻辑）
	db, err := getSignupDB()
	if err != nil {
		return
	}
	tbl := devicePoolTable()
	maxFail := int64(signupDeviceMaxConsecFail())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 失败：fail_count + 1
	_, _ = db.ExecContext(ctx, fmt.Sprintf("UPDATE `%s` SET fail_count=fail_count+1 WHERE device_id=?", tbl), deviceID)

	var failCnt int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT fail_count FROM `%s` WHERE device_id=?", tbl), deviceID).Scan(&failCnt)

	if maxFail > 0 && failCnt > maxFail {
		// 失败过多，也投递到删除队列（统一异步删除）
		select {
		case deviceEvictChan <- deviceID:
			log.Printf("[device] 连续失败>%d，触发异步删除 device_id=%s", maxFail, deviceID)
		default:
		}
	}
}
