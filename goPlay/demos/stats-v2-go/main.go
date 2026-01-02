package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/hex"
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

// ---------- Config & Types ----------

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

// ---------- Resource Pools ----------

var (
	clientMap sync.Map
	gzipPool  = sync.Pool{
		New: func() interface{} {
			zw, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
			return zw
		},
	}
	bufferPool = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}
)

func getClient(proxy string) *http.Client {
	if client, ok := clientMap.Load(proxy); ok {
		return client.(*http.Client)
	}

	transport := &http.Transport{
		MaxIdleConns:        5000,
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConnsPerHost: 500,
		DisableCompression:  true,
		ForceAttemptHTTP2:   false,
	}

	if proxy != "" {
		if pUrl, err := url.Parse(proxy); err == nil {
			transport.Proxy = http.ProxyURL(pUrl)
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}

	actual, _ := clientMap.LoadOrStore(proxy, client)
	return actual.(*http.Client)
}

// ---------- Fast Helpers ----------

func writeInt(b *bytes.Buffer, i int64) {
	b.WriteString(strconv.FormatInt(i, 10))
}

func writeIntVal(b *bytes.Buffer, i int) {
	b.WriteString(strconv.Itoa(i))
}

func uuid4Fast(r *rand.Rand) string {
	var buf [36]byte
	const charset = "0123456789abcdef"
	for i := 0; i < 36; i++ {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			buf[i] = '-'
		} else {
			buf[i] = charset[r.Intn(16)]
		}
	}
	return string(buf[:])
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
	now := time.Now().Unix()
	nowMS := now * 1000

	tpl := deviceTemplates[r.Intn(len(deviceTemplates))]
	deviceID := digitsLocal(19, r)
	installID := digitsLocal(19, r)
	cdid := uuid4Fast(r)
	openudid := hexStringLocal(16, r)

	// QS Build
	qsb := bufferPool.Get().(*bytes.Buffer)
	qsb.Reset()
	qsb.WriteString("device_platform=android&os=android&ssmix=a&_rticket=")
	writeInt(qsb, nowMS)
	qsb.WriteString("&cdid=")
	qsb.WriteString(cdid)
	qsb.WriteString("&channel=googleplay&aid=1233&app_name=musical_ly&version_code=350003&version_name=35.0.3&resolution=")
	qsb.WriteString(tpl.r)
	qsb.WriteString("&dpi=")
	writeIntVal(qsb, tpl.d)
	qsb.WriteString("&device_type=")
	qsb.WriteString(tpl.m)
	qsb.WriteString("&device_brand=")
	qsb.WriteString(tpl.b)
	qsb.WriteString("&language=tr&os_api=34&os_version=14&ac=wifi&is_pad=1&current_region=TR&app_type=normal&sys_region=TR&is_foldable=1&timezone_name=Asia/Istanbul&timezone_offset=10800&build_number=35.0.3&host_abi=arm64-v8a&region=TR&ts=")
	writeInt(qsb, now)
	qsb.WriteString("&iid=")
	qsb.WriteString(installID)
	qsb.WriteString("&device_id=")
	qsb.WriteString(deviceID)
	qsb.WriteString("&openudid=")
	qsb.WriteString(openudid)
	qsReg := qsb.String()
	bufferPool.Put(qsb)

	// JSON Build (Manual, avoiding Marshal reflection)
	bodyRegStr := `{"header":{"device_model":"` + tpl.m + `","device_brand":"` + tpl.b + `","os":"Android","os_version":"14","resolution":"` + tpl.r + `","density_dpi":` + strconv.Itoa(tpl.d) + `,"install_id":"` + installID + `","device_id":"` + deviceID + `"},"magic_tag":"ss_app_log","_gen_time":` + strconv.FormatInt(nowMS, 10) + `}`

	hReg := headers.MakeHeaders(deviceID, now, 1, 0, 0, now, "", tpl.m, "", 0, "", "", "", qsReg, hex.EncodeToString([]byte(bodyRegStr)), "35.0.3", "v02.05.00-ov-android", 0x02050000, 738, 0)

	reqReg, _ := http.NewRequest("POST", "https://log22-normal-alisg.tiktokv.com/service/2/device_register/?"+qsReg, strings.NewReader(bodyRegStr))
	reqReg.Header.Set("Content-Type", "application/json")
	reqReg.Header.Set("x-gorgon", hReg.XGorgon)
	reqReg.Header.Set("x-khronos", hReg.XKhronos)

	respReg, err := client.Do(reqReg)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, respReg.Body)
	respReg.Body.Close()

	if respReg.StatusCode != 200 {
		return false
	}

	// 2. View Stats
	pb := bufferPool.Get().(*bytes.Buffer)
	pb.Reset()
	pb.WriteString("pre_item_playtime=915&user_algo_refresh_status=false&first_install_time=")
	writeInt(pb, now-50000)
	pb.WriteString("&item_id=")
	pb.WriteString(awemeID)
	pb.WriteString("&is_ad=0&follow_status=0&sync_origin=false&follower_status=0&action_time=")
	writeInt(pb, now)
	pb.WriteString("&tab_type=22&pre_hot_sentence=&play_delta=1&request_id=&aweme_type=0&order=")
	payloadView := pb.String()
	bufferPool.Put(pb)

	var buf bytes.Buffer
	zw := gzipPool.Get().(*gzip.Writer)
	zw.Reset(&buf)
	zw.Write([]byte(payloadView))
	zw.Close()
	gzipPool.Put(zw)

	qsvb := bufferPool.Get().(*bytes.Buffer)
	qsvb.Reset()
	qsvb.WriteString("os=android&_rticket=")
	writeInt(qsvb, nowMS)
	qsvb.WriteString("&is_pad=1&last_install_time=")
	writeInt(qsvb, now-20000)
	qsvb.WriteString("&is_foldable=1&host_abi=arm64-v8a&ts=")
	writeInt(qsvb, now)
	qsvb.WriteString("&ab_version=35.0.3&aid=1233&app_name=musical_ly&device_id=")
	qsvb.WriteString(deviceID)
	qsvb.WriteString("&iid=")
	qsvb.WriteString(installID)
	qsView := qsvb.String()
	bufferPool.Put(qsvb)

	hView := headers.MakeHeaders(deviceID, now, 1, 0, 0, now, "", tpl.m, "", 0, "", "", "", qsView, hex.EncodeToString([]byte(payloadView)), "35.0.3", "v35.0.3-ov-android", 350003, 738, 0)

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

// ---------- Engine ----------

func (e *Engine) Run() {
	var wg sync.WaitGroup
	startTime := time.Now()

	log.Printf("[Engine] Started with %d concurrency. Target: %d", e.cfg.MaxConcurrency, e.cfg.TargetSuccess)

	// Progress Logger
	go func() {
		ticker := time.NewTicker(1 * time.Second) // Faster UI updates
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
				if s >= e.cfg.TargetSuccess || (e.cfg.MaxRequests > 0 && t >= e.cfg.MaxRequests) {
					e.cancel()
					return
				}
			case <-e.ctx.Done():
				return
			}
		}
	}()

	for i := 0; i < e.cfg.MaxConcurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))

			for {
				select {
				case <-e.ctx.Done():
					return
				default:
				}

				// Check global state
				if atomic.LoadInt64(&e.currentSuccess) >= e.cfg.TargetSuccess ||
					(e.cfg.MaxRequests > 0 && atomic.LoadInt64(&e.total) >= e.cfg.MaxRequests) {
					e.cancel()
					return
				}

				proxy := ""
				if len(e.proxies) > 0 {
					proxy = e.proxies[r.Intn(len(e.proxies))]
				}

				// Execute and Atomic Update immediately for better UX
				if executeTask(e.cfg.AwemeID, proxy, r) {
					atomic.AddInt64(&e.currentSuccess, 1)
					if e.onPlaySuccess != nil {
						e.onPlaySuccess()
					}
				} else {
					atomic.AddInt64(&e.failed, 1)
					// Small backoff on failure to prevent CPU spin
					time.Sleep(time.Duration(10+r.Intn(50)) * time.Millisecond)
				}
				atomic.AddInt64(&e.total, 1)
			}
		}(i)
	}

	wg.Wait()
	log.Printf("[Engine] Finished in %.2fs. Success: %d",
		time.Since(startTime).Seconds(), atomic.LoadInt64(&e.currentSuccess))
}

// ---------- Helpers ----------

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
	sb.Grow(n)
	for i := 0; i < n; i++ {
		sb.WriteByte(byte('0' + r.Intn(10)))
	}
	return sb.String()
}

func hexStringLocal(n int, r *rand.Rand) string {
	const charset = "0123456789abcdef"
	var sb strings.Builder
	sb.Grow(n)
	for i := 0; i < n; i++ {
		sb.WriteByte(charset[r.Intn(len(charset))])
	}
	return sb.String()
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
}

// ---------- Database ----------

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

// ---------- Main ----------

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

	flag.Int64Var(&flags.TargetSuccess, "n", flags.TargetSuccess, "Target success")
	flag.Int64Var(&flags.MaxRequests, "max", 0, "Max requests")
	flag.IntVar(&flags.MaxConcurrency, "c", flags.MaxConcurrency, "Concurrency")
	flag.StringVar(&flags.AwemeID, "id", "", "Video ID")
	flag.BoolVar(&flags.OrderMode, "order", envBool("STATS_ORDER_MODE", false), "Order mode")
	flag.StringVar(&flags.ProxyPath, "proxy", "proxies.txt", "Proxy path")
	flag.Parse()

	proxies := loadLines(flags.ProxyPath)
	if len(proxies) == 0 {
		proxies = loadLines(filepath.Join("..", "..", "..", flags.ProxyPath))
	}

	if flags.OrderMode {
		log.Println("[Main] Starting in Order Mode...")
		db, err := getDB()
		if err != nil {
			log.Fatalf("DB fail: %v", err)
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

			log.Printf("[Main] Order %s, ID %s, Goal %d", order.OrderID, order.AwemeID, order.Quantity-order.Delivered)

			var batchSuccessCount int64
			batchCtx, batchCancel := context.WithCancel(mainCtx)

			engine := &Engine{
				cfg: &EngineConfig{
					MaxConcurrency: flags.MaxConcurrency,
					TargetSuccess:  order.Quantity - order.Delivered,
					MaxRequests:    (order.Quantity - order.Delivered) * 10,
					AwemeID:        order.AwemeID,
				},
				ctx:           batchCtx,
				cancel:        batchCancel,
				proxies:       proxies,
				onPlaySuccess: func() { atomic.AddInt64(&batchSuccessCount, 1) },
			}

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
			flushOrderProgress(db, order.OrderID, atomic.SwapInt64(&batchSuccessCount, 0))
			finalizeOrderStatus(db, order.OrderID)
		}
	} else {
		if flags.AwemeID == "" {
			fmt.Print("Video ID: ")
			fmt.Scanln(&flags.AwemeID)
		}
		if flags.TargetSuccess <= 0 {
			fmt.Print("Target Success: ")
			fmt.Scanln(&flags.TargetSuccess)
		}
		if flags.MaxRequests <= 0 {
			flags.MaxRequests = flags.TargetSuccess * 10
		}

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
