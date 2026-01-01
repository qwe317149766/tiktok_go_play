package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// 需求：
// - signup 项目：同一个设备成功注册次数 > 3 则从设备池删除
// - 连续失败次数 > 10 也从设备池删除
//
// 说明（全 DB 模式）：
// - 设备池：MySQL 表 device_pool_devices
// - 计数：用 device_pool_devices.use_count / fail_count

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

func signupDeviceUsageUpdate(deviceID string, success bool) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return
	}
	if !signupDeviceEvictEnabled() {
		return
	}
	// 全 DB 模式：用 device_pool_devices 的 use_count/fail_count 做阈值淘汰
	db, err := getSignupDB()
	if err != nil {
		return
	}
	tbl := devicePoolTable()
	maxSucc := int64(signupDeviceMaxSuccess())
	maxFail := int64(signupDeviceMaxConsecFail())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if success {
		_, _ = db.ExecContext(ctx, fmt.Sprintf("UPDATE `%s` SET use_count=use_count+1, fail_count=0 WHERE device_id=?", tbl), deviceID)
	} else {
		_, _ = db.ExecContext(ctx, fmt.Sprintf("UPDATE `%s` SET fail_count=fail_count+1 WHERE device_id=?", tbl), deviceID)
	}

	var useCnt, failCnt int64
	_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT use_count, fail_count FROM `%s` WHERE device_id=?", tbl), deviceID).Scan(&useCnt, &failCnt)
	reason := ""
	if maxSucc > 0 && useCnt > maxSucc {
		reason = fmt.Sprintf("success>%d", maxSucc)
	}
	if maxFail > 0 && failCnt > maxFail {
		reason = fmt.Sprintf("consec_fail>%d", maxFail)
	}
	if reason != "" {
		_ = deleteDeviceFromDBPool(deviceID)
		log.Printf("[device] evicted from DB table=%s device_id=%s reason=%s", tbl, deviceID, reason)
	}
}
