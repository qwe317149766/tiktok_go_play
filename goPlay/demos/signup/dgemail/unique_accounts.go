package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

func dgemailEmailDomain() string {
	d := strings.TrimSpace(getEnvStr("DGEMAIL_EMAIL_DOMAIN", "gmail.com"))
	if d == "" {
		return "gmail.com"
	}
	return d
}

func dgemailEmailPrefix() string {
	p := strings.TrimSpace(getEnvStr("DGEMAIL_EMAIL_PREFIX", "wazss"))
	if p == "" {
		return "wazss"
	}
	return p
}

func dgemailEmailSeqKey() string {
	// DB counters.name
	k := strings.TrimSpace(getEnvStr("DGEMAIL_EMAIL_SEQ_KEY", "dgemail_email_seq"))
	if k == "" {
		return "dgemail_email_seq"
	}
	return k
}

func dgemailUniqueAccountsEnabled() bool {
	// 你要求“账号不能重复”：默认开启
	return getEnvBool("DGEMAIL_UNIQUE_ACCOUNTS", true)
}

func base36(n int64) string {
	if n <= 0 {
		return "0"
	}
	const chars = "0123456789abcdefghijklmnopqrstuvwxyz"
	var b [32]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = chars[n%36]
		n = n / 36
	}
	return string(b[i:])
}

func reserveEmailSeq(count int) (start, end int64, ok bool) {
	if count <= 0 {
		return 0, 0, false
	}
	db, err := getSignupDB()
	if err != nil {
		return 0, 0, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 原子申请区间：
	// INSERT ON DUPLICATE + LAST_INSERT_ID(val+count)
	// 再 SELECT LAST_INSERT_ID() 得到 end
	_, err = db.ExecContext(ctx,
		"INSERT INTO counters(name,val) VALUES(?,0) ON DUPLICATE KEY UPDATE val=LAST_INSERT_ID(val+?)",
		dgemailEmailSeqKey(), int64(count),
	)
	if err != nil {
		return 0, 0, false
	}
	var end64 int64
	if err := db.QueryRowContext(ctx, "SELECT LAST_INSERT_ID()").Scan(&end64); err != nil {
		return 0, 0, false
	}
	start64 := end64 - int64(count) + 1
	if start64 <= 0 {
		start64 = 1
	}
	return start64, end64, true
}

func buildEmailFromSeq(seq int64) string {
	// 示例：wazss_k3j2p@gmail.com
	return fmt.Sprintf("%s_%s@%s", dgemailEmailPrefix(), base36(seq), dgemailEmailDomain())
}

var localEmailSeq int64

func generateUniqueAccounts(count int) []AccountInfo {
	if count <= 0 {
		return []AccountInfo{}
	}
	if dgemailUniqueAccountsEnabled() {
		if start, end, ok := reserveEmailSeq(count); ok {
			out := make([]AccountInfo, 0, count)
			for s := start; s <= end && len(out) < count; s++ {
				out = append(out, AccountInfo{
					Email:    buildEmailFromSeq(s),
					Password: "qw123456789!",
				})
			}
			return out
		}
		log.Printf("[acct] ⚠️ 无法从 DB counters 申请唯一邮箱序号，将回退为随机生成（可能跨进程重复）；可检查 DB 配置/连接或设置 DGEMAIL_UNIQUE_ACCOUNTS=0")
	}

	// fallback：保证“本批次”不重复（跨进程不保证）
	rand.Seed(time.Now().UnixNano())
	seen := map[string]bool{}
	out := make([]AccountInfo, 0, count)
	for len(out) < count {
		seq := atomic.AddInt64(&localEmailSeq, 1)
		username := fmt.Sprintf("%s_%d_%d", dgemailEmailPrefix(), time.Now().UnixNano(), seq)
		// 再混一点随机，降低同纳秒碰撞概率
		username = username + "_" + strconv.FormatInt(int64(rand.Intn(1_000_000)), 10)
		email := fmt.Sprintf("%s@%s", username, dgemailEmailDomain())
		if seen[email] {
			continue
		}
		seen[email] = true
		out = append(out, AccountInfo{Email: email, Password: "qw123456789!"})
	}
	return out
}


