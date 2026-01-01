package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type Order struct {
	ID        int64
	OrderID   string
	APIKey    string
	AwemeID   string
	Quantity  int64
	Delivered int64
	Status    string
}

func shouldRunOrderMode() bool {
	// 默认：Linux 环境开启“抢单模式”；其它系统不开启
	def := runtime.GOOS == "linux"
	if envBool("STATS_ORDER_MODE", def) {
		return true
	}
	return false
}

func getDB() (*sql.DB, error) {
	host := envStr("DB_HOST", "127.0.0.1")
	port := envStr("DB_PORT", "3306")
	user := envStr("DB_USER", "root")
	pass := envStr("DB_PASSWORD", "123456")
	name := envStr("DB_NAME", "tiktok_go_play")

	// parseTime=true 方便扫描时间字段（虽然我们这里主要用不到）
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci",
		user, pass, host, port, name)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func claimOneOrder(ctx context.Context, db *sql.DB) (*Order, error) {
	// 乐观抢单（不使用 FOR UPDATE）：
	// - 先 SELECT 候选订单
	// - 再用带条件的 UPDATE 去“抢单”，通过 RowsAffected 判断是否抢到
	// - 如果 RowsAffected=0，说明被其它进程抢走/状态变化，继续轮询即可
	//
	// 抢单互斥策略：
	// - 默认只抢 Pending/Partial（空闲订单）
	// - In progress 只有在“超时未更新”时（疑似进程崩溃）才允许被重新抢
	staleSec := envInt("STATS_ORDER_STALE_SEC", 120)
	pollMS := envInt("STATS_ORDER_POLL_MS", 500)
	if pollMS < 50 {
		pollMS = 50
	}
	poll := time.Duration(pollMS) * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			return nil, nil
		default:
		}

		var o Order
		err := db.QueryRowContext(ctx, `
SELECT id, order_id, api_key, aweme_id, quantity, delivered, status
FROM orders
WHERE (
    status IN ('Pending','Partial')
 OR (status = 'In progress' AND updated_at < (NOW() - INTERVAL ? SECOND))
) AND delivered < quantity
ORDER BY id ASC
LIMIT 1`, staleSec).Scan(&o.ID, &o.OrderID, &o.APIKey, &o.AwemeID, &o.Quantity, &o.Delivered, &o.Status)
		if err == sql.ErrNoRows {
			// 没有可抢订单：按间隔轮询
			select {
			case <-time.After(poll):
				continue
			case <-ctx.Done():
				return nil, nil
			}
		}
		if err != nil {
			return nil, err
		}

		// 尝试抢单：只有满足条件时才更新成功（RowsAffected=1）
		res, err := db.ExecContext(ctx, `
UPDATE orders
SET status='In progress',
    updated_at=NOW()
WHERE id=?
  AND delivered < quantity
  AND (
        status IN ('Pending','Partial')
     OR (status='In progress' AND updated_at < (NOW() - INTERVAL ? SECOND))
  )`, o.ID, staleSec)
		if err != nil {
			return nil, err
		}
		ra, _ := res.RowsAffected()
		if ra == 0 {
			// 被别人抢走/状态变化：继续轮询
			select {
			case <-time.After(poll):
				continue
			case <-ctx.Done():
				return nil, nil
			}
		}
		// 抢到：返回订单（状态在 DB 已变更）
		o.Status = "In progress"
		return &o, nil
	}
}

func updateOrderDelivered(ctx context.Context, db *sql.DB, orderID string, delta int64) error {
	if strings.TrimSpace(orderID) == "" || delta <= 0 {
		return nil
	}
	// delivered 不超过 quantity；达到后置 Completed，否则 In progress
	_, err := db.ExecContext(ctx, `
UPDATE orders
SET delivered = LEAST(quantity, delivered + ?),
    status = CASE WHEN LEAST(quantity, delivered + ?) >= quantity THEN 'Completed' ELSE 'In progress' END,
    updated_at=NOW()
WHERE order_id = ?`, delta, delta, orderID)
	return err
}

func setOrderDeliveredAtLeast(ctx context.Context, db *sql.DB, orderID string, delivered int64) error {
	if strings.TrimSpace(orderID) == "" || delivered <= 0 {
		return nil
	}
	// delivered 不超过 quantity；并且只向上更新（避免回退）
	_, err := db.ExecContext(ctx, `
UPDATE orders
SET delivered = GREATEST(delivered, LEAST(quantity, ?)),
    status = CASE WHEN GREATEST(delivered, LEAST(quantity, ?)) >= quantity THEN 'Completed'
                  WHEN GREATEST(delivered, LEAST(quantity, ?)) > 0 THEN 'In progress'
                  ELSE status END,
    updated_at=NOW()
WHERE order_id = ?`, delivered, delivered, delivered, orderID)
	return err
}

func finalizeOrderStatus(ctx context.Context, db *sql.DB, orderID string) error {
	var delivered, quantity int64
	var status string
	if err := db.QueryRowContext(ctx, `SELECT delivered, quantity, status FROM orders WHERE order_id=?`, orderID).Scan(&delivered, &quantity, &status); err != nil {
		return err
	}
	newStatus := "In progress"
	if delivered >= quantity && quantity > 0 {
		newStatus = "Completed"
	} else if delivered > 0 && delivered < quantity {
		newStatus = "Partial"
	} else if delivered == 0 {
		// 本轮没有任何成功：释放订单，让其它进程继续抢
		newStatus = "Pending"
	}
	_, err := db.ExecContext(ctx, `UPDATE orders SET status=?, updated_at=NOW() WHERE order_id=?`, newStatus, orderID)
	return err
}

func runOrderMode() {
	db, err := getDB()
	if err != nil {
		log.Fatalf("[order] db connect failed: %v", err)
	}
	defer db.Close()

	// 信号：尽量在退出时 flush DB
	stop := make(chan os.Signal, 2)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// 让 claimOneOrder 也能随 stop 立刻退出（否则会一直轮询）
	mainCtx, cancelMain := context.WithCancel(context.Background())
	defer cancelMain()
	go func() {
		<-stop
		cancelMain()
	}()

	for {
		order, err := claimOneOrder(mainCtx, db)
		if err != nil {
			log.Printf("[order] claim failed: %v", err)
			select {
			case <-time.After(2 * time.Second):
			case <-mainCtx.Done():
				log.Printf("[order] received stop signal, exit")
				return
			}
			continue
		}
		if order == nil {
			// claimOneOrder 内部已经轮询等待；这里通常不会拿到 nil
			select {
			case <-time.After(100 * time.Millisecond):
			case <-mainCtx.Done():
				log.Printf("[order] received stop signal, exit")
				return
			}
			continue
		}

		remaining := order.Quantity - order.Delivered
		remaining = order.Quantity - order.Delivered
		if remaining <= 0 {
			ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
			_ = finalizeOrderStatus(ctx2, db, order.OrderID)
			cancel2()
			continue
		}

		log.Printf("[order] claimed order_id=%s aweme_id=%s remaining=%d", order.OrderID, order.AwemeID, remaining)

		// 设置本轮任务目标
		config.AwemeID = order.AwemeID
		config.TargetSuccess = remaining
		// 允许失败：最多请求数放宽为 4 倍
		config.MaxRequests = remaining * 4

		// 记录本轮新增完成量，定期 flush
		var delta int64 = 0

		engine, err := NewEngine()
		if err != nil {
			log.Printf("[order] engine init failed: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		// 订单进度：全 DB 模式只累加本地 delta，定期 flush 到 MySQL
		engine.onPlaySuccess = func() {
			atomic.AddInt64(&delta, 1)
		}

		// DB 定期 flush（减少意外退出损失）
		done := make(chan struct{})
		go func() {
			ticker := time.NewTicker(time.Duration(envInt("STATS_ORDER_DB_FLUSH_SEC", 5)) * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					d := atomic.SwapInt64(&delta, 0)
					if d <= 0 {
						continue
					}
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					_ = updateOrderDelivered(ctx, db, order.OrderID, d)
					cancel()
				case <-done:
					return
				case <-stop:
					// 尽量 flush
					d := atomic.SwapInt64(&delta, 0)
					if d > 0 {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						_ = updateOrderDelivered(ctx, db, order.OrderID, d)
						cancel()
					}
					return
				}
			}
		}()

		engine.Run()
		close(done)

		// 最终 flush + 修正 status
		d := atomic.SwapInt64(&delta, 0)
		if d > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			_ = updateOrderDelivered(ctx, db, order.OrderID, d)
			cancel()
		}
		ctx2, cancel2 := context.WithTimeout(context.Background(), 8*time.Second)
		_ = finalizeOrderStatus(ctx2, db, order.OrderID)
		cancel2()
	}
}


