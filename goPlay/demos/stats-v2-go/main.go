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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tt_code/headers"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"
)

// ---------- Metrics ----------

var (
	metricSuccess = promauto.NewCounter(prometheus.CounterOpts{Name: "stats_v2_success_total"})
	metricFailed  = promauto.NewCounter(prometheus.CounterOpts{Name: "stats_v2_failed_total"})
	metricQPS     = promauto.NewGauge(prometheus.GaugeOpts{Name: "stats_v2_current_qps"})
)

// ---------- Types ----------

type EngineConfig struct {
	MaxConcurrency int
	TargetSuccess  int64
	MaxRequests    int64
	AwemeID        string
	GlobalRate     int
	ReuseCount     int // Device hits reuse
	ProxyLimit     int // How many times a proxy conn can be used
	StarryBase     string
}

type DeviceTemplate struct {
	m, b, r string
	d       int
}
type DeviceInfo struct {
	DeviceID, InstallID, CDID, OpenUDID string
	Template                            DeviceTemplate
}

type ProxySlot struct {
	URL   string
	Used  int64
	Limit int64
}

type Engine struct {
	cfg            *EngineConfig
	currentSuccess int64
	failed         int64
	total          int64
	activeWorkers  int32
	ctx            context.Context
	cancel         context.CancelFunc
	proxies        []string
	proxyPool      []*ProxySlot
	proxyPoolIdx   int64
	errReg         int64
	errStats       int64
	errNet         int64
	limiter        *rate.Limiter
	onPlaySuccess  func()
}

// ---------- Resource Pools ----------

var (
	clientMap  sync.Map
	gzipPool   = sync.Pool{New: func() interface{} { zw, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed); return zw }}
	bufferPool = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}
)

func getClient(proxy string) *http.Client {
	if client, ok := clientMap.Load(proxy); ok {
		return client.(*http.Client)
	}
	transport := &http.Transport{
		MaxIdleConns: 10000, MaxIdleConnsPerHost: 1000, IdleConnTimeout: 120 * time.Second,
		DisableKeepAlives: false, DisableCompression: true, ForceAttemptHTTP2: false,
	}
	if proxy != "" {
		purl := proxy
		// Simple normalization for Starry scheme socks5h
		purl = strings.Replace(purl, "socks5h://", "socks5://", 1)
		if !strings.Contains(purl, "://") {
			purl = "http://" + purl
		}
		if u, err := url.Parse(purl); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	client := &http.Client{Transport: transport, Timeout: 20 * time.Second}
	actual, _ := clientMap.LoadOrStore(proxy, client)
	return actual.(*http.Client)
}

// ---------- Pipeline Methods ----------

func RegisterDevice(client *http.Client, r *rand.Rand) (*DeviceInfo, error) {
	ts := time.Now().Unix()
	tsMS := ts * 1000
	tpl := deviceTemplates[r.Intn(len(deviceTemplates))]
	info := &DeviceInfo{
		DeviceID: digitsLocal(19, r), InstallID: digitsLocal(19, r),
		CDID: uuid4Fast(r), OpenUDID: hexStringLocal(16, r), Template: tpl,
	}
	qsb := bufferPool.Get().(*bytes.Buffer)
	qsb.Reset()
	defer bufferPool.Put(qsb)
	qsb.WriteString("device_platform=android&os=android&ssmix=a&_rticket=")
	writeInt(qsb, tsMS)
	qsb.WriteString("&cdid=")
	qsb.WriteString(info.CDID)
	qsb.WriteString("&resolution=")
	qsb.WriteString(tpl.r)
	qsb.WriteString("&dpi=")
	writeIntVal(qsb, tpl.d)
	qsb.WriteString("&device_type=")
	qsb.WriteString(tpl.m)
	qsb.WriteString("&device_brand=")
	qsb.WriteString(tpl.b)
	qsb.WriteString("&ts=")
	writeInt(qsb, ts)
	qsb.WriteString("&iid=")
	qsb.WriteString(info.InstallID)
	qsb.WriteString("&device_id=")
	qsb.WriteString(info.DeviceID)
	qsb.WriteString("&openudid=")
	qsb.WriteString(info.OpenUDID)
	qs := qsb.String()
	body := `{"header":{"device_model":"` + tpl.m + `","device_brand":"` + tpl.b + `","install_id":"` + info.InstallID + `","device_id":"` + info.DeviceID + `"},"magic_tag":"ss_app_log","_gen_time":` + strconv.FormatInt(tsMS, 10) + `}`
	h := headers.MakeHeaders(info.DeviceID, ts, 1, 0, 0, ts, "", tpl.m, "", 0, "", "", "", qs, hex.EncodeToString([]byte(body)), "35.0.3", "v02.05.00-ov-android", 0x02050000, 738, 0)
	req, _ := http.NewRequest("POST", "https://log22-normal-alisg.tiktokv.com/service/2/device_register/?"+qs, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-gorgon", h.XGorgon)
	req.Header.Set("x-khronos", h.XKhronos)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("reg status %d", resp.StatusCode)
	}
	return info, nil
}

func SendStats(client *http.Client, info *DeviceInfo, awemeID string) error {
	ts := time.Now().Unix()
	payload := "pre_item_playtime=915&user_algo_refresh_status=false&item_id=" + awemeID + "&action_time=" + strconv.FormatInt(ts, 10) + "&tab_type=22&play_delta=1"
	var buf bytes.Buffer
	zw := gzipPool.Get().(*gzip.Writer)
	zw.Reset(&buf)
	zw.Write([]byte(payload))
	zw.Close()
	gzipPool.Put(zw)
	qs := "os=android&device_id=" + info.DeviceID + "&iid=" + info.InstallID + "&ts=" + strconv.FormatInt(ts, 10)
	h := headers.MakeHeaders(info.DeviceID, ts, 1, 0, 0, ts, "", info.Template.m, "", 0, "", "", "", qs, hex.EncodeToString([]byte(payload)), "35.0.3", "v35.0.3-ov-android", 350003, 738, 0)
	req, _ := http.NewRequest("POST", "https://api31-core-alisg.tiktokv.com/aweme/v1/aweme/stats/?"+qs, &buf)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("x-bd-content-encoding", "gzip")
	req.Header.Set("x-gorgon", h.XGorgon)
	req.Header.Set("x-khronos", h.XKhronos)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("stats status %d", resp.StatusCode)
	}
	return nil
}

// ---------- Engine Core ----------

func (e *Engine) Run() {
	var wg sync.WaitGroup
	startTime := time.Now()
	lastTotal, lastTime := int64(0), time.Now()

	log.Printf("[Engine] Running. Concurrency=%d, ProxyPoolSize=%d", e.cfg.MaxConcurrency, len(e.proxyPool))

	// UI Logger
	go func() {
		tk := time.NewTicker(2 * time.Second)
		defer tk.Stop()
		for {
			select {
			case <-tk.C:
				n, lt := time.Now(), atomic.LoadInt64(&e.total)
				dt := n.Sub(lastTime).Seconds()
				qps := float64(lt-lastTotal) / dt
				lastTotal, lastTime = lt, n
				s, f := atomic.LoadInt64(&e.currentSuccess), atomic.LoadInt64(&e.failed)
				eReg, eStats, eNet := atomic.LoadInt64(&e.errReg), atomic.LoadInt64(&e.errStats), atomic.LoadInt64(&e.errNet)
				fmt.Printf("[进度] QPS=%.1f, 成功=%d, 失败=%d(Reg:%d,Stats:%d,Net:%d), 总数=%d, 线程=%d\n",
					qps, s, f, eReg, eStats, eNet, lt, atomic.LoadInt32(&e.activeWorkers))
				metricQPS.Set(qps)
				if s >= e.cfg.TargetSuccess || (e.cfg.MaxRequests > 0 && lt >= e.cfg.MaxRequests) {
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
		atomic.AddInt32(&e.activeWorkers, 1)
		go e.taskWorker(&wg, i)
	}

	wg.Wait()
	log.Printf("[Engine] Finished in %.2fs. Total: %d, Success: %d", time.Since(startTime).Seconds(), atomic.LoadInt64(&e.total), atomic.LoadInt64(&e.currentSuccess))
}

func (e *Engine) getProxy() string {
	if len(e.proxyPool) > 0 {
		// Use usage-limited rotation
		for {
			idx := atomic.AddInt64(&e.proxyPoolIdx, 1) % int64(len(e.proxyPool))
			p := e.proxyPool[idx]
			if e.cfg.ProxyLimit > 0 {
				if atomic.AddInt64(&p.Used, 1) > int64(e.cfg.ProxyLimit) {
					continue // This connection slot used up, skip to next
				}
			}
			return p.URL
		}
	}
	if len(e.proxies) > 0 {
		return e.proxies[rand.Intn(len(e.proxies))]
	}
	return ""
}

func (e *Engine) taskWorker(wg *sync.WaitGroup, id int) {
	defer wg.Done()
	defer atomic.AddInt32(&e.activeWorkers, -1)
	r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(id)))

	var device *DeviceInfo
	var reuseLeft int

	for {
		select {
		case <-e.ctx.Done():
			return
		default:
		}
		if e.limiter != nil {
			_ = e.limiter.Wait(e.ctx)
		}
		if atomic.LoadInt64(&e.currentSuccess) >= e.cfg.TargetSuccess {
			return
		}

		proxy := e.getProxy()
		client := getClient(proxy)

		if device == nil || reuseLeft <= 0 {
			info, err := RegisterDevice(client, r)
			if err != nil {
				atomic.AddInt64(&e.failed, 1)
				metricFailed.Inc()
				atomic.AddInt64(&e.total, 1)
				if strings.Contains(err.Error(), "reg status") {
					atomic.AddInt64(&e.errReg, 1)
				} else {
					atomic.AddInt64(&e.errNet, 1)
				}
				continue
			}
			device = info
			reuseLeft = e.cfg.ReuseCount
			if reuseLeft <= 0 {
				reuseLeft = 1
			}
		}

		err := SendStats(client, device, e.cfg.AwemeID)
		if err == nil {
			atomic.AddInt64(&e.currentSuccess, 1)
			metricSuccess.Inc()
			reuseLeft--
			if e.onPlaySuccess != nil {
				e.onPlaySuccess()
			}
		} else {
			atomic.AddInt64(&e.failed, 1)
			metricFailed.Inc()
			if strings.Contains(err.Error(), "stats status") {
				atomic.AddInt64(&e.errStats, 1)
			} else {
				atomic.AddInt64(&e.errNet, 1)
			}
			device = nil // Force new registration on hit failure
			time.Sleep(time.Duration(5+r.Intn(20)) * time.Millisecond)
		}
		atomic.AddInt64(&e.total, 1)
	}
}

// ---------- Utils ----------

func createStarryPool(base string, concurrency int, limit int) []*ProxySlot {
	if !strings.Contains(base, "://") {
		base = "socks5://" + base
	}
	u, err := url.Parse(base)
	if err != nil {
		log.Fatalf("Invalid starry proxy base: %v", err)
		return nil
	}

	newPool := make([]*ProxySlot, 0, concurrency*2)
	baseUser := u.User.Username()
	pass, _ := u.User.Password()

	for i := 1; i <= concurrency*2; i++ {
		// Rule: Append -conn-N to username
		newUser := fmt.Sprintf("%s-conn-%d", baseUser, i)
		u.User = url.UserPassword(newUser, pass)
		newPool = append(newPool, &ProxySlot{
			URL:   u.String(),
			Limit: int64(limit),
		})
	}
	return newPool
}

func uuid4Fast(r *rand.Rand) string {
	var b [36]byte
	const cs = "0123456789abcdef"
	for i := 0; i < 36; i++ {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			b[i] = '-'
		} else {
			b[i] = cs[r.Intn(16)]
		}
	}
	return string(b[:])
}

func digitsLocal(n int, r *rand.Rand) string {
	var s strings.Builder
	s.Grow(n)
	for i := 0; i < n; i++ {
		s.WriteByte(byte('0' + r.Intn(10)))
	}
	return s.String()
}

func hexStringLocal(n int, r *rand.Rand) string {
	const cs = "0123456789abcdef"
	var s strings.Builder
	s.Grow(n)
	for i := 0; i < n; i++ {
		s.WriteByte(cs[r.Intn(len(cs))])
	}
	return s.String()
}

func writeInt(b *bytes.Buffer, i int64)  { b.WriteString(strconv.FormatInt(i, 10)) }
func writeIntVal(b *bytes.Buffer, i int) { b.WriteString(strconv.Itoa(i)) }

func loadLines(p string) []string {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	var l []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		t := strings.TrimSpace(sc.Text())
		if t != "" {
			l = append(l, t)
		}
	}
	return l
}

func loadEnv() {
	if p := os.Getenv("ENV_FILE"); p != "" {
		_ = godotenv.Overload(p)
		return
	}
	for _, p := range []string{".env.windows", "env.windows", ".env.linux", "env.linux"} {
		if _, err := os.Stat(p); err == nil {
			_ = godotenv.Overload(p)
			return
		}
	}
}

// ---------- Database ----------

type Order struct {
	ID                  int64
	OrderID, AwemeID    string
	Quantity, Delivered int64
}

func getDB() (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", envStr("DB_USER", "root"), envStr("DB_PASSWORD", "123456"), envStr("DB_HOST", "127.0.0.1"), envStr("DB_PORT", "3306"), envStr("DB_NAME", "tiktok_go_play"))
	db, _ := sql.Open("mysql", dsn)
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
		err := db.QueryRowContext(ctx, `SELECT id, order_id, aweme_id, quantity, delivered FROM orders WHERE (status IN ('Pending','Partial') OR (status='In progress' AND updated_at < NOW() - INTERVAL ? SECOND)) AND delivered < quantity ORDER BY id ASC LIMIT 1`, stale).Scan(&o.ID, &o.OrderID, &o.AwemeID, &o.Quantity, &o.Delivered)
		if err == sql.ErrNoRows {
			time.Sleep(2 * time.Second)
			continue
		}
		if err != nil {
			return nil, err
		}
		res, _ := db.ExecContext(ctx, `UPDATE orders SET status='In progress', updated_at=NOW() WHERE id=?`, o.ID)
		if ra, _ := res.RowsAffected(); ra > 0 {
			return &o, nil
		}
	}
}

func flushOrderProgress(db *sql.DB, id string, d int64) {
	if d > 0 {
		_, _ = db.Exec(`UPDATE orders SET delivered = LEAST(quantity, delivered + ?), updated_at = NOW() WHERE order_id = ?`, d, id)
	}
}

func finalizeOrderStatus(db *sql.DB, id string) {
	var d, q int64
	_ = db.QueryRow("SELECT delivered, quantity FROM orders WHERE order_id=?", id).Scan(&d, &q)
	st := "Pending"
	if d >= q {
		st = "Completed"
	} else if d > 0 {
		st = "Partial"
	}
	_, _ = db.Exec("UPDATE orders SET status=?, updated_at=NOW() WHERE order_id=?", st, id)
}

// ---------- App Start ----------

var deviceTemplates = []DeviceTemplate{
	{"SM-F936B", "samsung", "904*2105", 420}, {"M2012K11AG", "xiaomi", "904*2105", 440}, {"RMX3081", "realme", "904*2105", 480}, {"Pixel 6", "google", "904*2105", 420}, {"CPH2411", "oppo", "904*2105", 440},
}

func envStr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func envInt(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return d
}
func envBool(k string, d bool) bool {
	v := strings.ToLower(os.Getenv(k))
	return v == "1" || v == "true"
}

func main() {
	loadEnv()
	flags := struct {
		TargetSuccess  int64
		MaxConcurrency int
		AwemeID        string
		OrderMode      bool
		ProxyPath      string
		Rate           int
		Reuse          int
		EnableProm     bool
		StarryProxy    string
		ProxyUseLimit  int
	}{MaxConcurrency: 100, TargetSuccess: 1000, Reuse: 1, ProxyUseLimit: 0}

	flag.Int64Var(&flags.TargetSuccess, "n", flags.TargetSuccess, "Target count")
	flag.IntVar(&flags.MaxConcurrency, "c", flags.MaxConcurrency, "Concurrency")
	flag.StringVar(&flags.AwemeID, "id", "", "Video ID")
	flag.BoolVar(&flags.OrderMode, "order", envBool("STATS_ORDER_MODE", false), "DB mode")
	flag.StringVar(&flags.ProxyPath, "proxy", "proxies.txt", "Proxy path")
	flag.IntVar(&flags.Rate, "rate", 0, "Global QPS 0=max")
	flag.IntVar(&flags.Reuse, "reuse", 1, "Stats reuse per device")
	flag.StringVar(&flags.StarryProxy, "pstarry", "", "Starry proxy base URL")
	flag.IntVar(&flags.ProxyUseLimit, "plimit", 0, "Max usage per proxy connection slot")
	flag.BoolVar(&flags.EnableProm, "prom", false, "Prometheus metrics")
	flag.Parse()

	if flags.EnableProm {
		go func() { http.Handle("/metrics", promhttp.Handler()); _ = http.ListenAndServe(":2112", nil) }()
	}

	var pool []*ProxySlot
	if flags.StarryProxy != "" {
		pool = createStarryPool(flags.StarryProxy, flags.MaxConcurrency, flags.ProxyUseLimit)
	}

	proxies := loadLines(flags.ProxyPath)
	if len(proxies) == 0 && strings.Contains(flags.ProxyPath, ":") {
		proxies = []string{flags.ProxyPath}
	}

	if flags.OrderMode {
		db, _ := getDB()
		defer db.Close()
		mCtx, cancel := context.WithCancel(context.Background())
		for {
			order, _ := claimOrder(mCtx, db)
			if order == nil {
				break
			}
			var delta int64
			oCtx, oKill := context.WithCancel(mCtx)
			var l *rate.Limiter
			if flags.Rate > 0 {
				l = rate.NewLimiter(rate.Limit(flags.Rate), flags.Rate)
			}
			engine := &Engine{
				cfg: &EngineConfig{MaxConcurrency: flags.MaxConcurrency, TargetSuccess: order.Quantity - order.Delivered, AwemeID: order.AwemeID, GlobalRate: flags.Rate, ReuseCount: flags.Reuse, ProxyLimit: flags.ProxyUseLimit, StarryBase: flags.StarryProxy},
				ctx: oCtx, cancel: oKill, proxies: proxies, proxyPool: pool, limiter: l, onPlaySuccess: func() { atomic.AddInt64(&delta, 1) },
			}
			go func() {
				tk := time.NewTicker(10 * time.Second)
				for {
					select {
					case <-tk.C:
						flushOrderProgress(db, order.OrderID, atomic.SwapInt64(&delta, 0))
					case <-oCtx.Done():
						return
					}
				}
			}()
			engine.Run()
			flushOrderProgress(db, order.OrderID, atomic.SwapInt64(&delta, 0))
			finalizeOrderStatus(db, order.OrderID)
		}
		cancel()
	} else {
		var l *rate.Limiter
		if flags.Rate > 0 {
			l = rate.NewLimiter(rate.Limit(flags.Rate), flags.Rate)
		}
		ctx, cancel := context.WithCancel(context.Background())
		engine := &Engine{
			cfg: &EngineConfig{MaxConcurrency: flags.MaxConcurrency, TargetSuccess: flags.TargetSuccess, AwemeID: flags.AwemeID, GlobalRate: flags.Rate, ReuseCount: flags.Reuse, ProxyLimit: flags.ProxyUseLimit, StarryBase: flags.StarryProxy},
			ctx: ctx, cancel: cancel, proxies: proxies, proxyPool: pool, limiter: l,
		}
		engine.Run()
		cancel()
	}
}
