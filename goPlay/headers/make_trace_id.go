package headers

import (
	"fmt"
	"math/rand"
	"time"
)

// MakeXTTTraceID 生成x-tt-trace-id
func MakeXTTTraceID(deviceID string) string {
	now := time.Now()
	timestamp := now.Unix()

	var traceID string
	if deviceID != "" {
		// 使用设备ID生成
		traceID = fmt.Sprintf("00-%016x%016x-0000000000000000-01", timestamp, rand.Int63())
	} else {
		// 随机生成
		traceID = fmt.Sprintf("00-%016x%016x-0000000000000000-01", rand.Int63(), rand.Int63())
	}

	return traceID
}
