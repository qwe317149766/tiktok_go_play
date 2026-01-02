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

// ---------- Config & Types ----------

type Config struct {
	MaxConcurrency int
	TargetSuccess  int64
	MaxRequests    int64
	AwemeID        string
	ProxyFile      string
	OrderMode      bool
}

var config = Config{
	MaxConcurrency: 100,
	TargetSuccess:  1000,
	MaxRequests:    5000,
	ProxyFile:      "proxies.txt",
}

type Order struct {
	ID        int64
	OrderID   string
	AwemeID   string
	Quantity  int64
	Delivered int64
}

type Engine struct {
	maxConcurrency int
	currentSuccess int64
	failed         int64
	total          int64
	stopChan       chan struct{}
	stopOnce       sync.Once
	proxies        []string
	onPlaySuccess  func()
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
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "on" || v == "yes"
}

func digits(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteByte(byte('0' + rand.Intn(10)))
	}
	return sb.String()
}

func hexString(n int) string {
	const charset = "0123456789abcdef"
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteByte(charset[rand.Intn(len(charset))])
	}
	return sb.String()
}

func uuid4() string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		rand.Uint32(),
		rand.Uint32()&0xffff,
		rand.Uint32()&0xffff,
		rand.Uint32()&0xffff,
		rand.Uint64()&0xffffffffffff)
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
	// root
	for _, p := range candidates {
		rootPath := filepath.Join("..", "..", "..", p)
		if _, err := os.Stat(rootPath); err == nil {
			_ = godotenv.Overload(rootPath)
			log.Printf("[env] loaded: %s", rootPath)
			return
		}
	}
}

// ---------- Business Logic (Task) ----------

func getDensityClass(dpi int) string {
	if dpi < 400 {
		return "hdpi"
	}
	if dpi < 440 {
		return "xhdpi"
	}
	if dpi < 480 {
		return "xxhdpi"
	}
	return "xxxhdpi"
}

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

func executeTask(awemeID string, proxy string) bool {
	client := &http.Client{Timeout: 15 * time.Second}
	if proxy != "" {
		pUrl, _ := url.Parse(proxy)
		client.Transport = &http.Transport{Proxy: http.ProxyURL(pUrl)}
	}

	// 1. Device Register
	tpl := deviceTemplates[rand.Intn(len(deviceTemplates))]
	deviceID := digits(19)
	installID := digits(19)
	cdid := uuid4()
	openudid := hexString(16)

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
	defer respReg.Body.Close()

	// 2. View Stats
	ts = time.Now().Unix()
	payloadView := fmt.Sprintf("pre_item_playtime=915&user_algo_refresh_status=false&first_install_time=%d&item_id=%s&is_ad=0&follow_status=0&sync_origin=false&follower_status=0&action_time=%d&tab_type=22&pre_hot_sentence=&play_delta=1&request_id=&aweme_type=0&order=", ts-50000, awemeID, ts)
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte(payloadView))
	zw.Close()

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
	defer respView.Body.Close()

	return respView.StatusCode == 200
}

// ---------- Engine ----------

func (e *Engine) Run() {
	var wg sync.WaitGroup
	startTime := time.Now()
	stopLog := make(chan struct{})

	// Progress Logger
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
				if s >= config.TargetSuccess || t >= config.MaxRequests {
					e.stopOnce.Do(func() { close(e.stopChan) })
					return
				}
			case <-stopLog:
				return
			case <-e.stopChan:
				return
			}
		}
	}()

	// Workers
	for i := 0; i < e.maxConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-e.stopChan:
					return
				default:
				}
				if atomic.LoadInt64(&e.currentSuccess) >= config.TargetSuccess {
					e.stopOnce.Do(func() { close(e.stopChan) })
					return
				}

				proxy := ""
				if len(e.proxies) > 0 {
					proxy = e.proxies[rand.Intn(len(e.proxies))]
				}

				if executeTask(config.AwemeID, proxy) {
					atomic.AddInt64(&e.currentSuccess, 1)
					if e.onPlaySuccess != nil {
						e.onPlaySuccess()
					}
				} else {
					atomic.AddInt64(&e.failed, 1)
				}
				atomic.AddInt64(&e.total, 1)
			}
		}()
	}

	wg.Wait()
	close(stopLog)
	log.Printf("[Engine] Finished in %.2fs. Success: %d", time.Since(startTime).Seconds(), e.currentSuccess)
}

// ---------- Order Mode ----------

func getDB() (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", envStr("DB_USER", "root"), envStr("DB_PASSWORD", "123456"), envStr("DB_HOST", "127.0.0.1"), envStr("DB_PORT", "3306"), envStr("DB_NAME", "tiktok_go_play"))
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
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
		err := db.QueryRowContext(ctx, "SELECT id, order_id, aweme_id, quantity, delivered FROM orders WHERE (status IN ('Pending','Partial') OR (status='In progress' AND updated_at < NOW() - INTERVAL ? SECOND)) AND delivered < quantity LIMIT 1", stale).Scan(&o.ID, &o.OrderID, &o.AwemeID, &o.Quantity, &o.Delivered)
		if err == sql.ErrNoRows {
			time.Sleep(2 * time.Second)
			continue
		}
		if err != nil {
			return nil, err
		}

		res, err := db.ExecContext(ctx, "UPDATE orders SET status='In progress', updated_at=NOW() WHERE id=? AND status IN ('Pending','Partial','In progress')", o.ID)
		if err != nil {
			return nil, err
		}
		ra, _ := res.RowsAffected()
		if ra > 0 {
			return &o, nil
		}
	}
}

func flushOrder(db *sql.DB, orderID string, delta int64) {
	if delta <= 0 {
		return
	}
	_, _ = db.Exec("UPDATE orders SET delivered=LEAST(quantity, delivered + ?), status=CASE WHEN delivered+? >= quantity THEN 'Completed' ELSE 'In progress' END, updated_at=NOW() WHERE order_id=?", delta, delta, orderID)
}

func finalizeOrder(db *sql.DB, orderID string) {
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

// ---------- Main ----------

func main() {
	loadEnv()
	flag.Int64Var(&config.TargetSuccess, "n", config.TargetSuccess, "Target count")
	flag.IntVar(&config.MaxConcurrency, "c", config.MaxConcurrency, "Concurrency")
	flag.StringVar(&config.AwemeID, "id", config.AwemeID, "Video ID")
	flag.BoolVar(&config.OrderMode, "order", envBool("STATS_ORDER_MODE", false), "Order mode")
	proxyPath := flag.String("proxy", "proxies.txt", "Proxy file")
	flag.Parse()

	proxies := loadLines(*proxyPath)
	if len(proxies) == 0 {
		proxies = loadLines(filepath.Join("..", "..", "..", *proxyPath))
	}

	if config.OrderMode {
		log.Println("[Order] Starting Order Worker...")
		db, err := getDB()
		if err != nil {
			log.Fatalf("DB connect failed: %v", err)
		}

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		ctx, stop := context.WithCancel(context.Background())
		go func() { <-sig; stop() }()

		for {
			order, err := claimOrder(ctx, db)
			if err != nil || order == nil {
				break
			}

			log.Printf("[Order] Processing %s (Goal: %d)", order.OrderID, order.Quantity-order.Delivered)
			config.AwemeID = order.AwemeID
			config.TargetSuccess = order.Quantity - order.Delivered
			config.MaxRequests = config.TargetSuccess * 5

			var delta int64
			engine := &Engine{maxConcurrency: config.MaxConcurrency, stopChan: make(chan struct{}), proxies: proxies}
			engine.onPlaySuccess = func() { atomic.AddInt64(&delta, 1) }

			// Flush Ticker
			done := make(chan bool)
			go func() {
				tk := time.NewTicker(5 * time.Second)
				for {
					select {
					case <-tk.C:
						d := atomic.SwapInt64(&delta, 0)
						flushOrder(db, order.OrderID, d)
					case <-done:
						return
					}
				}
			}()

			engine.Run()
			close(done)
			flushOrder(db, order.OrderID, atomic.SwapInt64(&delta, 0))
			finalizeOrder(db, order.OrderID)
		}
	} else {
		if config.AwemeID == "" {
			fmt.Print("ID: ")
			fmt.Scanln(&config.AwemeID)
		}
		if config.TargetSuccess <= 0 {
			fmt.Print("Count: ")
			fmt.Scanln(&config.TargetSuccess)
		}
		engine := &Engine{maxConcurrency: config.MaxConcurrency, stopChan: make(chan struct{}), proxies: proxies}
		engine.Run()
	}
}
