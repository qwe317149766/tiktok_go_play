package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"tt_code/headers"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

// ---------- Types & Config ----------

type EngineConfig struct {
	MaxConcurrency int
	TargetSuccess  int64
	MaxRequests    int64
	AwemeID        string
}

type Order struct {
	ID        int64
	OrderID   string
	AwemeID   string
	Quantity  int64
	Delivered int64
}

type Engine struct {
	cfg            *EngineConfig
	currentSuccess int64
	failed         int64
	total          int64
	ctx            context.Context
	cancel         context.CancelFunc
	proxies        []string
	onPlaySuccess  func()
}

// ---------- Resource Pools & Shared Clients ----------

var (
	// clientMap reuses http.Client per proxy for connection pooling
	clientMap sync.Map
	// gzipPool reuses gzip.Writer to minimize GC pressure
	gzipPool = sync.Pool{
		New: func() interface{} {
			return gzip.NewWriter(nil)
		},
	}
)

func getClient(proxy string) *http.Client {
	if client, ok := clientMap.Load(proxy); ok {
		return client.(*http.Client)
	}

	transport := &http.Transport{
		MaxIdleConns:        1000,
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConnsPerHost: 100,
		DisableCompression:  true, // Manual gzip handling
	}

	if proxy != "" {
		if pUrl, err := url.Parse(proxy); err == nil {
			transport.Proxy = http.ProxyURL(pUrl)
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
	}

	actual, _ := clientMap.LoadOrStore(proxy, client)
	return actual.(*http.Client)
}

// ---------- Helper Functions ----------

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.ToLower(os.Getenv(key))
	return v == "1" || v == "true" || v == "on" || v == "yes"
}

func digitsLocal(n int, r *rand.Rand) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteByte(byte('0' + r.Intn(10)))
	}
	return sb.String()
}

func hexStringLocal(n int, r *rand.Rand) string {
	const charset = "0123456789abcdef"
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteByte(charset[r.Intn(len(charset))])
	}
	return sb.String()
}

func uuid4Local(r *rand.Rand) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		r.Uint32(),
		r.Uint32()&0xffff,
		r.Uint32()&0xffff,
		r.Uint32()&0xffff,
		r.Uint64()&0xffffffffffff)
}

func loadLines(filename string) []string {
	file, err := os.Open(filename)
	if err != nil {
		return nil
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func loadEnv() {
	if p := os.Getenv("ENV_FILE"); p != "" {
		_ = godotenv.Overload(p)
		return
	}
	candidates := []string{".env.windows", "env.windows", ".env.linux", "env.linux"}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			_ = godotenv.Overload(p)
			log.Printf("[env] loaded: %s", p)
			return
		}
	}
	for _, p := range candidates {
		rootPath := filepath.Join("..", "..", "..", p)
		if _, err := os.Stat(rootPath); err == nil {
			_ = godotenv.Overload(rootPath)
			log.Printf("[env] loaded: %s", rootPath)
			return
		}
	}
}

// ---------- Task Logic ----------

var deviceTemplates = []struct {
	m, b, r string
	d       int
}{
	{"SM-F936B", "samsung", "904*2105", 420},
	{"M2012K11AG", "xiaomi", "904*2105", 440},
	{"RMX3081", "realme", "904*2105", 480},
	{"Pixel 6", "google", "904*2105", 420},
	{"CPH2411", "oppo", "904*2105", 440},
}

func executeTask(awemeID string, proxy string, r *rand.Rand) bool {
	client := getClient(proxy)

	// 1. Device Register
	tpl := deviceTemplates[r.Intn(len(deviceTemplates))]
	deviceID := digitsLocal(19, r)
	installID := digitsLocal(19, r)
	cdid := uuid4Local(r)
	openudid := hexStringLocal(16, r)

	ts := time.Now().Unix()
	qsReg := fmt.Sprintf("device_platform=android&os=android&ssmix=a&_rticket=%d&cdid=%s&channel=googleplay&aid=1233&app_name=musical_ly&version_code=350003&version_name=35.0.3&resolution=%s&dpi=%d&device_type=%s&device_brand=%s&language=tr&os_api=34&os_version=14&ac=wifi&is_pad=1&current_region=TR&app_type=normal&sys_region=TR&is_foldable=1&timezone_name=Asia/Istanbul&timezone_offset=10800&build_number=35.0.3&host_abi=arm64-v8a&region=TR&ts=%d&iid=%s&device_id=%s&openudid=%s",
		ts*1000, cdid, tpl.r, tpl.d, tpl.m, tpl.b, ts, installID, deviceID, openudid)

	payloadReg := map[string]interface{}{
		"header": map[string]interface{}{
			"device_model": tpl.m, "device_brand": tpl.b, "os": "Android", "os_version": "14",
			"resolution": tpl.r, "density_dpi": tpl.d, "install_id": installID, "device_id": deviceID,
		},
		"magic_tag": "ss_app_log", "_gen_time": ts * 1000,
	}
	bodyReg, _ := json.Marshal(payloadReg)
	hReg := headers.MakeHeaders(deviceID, ts, 1, 0, 0, ts, "", tpl.m, "", 0, "", "", "", qsReg, hex.EncodeToString(bodyReg), "35.0.3", "v02.05.00-ov-android", 0x02050000, 738, 0)

	reqReg, _ := http.NewRequest("POST", "https://log22-normal-alisg.tiktokv.com/service/2/device_register/?"+qsReg, bytes.NewReader(bodyReg))
	reqReg.Header.Set("Content-Type", "application/json")
	reqReg.Header.Set("x-gorgon", hReg.XGorgon)
	reqReg.Header.Set("x-khronos", hReg.XKhronos)
	respReg, err := client.Do(reqReg)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, respReg.Body)
	respReg.Body.Close()

	// 2. View Stats
	ts = time.Now().Unix()
	payloadView := fmt.Sprintf("pre_item_playtime=915&user_algo_refresh_status=false&first_install_time=%d&item_id=%s&is_ad=0&follow_status=0&sync_origin=false&follower_status=0&action_time=%d&tab_type=22&pre_hot_sentence=&play_delta=1&request_id=&aweme_type=0&order=", ts-50000, awemeID, ts)

	var buf bytes.Buffer
	zw := gzipPool.Get().(*gzip.Writer)
	zw.Reset(&buf)
	zw.Write([]byte(payloadView))
	zw.Close()
	gzipPool.Put(zw)

	qsView := fmt.Sprintf("os=android&_rticket=%d&is_pad=1&last_install_time=%d&is_foldable=1&host_abi=arm64-v8a&ts=%d&ab_version=35.0.3&aid=1233&app_name=musical_ly&device_id=%s&iid=%s", ts*1000, ts-20000, ts, deviceID, installID)
	hView := headers.MakeHeaders(deviceID, ts, 1, 0, 0, ts, "", tpl.m, "", 0, "", "", "", qsView, hex.EncodeToString([]byte(payloadView)), "35.0.3", "v35.0.3-ov-android", 350003, 738, 0)

	reqView, _ := http.NewRequest("POST", "https://api31-core-alisg.tiktokv.com/aweme/v1/aweme/stats/?"+qsView, &buf)
	reqView.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqView.Header.Set("x-bd-content-encoding", "gzip")
	reqView.Header.Set("x-gorgon", hView.XGorgon)
	reqView.Header.Set("x-khronos", hView.XKhronos)

	respView, err := client.Do(reqView)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, respView.Body)
	respView.Body.Close()

	return respView.StatusCode == 200
}

// ---------- Concurrent Engine ----------

func (e *Engine) Run() {
	var wg sync.WaitGroup
	startTime := time.Now()

	// 1. Logging Goroutine
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s := atomic.LoadInt64(&e.currentSuccess)
				f := atomic.LoadInt64(&e.failed)
				t := atomic.LoadInt64(&e.total)
				rate := 0.0
				if t > 0 {
					rate = float64(s) / float64(t) * 100
				}
				fmt.Printf("[进度] 成功=%d, 失败=%d, 总数=%d, 成功率=%.2f%%\n", s, f, t, rate)
				if s >= e.cfg.TargetSuccess || t >= e.cfg.MaxRequests {
					e.cancel()
					return
				}
			case <-e.ctx.Done():
				return
			}
		}
	}()

	// 2. Worker Pool
	for i := 0; i < e.cfg.MaxConcurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			// Isolated rand source per worker to avoid global lock contention
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))
			for {
				select {
				case <-e.ctx.Done():
					return
				default:
				}

				// Hard stop check
				if atomic.LoadInt64(&e.currentSuccess) >= e.cfg.TargetSuccess ||
					atomic.LoadInt64(&e.total) >= e.cfg.MaxRequests {
					e.cancel()
					return
				}

				proxy := ""
				if len(e.proxies) > 0 {
					proxy = e.proxies[r.Intn(len(e.proxies))]
				}

				if executeTask(e.cfg.AwemeID, proxy, r) {
					atomic.AddInt64(&e.currentSuccess, 1)
					if e.onPlaySuccess != nil {
						e.onPlaySuccess()
					}
				} else {
					atomic.AddInt64(&e.failed, 1)
					// Exponential Backoff / Small jitter to avoid CPU spinning on failure
					time.Sleep(time.Duration(20+r.Intn(50)) * time.Millisecond)
				}
				atomic.AddInt64(&e.total, 1)
			}
		}(i)
	}

	wg.Wait()
	log.Printf("[Engine] Finished in %.2fs. Total: %d, Success: %d",
		time.Since(startTime).Seconds(), atomic.LoadInt64(&e.total), atomic.LoadInt64(&e.currentSuccess))
}

// ---------- Order Mode Logic ----------

func getDB() (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
		envStr("DB_USER", "root"), envStr("DB_PASSWORD", "123456"),
		envStr("DB_HOST", "127.0.0.1"), envStr("DB_PORT", "3306"),
		envStr("DB_NAME", "tiktok_go_play"))
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(50)
	return db, db.Ping()
}

func claimOrder(ctx context.Context, db *sql.DB) (*Order, error) {
	stale := envInt("STATS_ORDER_STALE_SEC", 120)
	for {
		select {
		case <-ctx.Done():
			return nil, nil
		default:
		}

		var o Order
		err := db.QueryRowContext(ctx, `
			SELECT id, order_id, aweme_id, quantity, delivered 
			FROM orders 
			WHERE (status IN ('Pending','Partial') 
			   OR (status='In progress' AND updated_at < NOW() - INTERVAL ? SECOND)) 
			  AND delivered < quantity 
			ORDER BY id ASC LIMIT 1`, stale).Scan(&o.ID, &o.OrderID, &o.AwemeID, &o.Quantity, &o.Delivered)

		if err == sql.ErrNoRows {
			time.Sleep(2 * time.Second)
			continue
		}
		if err != nil {
			return nil, err
		}

		// Atomic Claim
		res, err := db.ExecContext(ctx, `
			UPDATE orders 
			SET status='In progress', updated_at=NOW() 
			WHERE id=? 
			  AND (status IN ('Pending','Partial') 
			   OR (status='In progress' AND updated_at < NOW() - INTERVAL ? SECOND))`, o.ID, stale)
		if err != nil {
			return nil, err
		}
		ra, _ := res.RowsAffected()
		if ra > 0 {
			return &o, nil
		}
	}
}

func flushOrderProgress(db *sql.DB, orderID string, delta int64) {
	if delta <= 0 {
		return
	}
	_, _ = db.Exec(`
		UPDATE orders 
		SET delivered = LEAST(quantity, delivered + ?), 
		    status = CASE WHEN delivered + ? >= quantity THEN 'Completed' ELSE 'In progress' END, 
		    updated_at = NOW() 
		WHERE order_id = ?`, delta, delta, orderID)
}

func finalizeOrderStatus(db *sql.DB, orderID string) {
	var d, q int64
	_ = db.QueryRow("SELECT delivered, quantity FROM orders WHERE order_id=?", orderID).Scan(&d, &q)
	status := "In progress"
	if d >= q {
		status = "Completed"
	} else if d > 0 {
		status = "Partial"
	} else {
		status = "Pending"
	}
	_, _ = db.Exec("UPDATE orders SET status=?, updated_at=NOW() WHERE order_id=?", status, orderID)
}

// ---------- Main Entry ----------

func main() {
	loadEnv()

	flags := struct {
		TargetSuccess  int64
		MaxRequests    int64
		MaxConcurrency int
		AwemeID        string
		OrderMode      bool
		ProxyPath      string
	}{
		MaxConcurrency: 100,
		TargetSuccess:  1000,
	}

	flag.Int64Var(&flags.TargetSuccess, "n", flags.TargetSuccess, "Target success count")
	flag.Int64Var(&flags.MaxRequests, "max", 0, "Max total requests (default n * 5)")
	flag.IntVar(&flags.MaxConcurrency, "c", flags.MaxConcurrency, "Concurrency")
	flag.StringVar(&flags.AwemeID, "id", "", "TikTok Video ID")
	flag.BoolVar(&flags.OrderMode, "order", envBool("STATS_ORDER_MODE", false), "Enable database order mode")
	flag.StringVar(&flags.ProxyPath, "proxy", "proxies.txt", "Path to proxy list file")
	flag.Parse()

	proxies := loadLines(flags.ProxyPath)
	if len(proxies) == 0 {
		proxies = loadLines(filepath.Join("..", "..", "..", flags.ProxyPath))
	}

	if flags.OrderMode {
		log.Println("[Main] Starting in Order Mode...")
		db, err := getDB()
		if err != nil {
			log.Fatalf("[Main] DB Connection failed: %v", err)
		}
		defer db.Close()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		mainCtx, mainCancel := context.WithCancel(context.Background())
		go func() { <-sigChan; mainCancel() }()

		for {
			order, err := claimOrder(mainCtx, db)
			if err != nil || order == nil {
				break
			}

			log.Printf("[Main] Claimed Order %s for Video %s (Need: %d)",
				order.OrderID, order.AwemeID, order.Quantity-order.Delivered)

			var batchSuccessCount int64
			batchCtx, batchCancel := context.WithCancel(mainCtx)

			engine := &Engine{
				cfg: &EngineConfig{
					MaxConcurrency: flags.MaxConcurrency,
					TargetSuccess:  order.Quantity - order.Delivered,
					MaxRequests:    (order.Quantity - order.Delivered) * 10, // Generous limit for orders
					AwemeID:        order.AwemeID,
				},
				ctx:           batchCtx,
				cancel:        batchCancel,
				proxies:       proxies,
				onPlaySuccess: func() { atomic.AddInt64(&batchSuccessCount, 1) },
			}

			// Progress Persistence Ticker
			stopFlush := make(chan struct{})
			go func() {
				tk := time.NewTicker(5 * time.Second)
				defer tk.Stop()
				for {
					select {
					case <-tk.C:
						d := atomic.SwapInt64(&batchSuccessCount, 0)
						flushOrderProgress(db, order.OrderID, d)
					case <-stopFlush:
						return
					}
				}
			}()

			engine.Run()
			close(stopFlush)

			// Final sync for this order
			flushOrderProgress(db, order.OrderID, atomic.SwapInt64(&batchSuccessCount, 0))
			finalizeOrderStatus(db, order.OrderID)
		}
	} else {
		// Manual Mode
		if flags.AwemeID == "" {
			fmt.Print("Video ID: ")
			fmt.Scanln(&flags.AwemeID)
		}
		if flags.TargetSuccess <= 0 {
			fmt.Print("Target Success Count: ")
			fmt.Scanln(&flags.TargetSuccess)
		}
		if flags.MaxRequests <= 0 {
			flags.MaxRequests = flags.TargetSuccess * 5
		}

		log.Printf("[Main] Starting in Manual Mode: ID=%s, Target=%d, MaxTotal=%d, Concurrency=%d",
			flags.AwemeID, flags.TargetSuccess, flags.MaxRequests, flags.MaxConcurrency)

		ctx, cancel := context.WithCancel(context.Background())
		engine := &Engine{
			cfg: &EngineConfig{
				MaxConcurrency: flags.MaxConcurrency,
				TargetSuccess:  flags.TargetSuccess,
				MaxRequests:    flags.MaxRequests,
				AwemeID:        flags.AwemeID,
			},
			ctx:     ctx,
			cancel:  cancel,
			proxies: proxies,
		}
		engine.Run()
	}
}
