package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"runtime"
)

// ---------- rolling gzip compressor (best-effort, non-blocking) ----------

type gzipCompressor struct {
	enabled bool
	q       chan string
	wg      sync.WaitGroup
}

var globalGzipOnce sync.Once
var globalGzip *gzipCompressor

func gzipEnabled() bool {
	// 1=开启滚动后 gzip 压缩（默认开启）
	return envBool("STATS_ROTATE_GZIP", true)
}

func gzipWorkers() int {
	n := envInt("STATS_GZIP_WORKERS", 2)
	if n <= 0 {
		return 2
	}
	if n > 16 {
		return 16
	}
	return n
}

func gzipQueueSize() int {
	n := envInt("STATS_GZIP_QUEUE_SIZE", 200)
	if n <= 0 {
		return 200
	}
	return n
}

func getGzipCompressor() *gzipCompressor {
	globalGzipOnce.Do(func() {
		enabled := gzipEnabled()
		gc := &gzipCompressor{
			enabled: enabled,
			q:       make(chan string, gzipQueueSize()),
		}
		if enabled {
			for i := 0; i < gzipWorkers(); i++ {
				gc.wg.Add(1)
				go func() {
					defer gc.wg.Done()
					for path := range gc.q {
						_ = gzipFileReplace(path)
					}
				}()
			}
		}
		globalGzip = gc
	})
	return globalGzip
}

func (gc *gzipCompressor) Enqueue(path string) {
	if gc == nil || !gc.enabled {
		return
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	if strings.HasSuffix(strings.ToLower(path), ".gz") {
		return
	}
	// non-blocking：队列满就跳过（不影响主流程）
	select {
	case gc.q <- path:
	default:
	}
}

func (gc *gzipCompressor) Close() {
	if gc == nil || !gc.enabled {
		return
	}
	close(gc.q)
	gc.wg.Wait()
}

func gzipFileReplace(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	// 已不存在则忽略（可能被清理/重复入队）
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	in, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer in.Close()

	outPath := path + ".gz"
	tmpPath := outPath + ".tmp"
	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return nil
	}

	gw := gzip.NewWriter(out)
	_, copyErr := io.Copy(gw, in)
	closeErr := gw.Close()
	_ = out.Close()

	if copyErr != nil || closeErr != nil {
		_ = os.Remove(tmpPath)
		return nil
	}
	// 原子替换
	_ = os.Remove(outPath) // 覆盖旧的 gz（若存在）
	if err := os.Rename(tmpPath, outPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil
	}
	// 压缩成功后删除原文件
	_ = os.Remove(path)
	return nil
}

// Config 配置
type Config struct {
	MaxConcurrency int
	TargetSuccess  int64
	MaxRequests    int64
	Proxies        []string
	Devices        []string
	AwemeID        string
	ResultFile     string
	ErrorFile      string
}

var (
	config = Config{
		MaxConcurrency: 500, // 进一步提高并发数以优化速度
		TargetSuccess:  10000,
		MaxRequests:    19000,
		AwemeID:        "7569635953183100191",
		ResultFile:     "results.jsonl",
		ErrorFile:      "error.log",
	}
	cacheFile = "device_cache.txt" // 设备缓存文件
)

// TaskResult 任务结果
type TaskResult struct {
	TaskID  int
	Success bool
	Extra   map[string]interface{}
	Time    string
}

// StatsResult stats请求结果
type StatsResult struct {
	Res string
	Err error
}

// ResultWriter 结果写入器 - 并行写版本（主任务非阻塞投递）
// 设计目标：不管成功/失败，都不能干预主任务运行。
// 策略：主任务写入时使用 non-blocking send；队列满则丢弃并计数（避免卡住主流程）。
type ResultWriter struct {
	baseFilename string
	maxBytes     int64 // 单文件最大字节数（到达后滚动）
	workers      int
	queueSize    int
	dropped      int64

	chans []chan TaskResult
	wg    sync.WaitGroup
	done  chan struct{}
}

type resultWorker struct {
	id           int
	baseFilename string
	maxBytes     int64
	queue        <-chan TaskResult
	done         <-chan struct{}
	gzipper      *gzipCompressor

	file         *os.File
	currentBytes int64
	part         int
	batch        []TaskResult
}

func (rw *ResultWriter) Dropped() int64 {
	if rw == nil {
		return 0
	}
	return atomic.LoadInt64(&rw.dropped)
}

func getStatsResultMaxBytes() int64 {
	// 结果文件最大体积（MB），默认 20MB
	mb := envInt("STATS_RESULT_MAX_MB", 20)
	if mb <= 0 {
		mb = 20
	}
	return int64(mb) * 1024 * 1024
}

func getStatsResultWriterWorkers() int {
	n := envInt("STATS_RESULT_WRITER_WORKERS", 4)
	if n <= 0 {
		return 4
	}
	if n > 32 {
		return 32
	}
	return n
}

func getStatsResultQueueSize() int {
	n := envInt("STATS_RESULT_QUEUE_SIZE", 20000)
	if n <= 0 {
		return 20000
	}
	return n
}

func makePartFilename(base string, workerID int, part int) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "results.jsonl"
	}
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if ext == "" {
		// 没有扩展名时也保持可读
		return fmt.Sprintf("%s_w%02d_part%04d", name, workerID, part)
	}
	return fmt.Sprintf("%s_w%02d_part%04d%s", name, workerID, part, ext)
}

func (w *resultWorker) openForPart(part int) error {
	filename := makePartFilename(w.baseFilename, w.id, part)
	dir := filepath.Dir(filename)
	if dir != "." && dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	w.file = f
	w.part = part
	// 初始化当前文件大小（用于滚动判断）
	if st, err := f.Stat(); err == nil {
		w.currentBytes = st.Size()
	} else {
		w.currentBytes = 0
	}
	return nil
}

func NewResultWriter(filename string) (*ResultWriter, error) {
	gz := getGzipCompressor()
	rw := &ResultWriter{
		baseFilename: filename,
		maxBytes:     getStatsResultMaxBytes(),
		workers:      getStatsResultWriterWorkers(),
		queueSize:    getStatsResultQueueSize(),
		done:         make(chan struct{}),
	}
	rw.chans = make([]chan TaskResult, 0, rw.workers)
	for i := 0; i < rw.workers; i++ {
		ch := make(chan TaskResult, rw.queueSize)
		rw.chans = append(rw.chans, ch)
		w := &resultWorker{
			id:           i,
			baseFilename: rw.baseFilename,
			maxBytes:     rw.maxBytes,
			queue:        ch,
			done:         rw.done,
			gzipper:      gz,
			batch:        make([]TaskResult, 0, 200),
		}
		// 每个 worker 从 part0001 开始写自己的文件
		if err := w.openForPart(1); err != nil {
			return nil, err
		}
		rw.wg.Add(1)
		go func(ww *resultWorker) {
			defer rw.wg.Done()
			ww.run()
		}(w)
	}

	return rw, nil
}

func (rw *ResultWriter) Write(result TaskResult) {
	if len(rw.chans) == 0 {
		return
	}
	// 用 TaskID 做分片，保证同一 task 更稳定地落到同一 writer
	idx := 0
	if result.TaskID >= 0 {
		idx = result.TaskID % len(rw.chans)
	}
	select {
	case rw.chans[idx] <- result:
		// ok
	default:
		// 队列满：丢弃，确保不影响主任务
		atomic.AddInt64(&rw.dropped, 1)
	}
}

func (w *resultWorker) run() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	flush := func() {
		if len(w.batch) == 0 {
			return
		}
		for _, result := range w.batch {
			data, _ := json.Marshal(result)
			line := string(data) + "\n"
			// 到达阈值时滚动到下一个文件（默认 20MB）
			if w.maxBytes > 0 && w.currentBytes+int64(len(line)) > w.maxBytes {
				// 关闭并异步压缩旧文件（best-effort，不阻塞）
				oldPart := w.part
				oldName := makePartFilename(w.baseFilename, w.id, oldPart)
				if w.file != nil {
					_ = w.file.Close()
				}
				if w.gzipper != nil {
					w.gzipper.Enqueue(oldName)
				}
				_ = w.openForPart(w.part + 1)
			}
			if w.file != nil {
				_, _ = w.file.WriteString(line)
				w.currentBytes += int64(len(line))
			}
		}
		w.batch = w.batch[:0]
	}
	for {
		select {
		case r, ok := <-w.queue:
			if !ok {
				flush()
				if w.file != nil {
					_ = w.file.Close()
				}
				return
			}
			w.batch = append(w.batch, r)
			if len(w.batch) >= 200 {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-w.done:
			flush()
			if w.file != nil {
				_ = w.file.Close()
			}
			return
		}
	}
}

func (rw *ResultWriter) Close() {
	close(rw.done)
	// 主任务不应被 Close 阻塞太久，但这里是退出阶段，允许等待落盘
	for _, ch := range rw.chans {
		close(ch)
	}
	rw.wg.Wait()
}

// ErrorWriter 错误日志写入器
type ErrorWriter struct {
	baseFilename string
	maxBytes     int64
	workers      int
	queueSize    int
	dropped      int64

	chans []chan string
	wg    sync.WaitGroup
	done  chan struct{}
}

func NewErrorWriter(filename string) (*ErrorWriter, error) {
	gz := getGzipCompressor()
	ew := &ErrorWriter{
		baseFilename: filename,
		maxBytes:     getStatsErrorMaxBytes(),
		workers:      getStatsErrorWriterWorkers(),
		queueSize:    getStatsErrorQueueSize(),
		done:         make(chan struct{}),
	}
	ew.chans = make([]chan string, 0, ew.workers)
	for i := 0; i < ew.workers; i++ {
		ch := make(chan string, ew.queueSize)
		ew.chans = append(ew.chans, ch)
		w := &errorWorker{
			id:           i,
			baseFilename: ew.baseFilename,
			maxBytes:     ew.maxBytes,
			queue:        ch,
			done:         ew.done,
			gzipper:      gz,
			batch:        make([]string, 0, 100),
		}
		if err := w.openForPart(1); err != nil {
			return nil, err
		}
		ew.wg.Add(1)
		go func(ww *errorWorker) {
			defer ew.wg.Done()
			ww.run()
		}(w)
	}

	return ew, nil
}

func (ew *ErrorWriter) Write(msg string) {
	if len(ew.chans) == 0 {
		return
	}
	// 简单 hash 分片，避免锁竞争；失败写入不会阻塞主任务
	h := int(fnv1a32(msg))
	if h < 0 {
		h = -h
	}
	idx := h % len(ew.chans)
	select {
	case ew.chans[idx] <- msg:
	default:
		atomic.AddInt64(&ew.dropped, 1)
	}
}

func (ew *ErrorWriter) Close() {
	close(ew.done)
	for _, ch := range ew.chans {
		close(ch)
	}
	ew.wg.Wait()
}

func (ew *ErrorWriter) Dropped() int64 {
	if ew == nil {
		return 0
	}
	return atomic.LoadInt64(&ew.dropped)
}

func getStatsErrorMaxBytes() int64 {
	// error.log 单文件最大体积（MB），默认 20MB
	mb := envInt("STATS_ERROR_MAX_MB", 20)
	if mb <= 0 {
		mb = 20
	}
	return int64(mb) * 1024 * 1024
}

func getStatsErrorWriterWorkers() int {
	n := envInt("STATS_ERROR_WRITER_WORKERS", 2)
	if n <= 0 {
		return 2
	}
	if n > 16 {
		return 16
	}
	return n
}

func getStatsErrorQueueSize() int {
	n := envInt("STATS_ERROR_QUEUE_SIZE", 5000)
	if n <= 0 {
		return 5000
	}
	return n
}

func fnv1a32(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

type errorWorker struct {
	id           int
	baseFilename string
	maxBytes     int64
	queue        <-chan string
	done         <-chan struct{}
	gzipper      *gzipCompressor

	file         *os.File
	currentBytes int64
	part         int
	batch        []string
}

func makeErrorPartFilename(base string, workerID int, part int) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "error.log"
	}
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if ext == "" {
		return fmt.Sprintf("%s_w%02d_part%04d", name, workerID, part)
	}
	return fmt.Sprintf("%s_w%02d_part%04d%s", name, workerID, part, ext)
}

func (w *errorWorker) openForPart(part int) error {
	filename := makeErrorPartFilename(w.baseFilename, w.id, part)
	dir := filepath.Dir(filename)
	if dir != "." && dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	w.file = f
	w.part = part
	if st, err := f.Stat(); err == nil {
		w.currentBytes = st.Size()
	} else {
		w.currentBytes = 0
	}
	return nil
}

func (w *errorWorker) run() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	flush := func() {
		if len(w.batch) == 0 {
			return
		}
		for _, msg := range w.batch {
			line := msg + "\n"
			if w.maxBytes > 0 && w.currentBytes+int64(len(line)) > w.maxBytes {
				// 关闭并异步压缩旧文件（best-effort，不阻塞）
				oldPart := w.part
				oldName := makeErrorPartFilename(w.baseFilename, w.id, oldPart)
				if w.file != nil {
					_ = w.file.Close()
				}
				if w.gzipper != nil {
					w.gzipper.Enqueue(oldName)
				}
				_ = w.openForPart(w.part + 1)
			}
			if w.file != nil {
				_, _ = w.file.WriteString(line)
				w.currentBytes += int64(len(line))
			}
		}
		w.batch = w.batch[:0]
	}
	for {
		select {
		case msg, ok := <-w.queue:
			if !ok {
				flush()
				if w.file != nil {
					_ = w.file.Close()
				}
				return
			}
			w.batch = append(w.batch, msg)
			if len(w.batch) >= 100 {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-w.done:
			flush()
			if w.file != nil {
				_ = w.file.Close()
			}
			return
		}
	}
}

// ErrorStats 错误统计
type ErrorStats struct {
	SeedErrors    int64 // seed获取失败
	TokenErrors   int64 // token获取失败
	StatsErrors   int64 // stats请求失败
	NetworkErrors int64 // 网络连接错误
	ParseErrors   int64 // 解析错误
	OtherErrors   int64 // 其他错误
	// stats 阶段的细分错误（你提到的“其他错误也要统计”）
	TimeoutErrors   int64 // 超时（含 context deadline / Client.Timeout / i/o timeout）
	HTTP403Errors   int64 // 403
	HTTP429Errors   int64 // 429
	HTTP5xxErrors   int64 // 5xx
	CaptchaErrors   int64 // captcha/verify
	EmptyRespErrors int64 // err=nil 但响应为空
}

func looksTimeout(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return false
	}
	return strings.Contains(s, "timeout") ||
		strings.Contains(s, "deadline exceeded") ||
		strings.Contains(s, "context deadline exceeded") ||
		strings.Contains(s, "client.timeout") ||
		strings.Contains(s, "i/o timeout")
}

func looksHTTP403(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, " 403") || strings.Contains(s, "status 403") || strings.Contains(s, "statuscode=403") || strings.Contains(s, "status code 403")
}

func looksHTTP429(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, " 429") || strings.Contains(s, "status 429") || strings.Contains(s, "statuscode=429") || strings.Contains(s, "status code 429") || strings.Contains(s, "too many requests")
}

func looksHTTP5xx(s string) bool {
	s = strings.ToLower(s)
	for _, code := range []string{"500", "502", "503", "504"} {
		if strings.Contains(s, " "+code) || strings.Contains(s, "status "+code) || strings.Contains(s, "statuscode="+code) || strings.Contains(s, "status code "+code) {
			return true
		}
	}
	return false
}

func looksCaptcha(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "captcha") || strings.Contains(s, "verify") || strings.Contains(s, "verification")
}

// Engine 高性能引擎
type Engine struct {
	proxyIndex  int64
	deviceIndex int64
	proxyMutex  sync.Mutex
	deviceMutex sync.Mutex
	// 设备替换：被淘汰的 poolID 不再回补（本次运行内）
	bannedDeviceMu sync.RWMutex
	bannedPoolIDs  map[string]bool
	// 设备淘汰统计
	evictedTotal int64
	evictedFail  int64
	evictedPlay  int64

	// Linux 抢单模式：成功回调（每次成功播放触发一次，用于更新 Redis/DB 进度）
	onPlaySuccess func()

	writer        *ResultWriter
	errorWriter   *ErrorWriter
	sem           chan struct{}
	proxyManager  *ProxyManager
	deviceManager *DeviceManager

	success    int64
	failed     int64
	total      int64
	// inflight：正在执行中的任务数（用于减少达到目标后的“额外一轮并发”）
	inflight   int64
	errorStats ErrorStats

	// 动态并发调整
	currentConcurrency int64
	concurrencyMu      sync.RWMutex
	minConcurrency     int
	maxConcurrency     int
	// 趋势：用“和上一次对比”的成功率来决定是否允许降并发，以及何时升并发
	lastAdjustTotal      int64
	lastSuccessRate      float64
	rateIncreaseStreak   int

	// 退出信号
	stopChan chan struct{}
	stopOnce sync.Once // 确保只关闭一次
}

func NewEngine() (*Engine, error) {
	writer, err := NewResultWriter(config.ResultFile)
	if err != nil {
		return nil, err
	}

	errorWriter, err := NewErrorWriter(config.ErrorFile)
	if err != nil {
		writer.Close()
		return nil, err
	}

	// 初始化代理管理器
	InitProxyManager(config.Proxies)

	// 初始化设备管理器（用于连续失败阈值触发替换）
	InitDeviceManager()
	// 初始化 cookies 管理器（用于连续失败阈值触发替换）
	InitCookieManager()

	// 初始化设备缓存
	InitDeviceCache(cacheFile)

	maxConc := config.MaxConcurrency * 2
	if maxConc < 1 {
		maxConc = 1
	}
	// 最小并发数：按“配置并发/2”计算（用户要求）
	minConc := config.MaxConcurrency / 2
	if minConc < 1 {
		minConc = 1
	}

	return &Engine{
		// 方式A：sem 容量=并发上限（用于硬上限保护），动态并发由 currentConcurrency 门控
		sem:                make(chan struct{}, maxConc),
		writer:             writer,
		errorWriter:        errorWriter,
		proxyManager:       GetProxyManager(),
		deviceManager:      GetDeviceManager(),
		bannedPoolIDs:      make(map[string]bool),
		currentConcurrency: int64(config.MaxConcurrency),
		minConcurrency:     minConc,                   // 最小并发数（=配置并发/2）
		maxConcurrency:     maxConc,                   // 最大并发数（默认 2 倍初始值）
		lastAdjustTotal:    0,
		lastSuccessRate:    -1,
		rateIncreaseStreak: 0,
		stopChan:           make(chan struct{}),       // 初始化stopChan
	}, nil
}

// acquireDynamicPermit：方式A（动态并发门控，真正生效）
// - sem：硬上限（容量=maxConcurrency）
// - currentConcurrency：动态允许的并发（<=maxConcurrency）
func (e *Engine) acquireDynamicPermit() bool {
	for {
		// 退出信号
		select {
		case <-e.stopChan:
			return false
		default:
		}

		// 先拿硬上限 token（达到 maxConcurrency 时阻塞）
		e.sem <- struct{}{}

		// 再按 currentConcurrency 动态门控（不满足则释放 token 并短暂 sleep）
		cur := atomic.LoadInt64(&e.currentConcurrency)
		if cur <= 0 {
			cur = 1
		}
		if cur > int64(e.maxConcurrency) {
			cur = int64(e.maxConcurrency)
		}

		for {
			select {
			case <-e.stopChan:
				<-e.sem
				return false
			default:
			}

			in := atomic.LoadInt64(&e.inflight)
			if in >= cur {
				<-e.sem
				time.Sleep(2 * time.Millisecond)
				break
			}
			if atomic.CompareAndSwapInt64(&e.inflight, in, in+1) {
				return true
			}
			runtime.Gosched()
		}
	}
}

func (e *Engine) releaseDynamicPermit() {
	atomic.AddInt64(&e.inflight, -1)
	<-e.sem
}

// adjustConcurrency 根据成功率动态调整并发数
func (e *Engine) adjustConcurrency() {
	total := atomic.LoadInt64(&e.total)
	if total < 100 { // 至少需要100个样本才开始调整
		return
	}

	success := atomic.LoadInt64(&e.success)
	successRate := float64(success) / float64(total)

	e.concurrencyMu.Lock()
	defer e.concurrencyMu.Unlock()

	// 防止 3 秒一次的进度 tick 波动造成误判：要求与上一次调整相比新增一定样本量
	const minNewSamples = int64(200)
	if e.lastAdjustTotal > 0 && total-e.lastAdjustTotal < minNewSamples {
		return
	}

	current := int(atomic.LoadInt64(&e.currentConcurrency))
	newConcurrency := current

	// 规则（按你的要求）：
	// - 如果成功率比上一次对比是增加的：则不能减少并发
	// - 如果成功率持续增加 3 次：则需要增加并发
	const eps = 0.001 // 防抖（避免浮点噪声触发 streak）
	if e.lastSuccessRate >= 0 {
		if successRate > e.lastSuccessRate+eps {
			e.rateIncreaseStreak++
			// 连续提升 3 次：提升并发
			if e.rateIncreaseStreak >= 3 && current < e.maxConcurrency {
				newConcurrency = current + 10
				if newConcurrency > e.maxConcurrency {
					newConcurrency = e.maxConcurrency
				}
				// 提升一次后清零 streak，避免每个 tick 都加
				e.rateIncreaseStreak = 0
			}
			// 注意：成功率上升时，绝不下降并发（即使 successRate < 0.5）
		} else if successRate < e.lastSuccessRate-eps {
			// 成功率下降：允许下降并发（保留原阈值，避免无意义抖动）
			e.rateIncreaseStreak = 0
			if successRate < 0.5 && current > e.minConcurrency {
				newConcurrency = current - 10
				if newConcurrency < e.minConcurrency {
					newConcurrency = e.minConcurrency
				}
			}
		} else {
			// 基本持平：不调整，不累计
			e.rateIncreaseStreak = 0
		}
	} else {
		// 第一次：只记录，不调整
		e.rateIncreaseStreak = 0
	}

	if newConcurrency != current {
		atomic.StoreInt64(&e.currentConcurrency, int64(newConcurrency))
		// 动态调整并发数（静默处理，不打印日志）
	}

	// 记录本次用于“下次对比”
	e.lastSuccessRate = successRate
	e.lastAdjustTotal = total
}

func (e *Engine) nextProxy() string {
	// 使用智能代理选择
	if e.proxyManager != nil {
		return e.proxyManager.GetNextProxy()
	}
	// 降级到简单轮询
	e.proxyMutex.Lock()
	defer e.proxyMutex.Unlock()
	if len(config.Proxies) == 0 {
		return ""
	}
	idx := atomic.AddInt64(&e.proxyIndex, 1) - 1
	return config.Proxies[int(idx)%len(config.Proxies)]
}

func (e *Engine) nextDevice() (int, string) {
	e.deviceMutex.Lock()
	defer e.deviceMutex.Unlock()
	if len(config.Devices) == 0 {
		return 0, ""
	}

	// 简化版本：直接轮询，健康检查在失败时进行
	idx := atomic.AddInt64(&e.deviceIndex, 1) - 1
	slot := int(idx) % len(config.Devices)
	deviceJSON := config.Devices[slot]

	// 快速提取device_id（只解析一次，不检查健康状态）
	// 健康检查在taskWrapper失败时进行，避免每次选择都解析JSON
	return slot, deviceJSON
}

func extractPoolIDFromDeviceJSON(deviceJSON string) string {
	var device map[string]interface{}
	if err := json.Unmarshal([]byte(deviceJSON), &device); err != nil {
		return ""
	}
	return devicePoolIDFromDevice(device)
}

func (e *Engine) snapshotActivePoolIDsLocked() map[string]bool {
	out := make(map[string]bool, len(config.Devices))
	for _, dj := range config.Devices {
		pid := extractPoolIDFromDeviceJSON(dj)
		if strings.TrimSpace(pid) != "" {
			out[pid] = true
		}
	}
	return out
}

// replaceBadDeviceIfNeeded：当某个 poolID 连续失败超过阈值时，从 Redis 设备池补一个新设备替换该 slot。
func (e *Engine) replaceBadDeviceIfNeeded(slot int, deviceJSON string, poolID string) {
	// 仅 Redis 设备来源才支持动态补位
	if !shouldLoadDevicesFromRedis() {
		return
	}
	if e.deviceManager == nil {
		return
	}
	if strings.TrimSpace(poolID) == "" {
		return
	}
	// 连续失败阈值：沿用 DeviceManager 的规则（阈值可配置）
	if e.deviceManager.IsHealthy(poolID) {
		return
	}

	e.replaceDevice(slot, deviceJSON, poolID, "consecutive_fail")
}

func (e *Engine) replaceDevice(slot int, deviceJSON string, poolID string, reason string) {
	// 加锁替换，保证与 nextDevice 互斥
	e.deviceMutex.Lock()
	defer e.deviceMutex.Unlock()
	// slot 可能越界（理论上不会），防御一下
	if slot < 0 || slot >= len(config.Devices) {
		return
	}
	// 若 slot 已被其它协程替换过，则不重复操作
	if config.Devices[slot] != deviceJSON {
		return
	}

	// 组装 exclude：当前活跃 + banned
	exclude := e.snapshotActivePoolIDsLocked()
	e.bannedDeviceMu.RLock()
	for k := range e.bannedPoolIDs {
		exclude[k] = true
	}
	e.bannedDeviceMu.RUnlock()

	newPoolID, newJSON, err := pickOneDeviceFromRedis(exclude)
	if err != nil || strings.TrimSpace(newJSON) == "" || strings.TrimSpace(newPoolID) == "" {
		return
	}

	// 替换 slot
	config.Devices[slot] = newJSON
	// ban 老的 poolID，避免后续再次被补回来
	e.bannedDeviceMu.Lock()
	e.bannedPoolIDs[poolID] = true
	e.bannedDeviceMu.Unlock()

	// 统计：淘汰次数
	atomic.AddInt64(&e.evictedTotal, 1)
	switch reason {
	case "consecutive_fail":
		atomic.AddInt64(&e.evictedFail, 1)
	case "play_max":
		atomic.AddInt64(&e.evictedPlay, 1)
	}
}

// executeTask 执行单个任务 - 优化版本
func executeTask(taskID int, awemeID, deviceJSON, proxy string) (bool, map[string]interface{}) {
	var device map[string]interface{}
	if err := json.Unmarshal([]byte(deviceJSON), &device); err != nil {
		return false, map[string]interface{}{
			"stage":   "parse",
			"reason":  err.Error(),
			"task_id": taskID,
			"proxy":   proxy,
			"device":  "parse_error",
		}
	}

	// 获取设备ID用于日志
	deviceID := "unknown"
	if id, ok := device["device_id"].(string); ok {
		deviceID = id
	} else if id, ok := device["device_id"].(float64); ok {
		deviceID = fmt.Sprintf("%.0f", id)
	}

	// Redis 设备池 ID（与 Python 写入的 key 保持一致）
	poolID := devicePoolIDFromDevice(device)

	// 转换device
	deviceMap := make(map[string]string)
	for k, v := range device {
		switch val := v.(type) {
		case string:
			deviceMap[k] = val
		case float64:
			deviceMap[k] = fmt.Sprintf("%.0f", val)
		case int:
			deviceMap[k] = fmt.Sprintf("%d", val)
		case int64:
			deviceMap[k] = fmt.Sprintf("%d", val)
		}
	}

	// 定义变量
	var seed string
	var seedType int
	var token string

	// 检查缓存（如果设备来自 Redis，则直接读/写 Redis，确保“缓存更新到 Python 注册设备信息”）
	if shouldLoadDevicesFromRedis() {
		if s, st, t, ok := getSeedTokenFromRedis(poolID); ok {
			seed, seedType, token = s, st, t
		}
	} else {
		cache := GetDeviceCache()
		if cacheInfo, exists := cache.Get(deviceID); exists {
			seed = cacheInfo.Seed
			seedType = cacheInfo.SeedType
			token = cacheInfo.Token
		}
	}

	if seed == "" || token == "" || seedType == 0 {
		// 缓存不存在，需要请求
		// 获取HTTP客户端（使用代理管理器）
		var client *http.Client
		if proxyManager := GetProxyManager(); proxyManager != nil {
			client = proxyManager.GetClient(proxy)
		} else {
			client = GetClientForProxy(proxy)
		}

		// 异步并行获取seed和token（同时发起，提高效率）
		seedChan := GetSeedAsync(deviceMap, client)
		tokenChan := GetTokenAsync(deviceMap, client)

		// 等待seed结果（带重试逻辑，最多3次）
		var err error
		seedRetry := 0
		maxSeedRetries := 3

		for seedRetry < maxSeedRetries {
			select {
			case seedResult := <-seedChan:
				if seedResult.Err == nil && seedResult.Seed != "" {
					seed = seedResult.Seed
					seedType = seedResult.SeedType
					break // 成功，退出循环
				} else {
					if seedRetry < maxSeedRetries-1 {
						// 重试：使用指数退避策略 (2^retry * baseDelay)
						baseDelay := 200 * time.Millisecond
						backoffDelay := time.Duration(1<<uint(seedRetry)) * baseDelay
						if backoffDelay > 5*time.Second {
							backoffDelay = 5 * time.Second // 最大延迟5秒
						}
						time.Sleep(backoffDelay)
						seedChan = GetSeedAsync(deviceMap, client)
						seedRetry++
						continue
					} else {
						err = seedResult.Err
						if err == nil {
							err = fmt.Errorf("empty seed")
						}
						seedRetry = maxSeedRetries
					}
				}
			case <-time.After(20 * time.Second):
				// 超时，重试（使用指数退避）
				if seedRetry < maxSeedRetries-1 {
					baseDelay := 200 * time.Millisecond
					backoffDelay := time.Duration(1<<uint(seedRetry)) * baseDelay
					if backoffDelay > 5*time.Second {
						backoffDelay = 5 * time.Second
					}
					time.Sleep(backoffDelay)
					seedChan = GetSeedAsync(deviceMap, client)
					seedRetry++
					continue
				} else {
					err = fmt.Errorf("seed request timeout")
					seedRetry = maxSeedRetries
				}
			}
			if seed != "" {
				break // 成功获取seed，退出循环
			}
		}

		if err != nil || seed == "" {
			reason := "empty seed"
			if err != nil {
				reason = fmt.Sprintf("empty seed: %v", err)
			}
			// 判断是否是网络错误
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			isNetworkError := strings.Contains(errStr, "connect") ||
				strings.Contains(errStr, "timeout") ||
				strings.Contains(errStr, "connection") ||
				strings.Contains(errStr, "wsarecv") ||
				strings.Contains(errStr, "panic")
			return false, map[string]interface{}{
				"stage":         "seed",
				"reason":        reason,
				"task_id":       taskID,
				"proxy":         proxy,
				"device_id":     deviceID,
				"network_error": isNetworkError,
				"error_detail":  errStr,
			}
		}

		// 等待token结果（带重试逻辑，最多3次）
		tokenRetry := 0
		maxTokenRetries := 3
		for tokenRetry < maxTokenRetries {
			select {
			case tokenResult := <-tokenChan:
				if tokenResult.Token != "" {
					token = tokenResult.Token
					break
				} else {
					if tokenRetry < maxTokenRetries-1 {
						// 使用指数退避策略
						baseDelay := 200 * time.Millisecond
						backoffDelay := time.Duration(1<<uint(tokenRetry)) * baseDelay
						if backoffDelay > 5*time.Second {
							backoffDelay = 5 * time.Second
						}
						time.Sleep(backoffDelay)
						tokenChan = GetTokenAsync(deviceMap, client)
						tokenRetry++
						continue
					} else {
						tokenRetry = maxTokenRetries
					}
				}
			case <-time.After(20 * time.Second):
				if tokenRetry < maxTokenRetries-1 {
					// 使用指数退避策略
					baseDelay := 200 * time.Millisecond
					backoffDelay := time.Duration(1<<uint(tokenRetry)) * baseDelay
					if backoffDelay > 5*time.Second {
						backoffDelay = 5 * time.Second
					}
					time.Sleep(backoffDelay)
					tokenChan = GetTokenAsync(deviceMap, client)
					tokenRetry++
					continue
				} else {
					tokenRetry = maxTokenRetries
				}
			}
			if token != "" {
				break
			}
		}

		if token == "" {
			// token 获取失败也算一次失败使用
			if shouldLoadDevicesFromRedis() {
				_ = incrDeviceFail(poolID, 1)
			}
			return false, map[string]interface{}{
				"stage":     "token",
				"reason":    "empty token after retries",
				"task_id":   taskID,
				"proxy":     proxy,
				"device_id": deviceID,
				"pool_id":   poolID,
			}
		}

		// 保存到缓存（Redis 模式：写回 Python 注册设备信息；文件模式：沿用 device_cache.txt）
		if shouldLoadDevicesFromRedis() {
			if err := setSeedTokenToRedis(poolID, seed, seedType, token); err != nil {
				return false, map[string]interface{}{
					"stage":     "cache",
					"reason":    fmt.Sprintf("write seed/token to redis failed: %v", err),
					"task_id":   taskID,
					"proxy":     proxy,
					"device_id": deviceID,
					"pool_id":   poolID,
				}
			}
		} else {
			cache := GetDeviceCache()
			cache.Set(deviceID, seed, seedType, token)
		}
	}

	// 执行stats请求 - 添加快速重试（最多2次）
	// 获取HTTP客户端（如果缓存中没有，client已经在上面获取了）
	var client *http.Client
	if proxyManager := GetProxyManager(); proxyManager != nil {
		client = proxyManager.GetClient(proxy)
	} else {
		client = GetClientForProxy(proxy)
	}

	signCount := 212
	var res string
	var err error
	// 执行stats请求 - 添加快速重试（最多2次）
	var ckID string
	var ck map[string]string
	for retry := 0; retry < 2; retry++ {
		// 尝试次数：每次发起 stats 请求即 +1
		if shouldLoadDevicesFromRedis() {
			_ = incrDeviceAttempt(poolID, 1)
		}
		// 使用defer recover来捕获panic
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic in Stats3: %v", r)
				}
			}()
			ckID, ck = getCookiesForTask(taskID)
			res, err = Stats3(awemeID, seed, seedType, token, device, ck, signCount, client)
		}()
		if err == nil {
			break
		}
		if retry < 1 {
			time.Sleep(100 * time.Millisecond) // 短暂延迟后重试
		}
	}
	if err != nil {
		if shouldLoadDevicesFromRedis() {
			_ = incrDeviceFail(poolID, 1)
		}
		// 判断是否是网络错误
		errStr := err.Error()
		isNetworkError := strings.Contains(errStr, "connect") ||
			strings.Contains(errStr, "timeout") ||
			strings.Contains(errStr, "connection") ||
			strings.Contains(errStr, "wsarecv")
		// cookie 连续失败统计（网络错误不计入连续失败）
		if cm := GetCookieManager(); cm != nil && ckID != "" {
			cm.RecordFailure(ckID, isNetworkError)
		}
		return false, map[string]interface{}{
			"stage":         "stats",
			"reason":        errStr,
			"task_id":       taskID,
			"proxy":         proxy,
			"device_id":     deviceID,
			"pool_id":       poolID,
			"network_error": isNetworkError,
		}
	}

	success := res != ""
	if !success {
		// 细分统计：err=nil 但响应为空，也算 stats 失败的一种
		// 统一给一个 reason，方便后续汇总
		if res == "" {
			// 只在没有其它 reason 的情况下补充
		}
	}
	// cookie 成功/失败统计：success=false 视为一次“非网络失败”（主要用于风控/被拒）
	// 注意：Stats3 err!=nil 的情况已在上面 return，这里只有 err==nil。
	if cm := GetCookieManager(); cm != nil && ckID != "" {
		if success {
			cm.RecordSuccess(ckID)
		} else {
			cm.RecordFailure(ckID, false)
		}
	}
	// 播放次数：只在成功时 +1
	if success && shouldLoadDevicesFromRedis() {
		// 记录 play_count，并返回当前值用于阈值淘汰
		if pc, err := incrDevicePlayGet(poolID, 1); err == nil {
			// 返回给上层用于淘汰判断
			//（注意：map 的 int64 在 JSON 序列化时会变成 number，不影响）
			result := map[string]interface{}{
				"stage":      "stats",
				"raw":        "",
				"pool_id":    poolID,
				"device_id":  deviceID,
				"play_count": pc,
			}
			if len(res) > 2000 {
				result["raw"] = res[:2000]
			} else {
				result["raw"] = res
			}
			return true, result
		} else {
			_ = incrDevicePlay(poolID, 1) // 降级：尽量不影响主流程
		}
	}
	result := map[string]interface{}{
		"stage":     "stats",
		"raw":       "",
		"pool_id":   poolID,
		"device_id": deviceID,
	}
	if !success {
		result["reason"] = "empty response"
		result["network_error"] = false
	}
	if len(res) > 2000 {
		result["raw"] = res[:2000]
	} else {
		result["raw"] = res
	}

	return success, result
}

func (e *Engine) taskWrapper(taskID int) {
	// 添加panic恢复，防止程序崩溃
	defer func() {
		if r := recover(); r != nil {
			// 静默处理panic，不打印日志
			atomic.AddInt64(&e.failed, 1)
			atomic.AddInt64(&e.total, 1)
		}
	}()
	// 方式A：动态并发门控（currentConcurrency 真正生效）
	if !e.acquireDynamicPermit() {
		return
	}
	defer e.releaseDynamicPermit()

	slot, deviceJSON := e.nextDevice()
	proxy := e.nextProxy()

	ok, extra := executeTask(taskID, config.AwemeID, deviceJSON, proxy)

	atomic.AddInt64(&e.total, 1)
	deviceID, _ := extra["device_id"].(string)
	// 用 poolID 做“设备健康/替换”的主键（与 Redis 设备池一致）
	poolID := extractPoolIDFromDeviceJSON(deviceJSON)

	if ok {
		atomic.AddInt64(&e.success, 1)
		// 记录代理成功
		if e.proxyManager != nil {
			e.proxyManager.RecordSuccess(proxy)
		}
		// 记录设备成功
		if e.deviceManager != nil {
			e.deviceManager.RecordSuccess(poolID)
		}
		if e.onPlaySuccess != nil {
			e.onPlaySuccess()
		}
		// 方式A（维度2）：成功播放达到阈值就淘汰并补位
		if shouldLoadDevicesFromRedis() && GetDevicePlayMax() > 0 {
			if v, ok2 := extra["play_count"]; ok2 {
				switch t := v.(type) {
				case int64:
					if t >= GetDevicePlayMax() {
						e.replaceDevice(slot, deviceJSON, poolID, "play_max")
					}
				case float64:
					if int64(t) >= GetDevicePlayMax() {
						e.replaceDevice(slot, deviceJSON, poolID, "play_max")
					}
				}
			}
		}
		// 成功，不打印日志
	} else {
		atomic.AddInt64(&e.failed, 1)
		// 记录代理失败
		if e.proxyManager != nil {
			e.proxyManager.RecordFailure(proxy)
		}
		// 记录设备失败
		if e.deviceManager != nil {
			// 注意：连续失败需要排除网络错误（network_error=true 不累加 ConsecutiveFailures）
			isNetworkError, _ := extra["network_error"].(bool)
			e.deviceManager.RecordFailure(poolID, isNetworkError)
			// 方式A：仅在“非网络错误导致的连续失败”达到阈值后动态补位
			if !isNetworkError {
				e.replaceBadDeviceIfNeeded(slot, deviceJSON, poolID)
			}
		}

		// 分类统计错误
		stage, _ := extra["stage"].(string)
		isNetworkError, _ := extra["network_error"].(bool)
		reason, _ := extra["reason"].(string)

		switch stage {
		case "seed":
			atomic.AddInt64(&e.errorStats.SeedErrors, 1)
			if isNetworkError {
				atomic.AddInt64(&e.errorStats.NetworkErrors, 1)
			}
		case "token":
			atomic.AddInt64(&e.errorStats.TokenErrors, 1)
		case "stats":
			atomic.AddInt64(&e.errorStats.StatsErrors, 1)
			// stats 阶段：补充细分统计（把“其他错误”拆出来）
			if isNetworkError || looksTimeout(reason) {
				atomic.AddInt64(&e.errorStats.NetworkErrors, 1)
			}
			if looksTimeout(reason) {
				atomic.AddInt64(&e.errorStats.TimeoutErrors, 1)
			}
			if looksHTTP403(reason) {
				atomic.AddInt64(&e.errorStats.HTTP403Errors, 1)
			}
			if looksHTTP429(reason) {
				atomic.AddInt64(&e.errorStats.HTTP429Errors, 1)
			}
			if looksHTTP5xx(reason) {
				atomic.AddInt64(&e.errorStats.HTTP5xxErrors, 1)
			}
			if looksCaptcha(reason) {
				atomic.AddInt64(&e.errorStats.CaptchaErrors, 1)
			}
			if strings.Contains(strings.ToLower(reason), "empty response") {
				atomic.AddInt64(&e.errorStats.EmptyRespErrors, 1)
			}
		case "parse":
			atomic.AddInt64(&e.errorStats.ParseErrors, 1)
		default:
			atomic.AddInt64(&e.errorStats.OtherErrors, 1)
		}

		// 不打印错误日志，只写入错误文件
		errorMsg := fmt.Sprintf("[%s] task=%d, stage=%s, reason=%s, proxy=%s, device=%s, network_error=%v",
			time.Now().Format(time.RFC3339), taskID, stage, reason, proxy, deviceID, isNetworkError)
		if e.errorWriter != nil {
			e.errorWriter.Write(errorMsg)
		}
	}

	// 异步写入结果，不阻塞
	e.writer.Write(TaskResult{
		TaskID:  taskID,
		Success: ok,
		Extra:   extra,
		Time:    time.Now().Format(time.RFC3339),
	})
}

func (e *Engine) Run() {
	defer func() {
		e.writer.Close()
		if e.errorWriter != nil {
			e.errorWriter.Close()
		}
		// 关闭压缩后台（等待压缩队列尽量处理完；仅影响退出阶段）
		getGzipCompressor().Close()
	}()

	startTime := time.Now()
	// 不打印开始日志，只打印进度日志

	var wg sync.WaitGroup
	taskID := int64(0)

	// 启动定期日志输出
	stopLog := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(3 * time.Second) // 更频繁的进度更新
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				success := atomic.LoadInt64(&e.success)
				failed := atomic.LoadInt64(&e.failed)
				total := atomic.LoadInt64(&e.total)
				rate := 0.0
				if total > 0 {
					rate = float64(success) / float64(total) * 100
				}
				// 详细错误统计
				seedErr := atomic.LoadInt64(&e.errorStats.SeedErrors)
				tokenErr := atomic.LoadInt64(&e.errorStats.TokenErrors)
				statsErr := atomic.LoadInt64(&e.errorStats.StatsErrors)
				networkErr := atomic.LoadInt64(&e.errorStats.NetworkErrors)
				parseErr := atomic.LoadInt64(&e.errorStats.ParseErrors)
				otherErr := atomic.LoadInt64(&e.errorStats.OtherErrors)
				tmoErr := atomic.LoadInt64(&e.errorStats.TimeoutErrors)
				h403 := atomic.LoadInt64(&e.errorStats.HTTP403Errors)
				h429 := atomic.LoadInt64(&e.errorStats.HTTP429Errors)
				h5xx := atomic.LoadInt64(&e.errorStats.HTTP5xxErrors)
				capErr := atomic.LoadInt64(&e.errorStats.CaptchaErrors)
				emptyErr := atomic.LoadInt64(&e.errorStats.EmptyRespErrors)
				// 设备淘汰统计（方式A）
				evAll := atomic.LoadInt64(&e.evictedTotal)
				evFail := atomic.LoadInt64(&e.evictedFail)
				evPlay := atomic.LoadInt64(&e.evictedPlay)
				e.bannedDeviceMu.RLock()
				bannedN := len(e.bannedPoolIDs)
				e.bannedDeviceMu.RUnlock()
				// cookies 淘汰统计（连续失败触发替换）
				ckRepl := getCookieReplacedTotal()
				ckBanned := getBannedCookieCount()

				dropRes := int64(0)
				if e.writer != nil {
					dropRes = e.writer.Dropped()
				}
				dropErr := int64(0)
				if e.errorWriter != nil {
					dropErr = e.errorWriter.Dropped()
				}
				curConc := atomic.LoadInt64(&e.currentConcurrency)

				log.Printf("[进度] 成功=%d, 失败=%d, 总数=%d, 成功率=%.2f%% | 错误分类: seed=%d, token=%d, stats=%d, network=%d, parse=%d, other=%d, timeout=%d, http403=%d, http429=%d, http5xx=%d, captcha=%d, empty=%d | 设备淘汰: total=%d (fail=%d, play=%d) banned=%d | Cookies更换: total=%d banned=%d | 并发(动态)=%d/%d | 写入丢弃: results=%d error=%d",
					success, failed, total, rate, seedErr, tokenErr, statsErr, networkErr, parseErr, otherErr, tmoErr, h403, h429, h5xx, capErr, emptyErr,
					evAll, evFail, evPlay, bannedN, ckRepl, ckBanned, curConc, int64(e.maxConcurrency), dropRes, dropErr)
				// 兜底：确保进度一定打印到 stdout（避免 log 输出被重定向/吞掉）
				fmt.Printf("[进度] 成功=%d, 失败=%d, 总数=%d, 成功率=%.2f%%\n", success, failed, total, rate)
				// 动态调整并发数
				e.adjustConcurrency()

				// 检查是否达到目标
				if success >= config.TargetSuccess || total >= config.MaxRequests {
					// 安全关闭stopChan（只关闭一次）
					e.stopOnce.Do(func() {
						close(e.stopChan)
					})
					return
				}
			case <-stopLog:
				return
			case <-e.stopChan:
				return
			}
		}
	}()

	// 使用worker pool模式：启动到并发上限，让 currentConcurrency 真正可以“向上扩容/向下收缩”
	for i := 0; i < e.maxConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				// 检查退出信号
				select {
				case <-e.stopChan:
					return
				default:
				}

				success := atomic.LoadInt64(&e.success)
				// 需求：去掉“提前判断总数(total>=MaxRequests)”的逻辑，让 total 允许超一轮；
				// 统一由进度 ticker（每3秒）按 total>=MaxRequests 触发停止即可。
				if success >= config.TargetSuccess {
					e.stopOnce.Do(func() {
						if e.stopChan != nil {
							close(e.stopChan)
						}
					})
					return
				}

				id := int(atomic.AddInt64(&taskID, 1))
				e.taskWrapper(id)
			}
		}()
	}

	wg.Wait()
	close(stopLog)

	// 最终详细统计
	elapsed := time.Since(startTime)
	success := atomic.LoadInt64(&e.success)
	failed := atomic.LoadInt64(&e.failed)
	total := atomic.LoadInt64(&e.total)
	seedErr := atomic.LoadInt64(&e.errorStats.SeedErrors)
	tokenErr := atomic.LoadInt64(&e.errorStats.TokenErrors)
	statsErr := atomic.LoadInt64(&e.errorStats.StatsErrors)
	networkErr := atomic.LoadInt64(&e.errorStats.NetworkErrors)
	parseErr := atomic.LoadInt64(&e.errorStats.ParseErrors)
	otherErr := atomic.LoadInt64(&e.errorStats.OtherErrors)
	tmoErr := atomic.LoadInt64(&e.errorStats.TimeoutErrors)
	h403 := atomic.LoadInt64(&e.errorStats.HTTP403Errors)
	h429 := atomic.LoadInt64(&e.errorStats.HTTP429Errors)
	h5xx := atomic.LoadInt64(&e.errorStats.HTTP5xxErrors)
	capErr := atomic.LoadInt64(&e.errorStats.CaptchaErrors)
	emptyErr := atomic.LoadInt64(&e.errorStats.EmptyRespErrors)

	successRate := 0.0
	if total > 0 {
		successRate = float64(success) / float64(total) * 100
	}

	fmt.Printf("\n========== 最终统计 ==========\n")
	fmt.Printf("总耗时: %.2f秒\n", elapsed.Seconds())
	fmt.Printf("成功: %d\n", success)
	fmt.Printf("失败: %d\n", failed)
	fmt.Printf("总数: %d\n", total)
	fmt.Printf("成功率: %.2f%%\n", successRate)
	fmt.Printf("错误分类统计：seed=%d, token=%d, stats=%d, network=%d, parse=%d, other=%d, timeout=%d, http403=%d, http429=%d, http5xx=%d, captcha=%d, empty=%d\n",
		seedErr, tokenErr, statsErr, networkErr, parseErr, otherErr, tmoErr, h403, h429, h5xx, capErr, emptyErr)
	fmt.Printf("=============================\n")
}

func loadLines(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
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
	return lines, scanner.Err()
}

func main() {
	rand.Seed(time.Now().UnixNano())
	loadEnvForDemo()
	applyRedisPoolShardFromArgs(os.Args)

	// 确保进度日志在控制台可见（有些环境 stderr 不明显/被上层吞掉）
	// 默认 log 输出到 stderr；这里强制到 stdout，避免“执行进度没有输出”的观感问题
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags)

	// 并发数：从 env 读取（统一配置）
	// 优先级：STATS_CONCURRENCY > GEN_CONCURRENCY > 代码默认值
	if v := envInt("STATS_CONCURRENCY", 0); v > 0 {
		config.MaxConcurrency = v
	} else if v := envInt("GEN_CONCURRENCY", 0); v > 0 {
		config.MaxConcurrency = v
	}

	proxiesPath := findTopmostFileUpwards("proxies.txt", 8)
	if proxiesPath == "" {
		proxiesPath = "proxies.txt"
	}
	// 设备文件路径（文件模式）
	// 优先级：
	// - DEVICES_FILE（统一配置，推荐）
	// - STATS_DEVICES_FILE（stats 专用，兼容）
	// - 向上查找仓库根目录的 devices.txt
	// - 当前目录 devices.txt
	devicesPath := strings.TrimSpace(os.Getenv("DEVICES_FILE"))
	if devicesPath == "" {
		devicesPath = strings.TrimSpace(os.Getenv("STATS_DEVICES_FILE"))
	}
	if devicesPath == "" {
		devicesPath = findTopmostFileUpwards("devices.txt", 8)
	}
	if devicesPath == "" {
		devicesPath = "devices.txt"
	}

	// 加载代理
	if data, err := ioutil.ReadFile(proxiesPath); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				config.Proxies = append(config.Proxies, line)
			}
		}
		fmt.Printf("已加载 %d 个代理\n", len(config.Proxies))
	} else {
		fmt.Printf("缺少 proxies.txt（请在仓库根目录放 proxies.txt）: %v\n", err)
		os.Exit(1)
	}

	// 加载设备：优先从 Python 注册成功写入的 Redis 设备池读取
	if shouldLoadDevicesFromRedis() {
		// 可观测性：明确告诉你当前是否在读“startup 注册成功设备池”
		if shouldLoadDevicesFromStartupRedis() {
			fmt.Printf("设备来源=startup_redis prefix=%s（来自 REDIS_STARTUP_DEVICE_POOL_KEY；未配置则回退 REDIS_DEVICE_POOL_KEY）\n",
				strings.TrimSpace(startupDevicePoolPrefix()))
		}
		if shouldLoadDevicesFromStartupCookieRedis() {
			fmt.Printf("设备来源=startup_cookie_redis prefix=%s（来自 REDIS_STARTUP_COOKIE_POOL_KEY；账号池=data 存完整账号JSON）\n",
				strings.TrimSpace(envStr("REDIS_STARTUP_COOKIE_POOL_KEY", "tiktok:startup_cookie_pool")))
		}
		// Redis 模式：用多少取多少
		// 默认按并发数取设备（例如并发 1000 就先取 1000 个），设备淘汰时再从 Redis 补位。
		need := envInt("DEVICES_LIMIT", 0)
		if need <= 0 {
			need = config.MaxConcurrency
		}
		if need <= 0 {
			need = 1
		}
		var devs []string
		var err error
		if shouldLoadDevicesFromStartupCookieRedis() {
			// 账号池为空时：按你的要求轮询等待，不要直接退出
			pollSec := envInt("STATS_ACCOUNT_POLL_INTERVAL_SEC", envInt("STATS_COOKIE_POLL_INTERVAL_SEC", envInt("COOKIES_POLL_INTERVAL_SEC", 10)))
			if pollSec <= 0 {
				pollSec = 10
			}
			for {
				devs, err = loadStartupAccountDevicesFromRedisN(need)
				if err == nil && len(devs) > 0 {
					break
				}
				fmt.Printf("账号池为空/无有效账号（REDIS_STARTUP_COOKIE_POOL_KEY=%s），将每 %d 秒轮询等待补齐...\n",
					strings.TrimSpace(envStr("REDIS_STARTUP_COOKIE_POOL_KEY", "tiktok:startup_cookie_pool")), pollSec)
				time.Sleep(time.Duration(pollSec) * time.Second)
			}
		} else {
			devs, err = loadDevicesFromRedisN(need)
		}
		if err != nil {
			fmt.Printf("从Redis读取设备失败: %v\n", err)
			os.Exit(1)
		}
		config.Devices = append(config.Devices, devs...)
		fmt.Printf("已从Redis加载 %d 个设备（按需加载，目标=%d）\n", len(config.Devices), need)
	} else {
		// 兼容旧逻辑：读本地文件（如果不存在则自动生成）
		if data, err := ioutil.ReadFile(devicesPath); err == nil {
			scanner := bufio.NewScanner(strings.NewReader(string(data)))
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line != "" {
					// 可选：筛选“创建时间早于 N 小时”的设备
					if !devicePassMinAge(line, GetDeviceMinAgeHours()) {
						continue
					}
					config.Devices = append(config.Devices, line)
				}
			}
		} else {
			// 文件不存在，自动生成1000个设备
			if err := GenerateDevicesFile(devicesPath, 1000); err != nil {
				fmt.Printf("生成设备文件失败: %v\n", err)
				os.Exit(1)
			}
			// 重新加载
			if data, err := ioutil.ReadFile(devicesPath); err == nil {
				scanner := bufio.NewScanner(strings.NewReader(string(data)))
				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())
					if line != "" {
						if !devicePassMinAge(line, GetDeviceMinAgeHours()) {
							continue
						}
						config.Devices = append(config.Devices, line)
					}
				}
			} else {
				fmt.Printf("加载生成的设备文件失败: %v\n", err)
				os.Exit(1)
			}
		}
		if h := GetDeviceMinAgeHours(); h > 0 {
			fmt.Printf("已从文件加载 %d 个设备（create_time 早于 %d 小时）\n", len(config.Devices), h)
		} else {
			fmt.Printf("已从文件加载 %d 个设备\n", len(config.Devices))
		}
	}

	if len(config.Proxies) == 0 || len(config.Devices) == 0 {
		fmt.Println("代理或设备列表为空")
		os.Exit(1)
	}

	// 加载 cookies：必须来自 Go startUp 注册写入的 Redis cookie 池
	// 你要求：stats 的 cookies 和 设备必须来自同一个 JSON（账号池里每条完整账号 JSON 含 cookies 字段）。
	// 当设备来源是 startup_cookie_redis 时，强制从 config.Devices（即账号 JSON 列表）抽取 cookies，保证同源。
	if shouldLoadDevicesFromStartupCookieRedis() {
		limit := envInt("COOKIES_LIMIT", 0)
		cookies := loadCookiesFromStartupDevices(config.Devices, limit)
		if len(cookies) == 0 {
			fmt.Printf("从账号池设备列表提取 cookies 失败：请确认 startup_cookie_pool:data 存的是完整账号 JSON 且包含 cookies 字段\n")
			os.Exit(1)
		}
		globalCookiePool = cookies
		fmt.Printf("已从账号池设备列表抽取 %d 份 cookies（与设备同源）\n", len(globalCookiePool))
	} else if shouldLoadCookiesFromDevicesFile() {
		limit := envInt("COOKIES_LIMIT", 0)
		// 从“设备列表”抽取 cookies：config.Devices 每行 JSON（包含 cookies 字段）
		// - 如果 DEVICES_SOURCE=file，则来自设备文件（如 startUp 导出的 devices12_21_3.txt）
		// - 如果 DEVICES_SOURCE=redis，则来自 Redis 设备池（需确保每条设备 JSON 也包含 cookies 字段）
		cookies := loadCookiesFromStartupDevices(config.Devices, limit)
		if len(cookies) == 0 {
			fmt.Printf("从设备文件提取 cookies 失败：请确认设备文件每行包含 cookies 字段（如 {'k':'v'}），并检查 create_time 过滤是否过严\n")
			os.Exit(1)
		}
		globalCookiePool = cookies
		fmt.Printf("已从设备列表抽取 %d 份 cookies（来自每行 cookies 字段）\n", len(globalCookiePool))
	} else {
		fmt.Printf("cookies 未配置：请使用 DEVICES_SOURCE=startup_cookie_redis（推荐）或 COOKIES_SOURCE=devices_file，从 signup 产出的账号 JSON 里解析 cookies\n")
		os.Exit(1)
	}

	// Linux 抢单模式：从数据库抢未完成订单，按订单 aweme_id 执行播放，并实时写 Redis/回写数据库
	if shouldRunOrderMode() {
		runOrderMode()
		return
	}

	// Windows/非抢单模式：视频 ID 从配置文件读取
	if aweme := strings.TrimSpace(envStr("AWEME_ID", "")); aweme != "" {
		config.AwemeID = aweme
	}

	engine, err := NewEngine()
	if err != nil {
		log.Fatal(err)
	}
	engine.Run()
	// 总耗时已在Run()方法中打印
}

// applyRedisPoolShardFromArgs 支持按脚本参数切换 Redis 设备池/ cookies 池：
// - go run . 1 1  => 使用 tiktok:device_pool:1 与 tiktok:startup_cookie_pool:1
// - 不传参       => 默认 0 号（不加后缀，保持 tiktok:device_pool / tiktok:startup_cookie_pool）
//
// “库数量”可通过配置限制：
// - REDIS_DEVICE_POOL_SHARDS（默认 1）
// - REDIS_COOKIE_POOL_SHARDS（默认 1）
func applyRedisPoolShardFromArgs(args []string) {
	deviceIdx := 0
	cookieIdx := 0
	if len(args) >= 2 {
		if n, err := strconv.Atoi(strings.TrimSpace(args[1])); err == nil && n >= 0 {
			deviceIdx = n
		}
	}
	// 只传一个 idx：默认按“cookie 分库”理解（device 默认不分库）
	if len(args) < 3 {
		cookieIdx = deviceIdx
		deviceIdx = 0
	} else if len(args) >= 3 {
		if n, err := strconv.Atoi(strings.TrimSpace(args[2])); err == nil && n >= 0 {
			cookieIdx = n
		}
	}

	devShards := envInt("REDIS_DEVICE_POOL_SHARDS", 1)
	ckShards := envInt("REDIS_COOKIE_POOL_SHARDS", 1)
	if devShards <= 0 {
		devShards = 1
	}
	if ckShards <= 0 {
		ckShards = 1
	}
	// device 池默认不分库：如果没有配置 device shards（或=1），则忽略 deviceIdx
	if devShards <= 1 {
		deviceIdx = 0
	}
	if deviceIdx >= devShards {
		log.Printf("[pool] devicePoolIdx=%d 超出 REDIS_DEVICE_POOL_SHARDS=%d，自动回退为 0", deviceIdx, devShards)
		deviceIdx = 0
	}
	if cookieIdx >= ckShards {
		log.Printf("[pool] cookiePoolIdx=%d 超出 REDIS_COOKIE_POOL_SHARDS=%d，自动回退为 0", cookieIdx, ckShards)
		cookieIdx = 0
	}

	baseDev := envStr("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool")
	baseCk := envStr("REDIS_STARTUP_COOKIE_POOL_KEY", "tiktok:startup_cookie_pool")

	devKey := baseDev
	if deviceIdx > 0 {
		devKey = fmt.Sprintf("%s:%d", baseDev, deviceIdx)
	}
	ckKey := baseCk
	if cookieIdx > 0 {
		ckKey = fmt.Sprintf("%s:%d", baseCk, cookieIdx)
	}

	// 覆盖进程内 env：后续所有 Redis 读写都将使用该前缀
	_ = os.Setenv("REDIS_DEVICE_POOL_KEY", devKey)
	_ = os.Setenv("REDIS_STARTUP_COOKIE_POOL_KEY", ckKey)

	log.Printf("[pool] selected devicePoolIdx=%d/%d key=%s | cookiePoolIdx=%d/%d key=%s",
		deviceIdx, devShards, devKey, cookieIdx, ckShards, ckKey)
}

// findTopmostFileUpwards 从当前目录开始向上查找文件，返回“最顶层”的那个路径（更接近仓库根目录）。
func findTopmostFileUpwards(name string, maxUp int) string {
	start, err := os.Getwd()
	if err != nil || start == "" {
		return ""
	}
	start, _ = filepath.Abs(start)

	found := ""
	dir := start
	for i := 0; i <= maxUp; i++ {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			found = p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return found
}
