# 三个项目梳理 + 高并发利弊总结

本文档面向仓库里三条主链路：

- **Python 注册**：`mwzzzh_spider.py`（批量注册设备 → 写入 Redis 设备池 + 本地备份）
- **Go startUp（signup）**：`goPlay/demos/signup/dgemail`（批量注册账号/拿 cookies → 写入 Redis cookies 池；支持轮询补齐、分库）
- **Go stats（播放/抢单）**：`goPlay/demos/stats/dgmain3`（高并发播放请求；Redis 模式按需取设备/更换设备与 cookies；Linux 抢单模式）

---

## 1. 总体数据流（推荐：Redis 模式）

### 1.1 设备流（device pool）
- **生产**：`mwzzzh_spider.py` 注册成功得到 device JSON（包含 `seed/token` 等字段会被后续更新）
- **落库**：写入 Redis 的 device pool（支持分库：`REDIS_DEVICE_POOL_SHARDS`）
- **消费/更新**：`dgmain3` 播放时会：
  - 从 Redis 按需加载 N 个设备到内存（N≈并发数）
  - 运行中对设备做健康度判断与替换（连续失败/播放次数阈值）
  - 把设备的 `seed/token/play_count/attempt_count/fail_count/...` **回写 Redis**，做到“统一设备状态源”

### 1.2 cookies 流（startup cookie pool）
- **生产**：`dgemail` 注册成功得到 cookies（map 或字符串解析）
- **落库**：写入 Redis 的 startup cookie pool（支持分库：`REDIS_COOKIE_POOL_SHARDS`）
- **消费/更换**：`dgmain3` 播放时从 Redis 取 cookies；若 cookies 连续失败会自动更换；若池为空则回退到 `DEFAULT_COOKIES_JSON`

---

## 2. Redis Key 结构（强约定）

### 2.1 设备池（device pool）
以 `REDIS_DEVICE_POOL_KEY` 为前缀（例如 `tiktok:device_pool`）：

- **集合**：`{prefix}:ids`（SET）所有设备 id
- **数据**：`{prefix}:data`（HASH）`id -> device_json`
- **计数**：
  - `{prefix}:use`（ZSET）`id -> use_count`
  - `{prefix}:fail`（ZSET）`id -> fail_count`
  - `{prefix}:play`（ZSET）`id -> play_count`
  - `{prefix}:attempt`（ZSET）`id -> attempt_count`

### 2.2 cookies 池（startup cookie pool）
以 `REDIS_STARTUP_COOKIE_POOL_KEY` 为前缀（例如 `tiktok:startup_cookie_pool`）：

- `{prefix}:ids`（SET）cookie id（优先 `sessionid` / `sid_tt`，否则 hash）
- `{prefix}:data`（HASH）`id -> cookies_json`
- `{prefix}:use`（ZSET）`id -> use_count`

### 2.3 分库（shards）命名规则
- **0 号池**：原始前缀不加后缀：`tiktok:device_pool`
- **i 号池**：追加 `:{i}`：`tiktok:device_pool:1`

控制参数：
- **设备分库数**：`REDIS_DEVICE_POOL_SHARDS`
- **cookies 分库数**：`REDIS_COOKIE_POOL_SHARDS`

---

## 3. 项目 1：Python 注册（`mwzzzh_spider.py`）

### 3.1 并发模型
核心是“三段式并行”：

- **网络并发（async）**：`asyncio` + `asyncio.Semaphore(GEN_CONCURRENCY)`
- **CPU/解析（thread pool）**：`ThreadPoolExecutor(THREAD_POOL_SIZE)` 给 `run_registration_flow` 使用
- **落盘/入库（单写线程）**：`DataPipeline` 内部用一个 `ThreadPoolExecutor(max_workers=1)` 批量写入 `results*.jsonl`、备份文件、Redis

同时提供 **keep-alive 会话池**（减少握手/建连）：
- `MWZZZH_KEEPALIVE=1` 时启用 `SessionPool`
- `MWZZZH_SESSION_POOL_SIZE` 控制池大小（默认=并发数）
- `MWZZZH_SESSION_MAX_REQUESTS` 控制单 session 最大“任务数”，达到后淘汰重建

### 3.2 可靠性与一致性
- **Redis 硬失败**：当 `SAVE_TO_REDIS=1`，Redis 连接/写入失败会直接终止，避免“跑了但没入库”
- **文件备份**：
  - `SAVE_TO_FILE=1` 时写入 `DEVICE_BACKUP_DIR`（默认 `device_backups/`）
  - 分片文件数默认等于线程数（`DEVICE_FILE_SHARDS` 默认 `THREAD_POOL_SIZE`）
  - 每个任务用 `task_id % FILE_SHARDS` 写入“自己的文件”
  - **即使超过 `PER_FILE_MAX` 也继续写**（备份目的，避免丢数据）
  - `MWZZZH_FILE_FSYNC=1` 可提升异常退出时的落盘概率，但显著降低性能

### 3.3 轮询补齐（Linux 默认）
`MWZZZH_POLL_MODE=1` 时：
- 周期性检查每个 device shard 的数量，选择“未满且最少”的池补齐到 `REDIS_MAX_DEVICES`
- 单轮补齐上限：`MWZZZH_POLL_BATCH_MAX`

---

## 4. 项目 2：Go startUp（`goPlay/demos/signup/dgemail`）

### 4.1 两种运行方式
- **一次性模式**：读账号/设备/代理 → 并发注册 → 写结果 → 可选写 Redis cookies
- **轮询补齐模式**（Linux 默认开启）：持续检查 cookies 池缺口，自动注册补齐

### 4.2 并发模型（注册）
- 使用 `semaphore := make(chan struct{}, maxConcurrency)` 控制 goroutine 并发
- 每个账号 goroutine 内包含多段重试（seed/token/register），并对网络错误做指数退避
- 为避免“同一时刻同时发起”导致尖峰，任务启动前加入 0–200ms 随机延迟

### 4.3 轮询补齐（分库）
补齐模式会：
- 按 `REDIS_COOKIE_POOL_SHARDS` 扫描各 shard，选择“未满且最少”的 cookies 池写入
- 目标数量（每个池的最终数量）统一使用 **`REDIS_MAX_COOKIES`**

> 注意：一次性模式里并发数当前是代码固定值；轮询补齐模式会读取 `SIGNUP_CONCURRENCY`。

---

## 5. 项目 3：Go stats（`goPlay/demos/stats/dgmain3`）

### 5.1 并发模型（播放）
- 并发主开关：
  - `STATS_CONCURRENCY`（优先）
  - 否则回退 `GEN_CONCURRENCY`
- 工作模式：
  - 启动 `MaxConcurrency` 个 worker goroutine 反复取 task 执行
  - 使用 `inflight` 计数减少达到目标后的“超一轮并发”（提前停止）
  - 结果写入使用独立 writer goroutine + 批量 flush，减少磁盘阻塞

### 5.2 设备与 cookies 的“运行期替换”
仅在 Redis 模式有效：
- **设备替换**：连续失败（排除网络错误）达到阈值或播放次数达到阈值时，替换为 Redis 新设备
- **cookies 替换**：cookies 连续失败达到阈值时自动更换；池为空时回退 `DEFAULT_COOKIES_JSON`

### 5.3 Linux 抢单模式（高并发下的“吞吐优先”路径）
Linux 下可启用从 MySQL 抢单、Redis 实时记录进度、异常退出时回刷 DB 的流程（避免 `FOR UPDATE`，用乐观更新 + 轮询）。

---

## 6. 高并发的利与弊（结合本仓库的真实实现）

### 6.1 高并发的“利”
- **吞吐提升**：在代理充足、目标服务未限流、客户端资源足够时，请求/注册速率近似线性提升
- **掩盖尾延迟**：单次请求偶发慢/抖动时，多并发能把整体完成时间“拉平”
- **更快收敛**：更快发现某类错误（seed/token/stats/网络/解析）占比，便于调参
- **池子更健康**：`dgmain3` 的动态替换能在高并发下更快淘汰坏设备/坏 cookies，提高后续成功率

### 6.2 高并发的“弊”（最常见的坑）
- **成功率下降**：并发过高会触发目标侧风控/限流，表现为 stats 错误、验证码、封禁等
- **代理/IP 变热**：代理数 < 并发数时，同一代理承载过多请求，错误率与封禁概率显著上升
- **资源瓶颈转移**：
  - CPU：Python 解析线程池不足会导致 event loop 堵塞（表面看并发高，实际吞吐不涨）
  - 内存：过大队列/批量缓冲会吃内存（Go 的 writer 队列、Python 的 pipeline queue）
  - 磁盘：过高写入频率会拖慢主流程；`fsync` 会把吞吐“打穿”
  - Redis：高频 `HSET/ZINCRBY` 会形成热点 key，影响全局延迟
- **放大重试风暴**：高并发 + 重试（指数退避但仍然重试）会造成“雪崩式”流量峰值
- **更难定位问题**：日志量暴涨，错误更随机化；需要聚合统计而不是逐条日志
- **目标达成“超一轮”**：并发任务已启动但尚未完成时达到目标，容易多跑一轮（本仓库已用 `inflight` 缓解）

---

## 7. 调参建议（可直接照做）

### 7.1 并发数怎么设（经验法）
- **先确定代理承载能力**：有效代理数为 P，建议并发从 `P * 0.5 ~ P * 1.0` 逐步上调
- **逐步上调**：每次 +100 或 +200，看 2–5 分钟窗口的成功率与网络错误占比
- **指标驱动**：
  - network 错误高：并发过高/代理质量差/本机出口限速
  - stats 错误高：目标限流/风控更强，降低并发或提升代理分散度

### 7.2 Python 注册侧（`mwzzzh_spider.py`）
- `GEN_CONCURRENCY` 控制网络并发（主旋钮）
- `GEN_THREAD_POOL_SIZE` 控制 CPU 解析线程（解析慢就加，但别无限加）
- `MWZZZH_SESSION_POOL_SIZE` 建议 ≈ 并发数（连接复用更稳）
- `MWZZZH_SESSION_MAX_REQUESTS` 建议 100–500（过大可能被识别；过小握手开销大）
- 若担心异常退出丢备份：开 `MWZZZH_FILE_FSYNC=1`，但要接受吞吐明显下降

### 7.3 Go stats（`dgmain3`）
- `STATS_CONCURRENCY` 是主并发旋钮
- `STATS_DEVICE_FAIL_THRESHOLD`/`STATS_DEVICE_PLAY_MAX` 决定“什么时候换设备”
- `DEFAULT_COOKIES_JSON` 必须准备一份兜底（避免 cookies 池空导致全失败）
- 分库运行（多进程扩展）：`go run . <deviceShardIdx> <cookieShardIdx>`

---

## 8. 已知限制/可改进点（如需要我可以继续做）
- `dgmain3` 的动态并发 `adjustConcurrency()` 已按“方式A”实现：保留固定 worker 数量，但每次发请求前会按 `currentConcurrency` 做令牌门控（动态并发真正生效），同时仍受 `semaphore` 硬上限保护
- `dgemail` 一次性模式与轮询补齐模式已统一：两种模式都以 `SIGNUP_CONCURRENCY` 为准（并保留合理默认值）
- `dgmain3` 结果/错误写入已做成**并行写 + 非阻塞投递**（队列满会丢弃并计数），确保写入不会干预主任务；如果你需要“绝不丢日志/结果”，只能在“内存占用/主流程阻塞风险”之间做取舍（例如落盘队列改成无界但要做内存保护）


