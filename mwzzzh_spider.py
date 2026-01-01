import asyncio
import logging
import json
import os
import random
import traceback
import time
import platform
import signal
from itertools import cycle
from concurrent.futures import ThreadPoolExecutor
from typing import Dict, Any
from dataclasses import dataclass
from pathlib import Path

# 你的核心请求库
from curl_cffi.requests import AsyncSession

from device_register.dgmain2.register_logic import run_registration_flow


def _load_env_for_runtime() -> str | None:
    """
    为 mwzzzh_spider 载入环境配置：
    - Windows: .env.windows / env.windows
    - Linux:   .env.linux   / env.linux
    """
    # 1) 显式指定（与 Go demos 对齐）：ENV_FILE=/path/to/env.linux
    explicit = (os.getenv("ENV_FILE") or "").strip()
    if explicit and os.path.exists(explicit):
        try:
            from dotenv import load_dotenv  # type: ignore
            load_dotenv(explicit, override=True)
        except Exception:
            pass
        return explicit

    sysname = platform.system().lower()
    if "windows" in sysname:
        candidates = [".env.windows", "env.windows"]
    else:
        candidates = [".env.linux", "env.linux"]

    # 2) 优先在当前工作目录找
    for p in candidates:
        if os.path.exists(p):
            env_path = p
            break
    else:
        env_path = None

    # 3) 再在脚本所在目录找（避免 “从 ~ 执行脚本” 时找不到 env.linux）
    if not env_path:
        try:
            base = Path(__file__).resolve().parent
            for p in candidates:
                cand = base / p
                if cand.exists():
                    env_path = str(cand)
                    break
        except Exception:
            env_path = None

    if not env_path:
        return None

    try:
        from dotenv import load_dotenv  # type: ignore
    except Exception:
        return env_path

    # 以文件为准，避免系统环境变量残留导致配置不生效
    load_dotenv(env_path, override=True)
    return env_path


_MWZZZH_ENV_FILE = _load_env_for_runtime()

def _parse_bool(v: str | None, default: bool) -> bool:
    if v is None:
        return default
    v = v.strip().lower()
    if v in {"1", "true", "yes", "y", "on"}:
        return True
    if v in {"0", "false", "no", "n", "off"}:
        return False
    return default

def _normalize_pool_base(prefix: str) -> str:
    """
    统一设备池前缀为“不分库”版本：
    - 若传入形如 "tiktok:device_pool:3"，则回退为 "tiktok:device_pool"
    - 其它保持不变
    """
    prefix = (prefix or "").strip()
    if not prefix:
        return prefix
    parts = prefix.split(":")
    if len(parts) >= 2 and parts[-1].isdigit():
        base = ":".join(parts[:-1]).strip()
        return base or prefix
    return prefix

def _get_int_from_env(*names: str, default: int) -> int:
    """
    按优先级读取多个环境变量中的第一个“可解析为 int”的值。
    例如：优先 MWZZZH_TASKS，其次 MAX_GENERATE。
    """
    for name in names:
        v = os.getenv(name)
        if v is None:
            continue
        v = v.strip()
        if not v:
            continue
        try:
            return int(v)
        except Exception:
            continue
    return default


def _clamp(n: int, lo: int, hi: int) -> int:
    return max(lo, min(hi, n))


def _auto_thread_pool_size() -> int:
    """
    解析线程池默认按机器 CPU 自动计算（不依赖 GEN_CONCURRENCY）。
    经验值：CPU 核心数 * 2，并限制在 [4, 64]
    """
    cores = os.cpu_count() or 4
    return _clamp(int(cores) * 2, 4, 64)


@dataclass(frozen=True)
class RedisConfig:
    url: str | None
    host: str
    port: int
    db: int
    username: str | None
    password: str | None
    ssl: bool
    key_prefix: str
    id_field: str
    max_size: int
    evict_policy: str  # play/use/attempt


def _get_redis_config(max_devices_default: int) -> RedisConfig:
    url = os.getenv("REDIS_URL") or None
    host = os.getenv("REDIS_HOST", "127.0.0.1")
    port = int(os.getenv("REDIS_PORT", "6379"))
    db = int(os.getenv("REDIS_DB", "0"))
    username = os.getenv("REDIS_USERNAME") or None
    password = os.getenv("REDIS_PASSWORD") or None
    ssl = _parse_bool(os.getenv("REDIS_SSL"), False)
    # Python 设备池：按你的要求“固定不分库”
    # 即便环境变量误配为 "tiktok:device_pool:<idx>"，这里也会自动去掉后缀
    key_prefix = _normalize_pool_base(os.getenv("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool"))
    id_field = os.getenv("REDIS_DEVICE_ID_FIELD", "cdid")
    max_size = int(os.getenv("REDIS_MAX_DEVICES", str(max_devices_default)))
    evict_policy = (os.getenv("REDIS_EVICT_POLICY", "play") or "play").strip().lower()
    return RedisConfig(
        url=url,
        host=host,
        port=port,
        db=db,
        username=username,
        password=password,
        ssl=ssl,
        key_prefix=key_prefix,
        id_field=id_field,
        max_size=max_size,
        evict_policy=evict_policy,
    )


class RedisDevicePool:
    """
    Redis 设备池：注册成功设备写入这里，供后续 Go/其它模块消费

    Key 结构（全部基于 key_prefix）：
    - {prefix}:ids   (SET)  所有设备 id
    - {prefix}:data  (HASH) id -> device_json
    - {prefix}:use   (ZSET) id -> use_count
    - {prefix}:fail  (ZSET) id -> fail_count
    - {prefix}:seq   (STRING/INT) 自增序号（严格 FIFO 用）
    - {prefix}:in    (ZSET) id -> seq（入队顺序，越小越早入队）

    超限淘汰：use_count 最大的先淘汰（最常用的淘汰）
    """

    def __init__(self, cfg: RedisConfig):
        self.cfg = cfg
        self.ids_key = f"{cfg.key_prefix}:ids"
        self.data_key = f"{cfg.key_prefix}:data"
        self.use_key = f"{cfg.key_prefix}:use"
        self.fail_key = f"{cfg.key_prefix}:fail"
        # 计数：Go stats 会写 play/attempt；Python 写入时也初始化为 0，方便后续淘汰
        self.play_key = f"{cfg.key_prefix}:play"
        self.attempt_key = f"{cfg.key_prefix}:attempt"
        # FIFO 队列：严格按入队顺序优先取（Go stats 读取时用）
        self.seq_key = f"{cfg.key_prefix}:seq"
        self.in_key = f"{cfg.key_prefix}:in"

        try:
            import redis  # type: ignore
        except Exception as e:
            raise RuntimeError("缺少依赖：redis，请先 pip install -r requirements.txt") from e

        if cfg.url:
            self.r = redis.Redis.from_url(cfg.url, decode_responses=True)
        else:
            self.r = redis.Redis(
                host=cfg.host,
                port=cfg.port,
                db=cfg.db,
                username=cfg.username,
                password=cfg.password,
                ssl=cfg.ssl,
                decode_responses=True,
            )

    def ping(self) -> None:
        self.r.ping()

    def count(self) -> int:
        return int(self.r.scard(self.ids_key))

    def _extract_id(self, device: Dict[str, Any]) -> str:
        raw = device.get(self.cfg.id_field)
        if isinstance(raw, str) and raw.strip():
            return raw.strip()
        # fallback：尽量找稳定字段
        for k in ("cdid", "clientudid", "openudid", "device_id", "install_id"):
            v = device.get(k)
            if isinstance(v, str) and v.strip():
                return v.strip()
        # 最后兜底
        return f"anon:{time.time_ns()}"

    def add_devices(self, devices: list[Dict[str, Any]]) -> tuple[int, int]:
        """
        批量写入：
        - 新设备：初始化 use=0, fail=0
        - 旧设备：更新 data，不重置计数
        返回：(写入数量, 淘汰数量)
        """
        # 可观测性：写入前后池子数量变化（用于排查“注册成功但缺口不变”）
        try:
            before = int(self.r.scard(self.ids_key))
        except Exception:
            before = -1

        write_n = 0
        for dev in devices:
            dev_id = self._extract_id(dev)
            dev_json = json.dumps(dev, ensure_ascii=False, separators=(",", ":"))

            # existed?
            existed = bool(self.r.sismember(self.ids_key, dev_id))
            pipe = self.r.pipeline(transaction=True)
            pipe.sadd(self.ids_key, dev_id)
            pipe.hset(self.data_key, dev_id, dev_json)
            if not existed:
                # 严格 FIFO：用自增 seq 作为 score（避免时间戳同秒导致顺序不稳定）
                try:
                    seq = int(self.r.incr(self.seq_key))
                except Exception:
                    # 回退：毫秒时间戳（非严格但可用）
                    seq = int(time.time() * 1000)
                pipe.zadd(self.in_key, {dev_id: seq})
                pipe.zadd(self.use_key, {dev_id: 0})
                pipe.zadd(self.fail_key, {dev_id: 0})
                pipe.zadd(self.play_key, {dev_id: 0})
                pipe.zadd(self.attempt_key, {dev_id: 0})
            pipe.execute()
            write_n += 1

        evicted = self.evict_if_needed()
        try:
            after = int(self.r.scard(self.ids_key))
        except Exception:
            after = -1

        if before >= 0 and after >= 0:
            delta = after - before
            # delta==0 常见原因：全部重复 id（SET 去重）或写入后被并发消费/淘汰抵消
            logger.info(
                f"[redis] batch_done key={self.cfg.key_prefix} id_field={self.cfg.id_field} "
                f"batch={len(devices)} wrote={write_n} before={before} after={after} delta={delta} evicted={evicted}"
            )
        return write_n, evicted

    def evict_if_needed(self) -> int:
        max_size = max(0, int(self.cfg.max_size))
        if max_size <= 0:
            return 0
        cur = int(self.r.scard(self.ids_key))
        if cur <= max_size:
            return 0
        excess = cur - max_size
        # 淘汰策略：优先按播放次数(play)最大淘汰；如果 play 为空则回退 use（历史兼容）。
        policy = (self.cfg.evict_policy or "play").lower()
        if policy not in {"play", "use", "attempt"}:
            policy = "play"
        zkey = self.play_key if policy == "play" else (self.attempt_key if policy == "attempt" else self.use_key)
        # 若选择 play/attempt 但没有任何成员，则回退到 use
        try:
            if zkey != self.use_key and int(self.r.zcard(zkey)) == 0:
                zkey = self.use_key
        except Exception:
            zkey = self.use_key

        ids = self.r.zrevrange(zkey, 0, excess - 1)
        if not ids:
            return 0
        pipe = self.r.pipeline(transaction=True)
        for dev_id in ids:
            pipe.srem(self.ids_key, dev_id)
            pipe.hdel(self.data_key, dev_id)
            pipe.zrem(self.in_key, dev_id)
            pipe.zrem(self.use_key, dev_id)
            pipe.zrem(self.fail_key, dev_id)
            pipe.zrem(self.play_key, dev_id)
            pipe.zrem(self.attempt_key, dev_id)
        pipe.execute()
        return len(ids)


# ================= 1. 配置区域 =================
class Config:
    # 网络并发数 (Async Semaphores)
    # 统一并发主开关：GEN_CONCURRENCY
    MAX_CONCURRENCY = _get_int_from_env("GEN_CONCURRENCY", default=200)

    # 线程池大小 (建议设置为 CPU 核心数 * 1 到 2 倍)
    # 如果你的解析逻辑特别重，可以开大一点，比如 16 或 32
    # 默认自动按 CPU 推导（可用 GEN_THREAD_POOL_SIZE 显式覆盖）
    THREAD_POOL_SIZE = _get_int_from_env(
        "GEN_THREAD_POOL_SIZE",
        "THREAD_POOL_SIZE",
        default=_auto_thread_pool_size(),
    )

    PROXIES = [
    ]

    RESULT_FILE = "results12_21_5.jsonl"
    ERROR_FILE = "error.log"

    # 是否保存“注册成功设备”到 Redis
    SAVE_TO_REDIS = _parse_bool(os.getenv("SAVE_TO_REDIS"), False)

    # 是否把“注册成功设备”写入本地备份文件（10 个文件，均分；追加写入）
    # 复用你已有的 SAVE_TO_FILE / PER_FILE_MAX 配置（与 generate_devices_bulk.py 一致）
    SAVE_TO_FILE = _parse_bool(os.getenv("SAVE_TO_FILE"), False)
    FILE_BACKUP_DIR = os.getenv("DEVICE_BACKUP_DIR", "device_backups")
    FILE_PREFIX = os.getenv("DEVICE_FILE_PREFIX", "devices")
    PER_FILE_MAX = _get_int_from_env("PER_FILE_MAX", default=10000)

    # 本地备份分片数：默认等于“线程数”（THREAD_POOL_SIZE）
    # 你的诉求：根据线程数写入，每个线程写自己的 => 用 task_id % FILE_SHARDS 分流到不同文件
    FILE_SHARDS = _get_int_from_env("DEVICE_FILE_SHARDS", default=THREAD_POOL_SIZE)
    # 文件刷盘策略：
    # - 默认只 flush（性能更好）
    # - 打开 MWZZZH_FILE_FSYNC=1 可在每批写入后执行 os.fsync，提升“异常退出时不丢数据”的概率（更慢）
    FILE_FSYNC = _parse_bool(os.getenv("MWZZZH_FILE_FSYNC"), False)
    # 任务数量：
    # - 优先 MWZZZH_TASKS
    # - 若未配置，则复用 MAX_GENERATE（与你的设备生成配置保持一致）
    # - 再否则默认 1000
    TASKS = _get_int_from_env("MWZZZH_TASKS", "MAX_GENERATE", default=1000)

    # Linux 轮询补齐模式（设备池补齐）：
    # - Linux 默认开启；Windows 默认关闭（可用 MWZZZH_POLL_MODE=1 强制开启调试）
    # - 轮询时，会检查 Redis 设备池当前数量，若少于目标数量，则自动补齐缺口
    POLL_MODE = _parse_bool(
        os.getenv("MWZZZH_POLL_MODE"),
        default=("linux" in platform.system().lower()),
    )
    # 轮询间隔（秒）
    POLL_INTERVAL_SEC = _get_int_from_env("MWZZZH_POLL_INTERVAL_SEC", default=10)
    # 设备池最终目标数量：统一使用 REDIS_MAX_DEVICES（作为“池子最终数量/容量”）
    # - REDIS_TARGET_DEVICES 已废弃（若配置了会被忽略），避免与 REDIS_MAX_DEVICES 冲突
    REDIS_MAX_DEVICES = _get_int_from_env("REDIS_MAX_DEVICES", default=TASKS)
    # 单轮补齐最大注册数量（避免一次补太多）
    POLL_BATCH_MAX = _get_int_from_env("MWZZZH_POLL_BATCH_MAX", default=TASKS)
    # Redis 设备池分库数量（用于平均分配/多实例并行消费）
    # - 1：不分库（所有设备写入同一个 tiktok:device_pool）
    # - N：分库（0号池为 base；i号池为 base:{i}）
    DEVICE_POOL_SHARDS = _get_int_from_env("REDIS_DEVICE_POOL_SHARDS", default=1)

    # keep-alive / Session 复用（连接复用 + 达到最大请求数自动淘汰重建）
    # - 默认开启（提升性能，减少握手/建连）
    # - MWZZZH_SESSION_MAX_REQUESTS：每个 Session 最多处理多少次“任务”（达到后淘汰重建）
    KEEPALIVE = _parse_bool(os.getenv("MWZZZH_KEEPALIVE"), True)
    SESSION_POOL_SIZE = _get_int_from_env("MWZZZH_SESSION_POOL_SIZE", default=MAX_CONCURRENCY)
    SESSION_MAX_REQUESTS = _get_int_from_env("MWZZZH_SESSION_MAX_REQUESTS", default=200)
    IMPERSONATE = os.getenv("MWZZZH_IMPERSONATE", "chrome131_android")


# ================= 2. 日志系统 =================
logger = logging.getLogger("mwzzzh_spider")
logger.setLevel(logging.INFO)
formatter = logging.Formatter('%(asctime)s - [%(threadName)s] - %(message)s')

ch = logging.StreamHandler()
ch.setFormatter(formatter)
logger.addHandler(ch)

fh = logging.FileHandler(Config.ERROR_FILE, encoding='utf-8')
fh.setLevel(logging.ERROR)
fh.setFormatter(formatter)
logger.addHandler(fh)

# 启动可观测性：告诉你 env 是否加载成功、是否会写 Redis
try:
    logger.info(
        "[env] loaded=%s | SAVE_TO_REDIS=%s | REDIS_DEVICE_POOL_KEY=%s | REDIS_DB=%s",
        _MWZZZH_ENV_FILE,
        os.getenv("SAVE_TO_REDIS"),
        os.getenv("REDIS_DEVICE_POOL_KEY"),
        os.getenv("REDIS_DB"),
    )
except Exception:
    pass


# ================= 3. 数据管道 (保持不变) =================
class DataPipeline:
    def __init__(self, filename, redis_pool: RedisDevicePool | None = None, save_to_file: bool = False):
        self.filename = filename
        self.queue = asyncio.Queue()
        self.executor = ThreadPoolExecutor(max_workers=1)
        self.running = True
        self._writer_task = None
        self.redis_pool = redis_pool
        self.save_to_file = save_to_file

        # 本地备份：按“线程数/分片数”写多个文件（每个分片一个文件）
        self._file_fps = None
        self._file_counts = None
        self._file_paths = None

    def _init_file_backup(self) -> None:
        if self._file_fps is not None:
            return

        out_dir = Path(Config.FILE_BACKUP_DIR)
        out_dir.mkdir(parents=True, exist_ok=True)

        file_count = max(1, int(Config.FILE_SHARDS))
        paths = [out_dir / f"{Config.FILE_PREFIX}_{i}.txt" for i in range(file_count)]

        # 记录每个文件当前行数（仅用于可观测性；不会因为 PER_FILE_MAX 满了就停止写入）
        counts = [0] * file_count
        if Config.PER_FILE_MAX and Config.PER_FILE_MAX > 0:
            for i, p in enumerate(paths):
                if p.exists():
                    try:
                        with p.open("r", encoding="utf-8", errors="ignore") as rf:
                            counts[i] = sum(1 for _ in rf)
                    except Exception:
                        counts[i] = 0

        fps = [p.open("a", encoding="utf-8") for p in paths]
        self._file_paths = paths
        self._file_counts = counts
        self._file_fps = fps
        logger.info(
            f"[file] 启用成功 out_dir={out_dir} prefix={Config.FILE_PREFIX} "
            f"per_file_max={Config.PER_FILE_MAX} (files={file_count}, shard=task_id%{file_count}, 满了也会继续追加写入)"
        )

    def _write_devices_to_backup_files(self, batch: list[tuple[int | None, Dict[str, Any]]]) -> None:
        if not self.save_to_file:
            return

        self._init_file_backup()
        assert self._file_fps is not None and self._file_counts is not None

        file_count = len(self._file_fps)

        for shard_key, dev in batch:
            # 每个“线程槽位/worker”写自己的文件：idx = shard_key % file_count
            # shard_key 取 task_id（由上层传入）
            if shard_key is None:
                fidx = 0
            else:
                fidx = int(shard_key) % file_count

            line = json.dumps(dev, ensure_ascii=False, separators=(",", ":"))
            self._file_fps[fidx].write(line + "\n")
            self._file_counts[fidx] += 1

        # 尽量及时刷盘（备份用途）
        for fp in self._file_fps:
            fp.flush()
            if Config.FILE_FSYNC:
                try:
                    os.fsync(fp.fileno())
                except Exception:
                    pass

    async def start(self):
        self._writer_task = asyncio.create_task(self._consumer())

    async def save(self, data: Dict, shard_key: int | None = None):
        # shard_key：用于本地备份分片（建议传 task_id）
        await self.queue.put((shard_key, data))

    def _write_impl(self, batch):
        try:
            with open(self.filename, 'a', encoding='utf-8') as f:
                for _, item in batch:
                    line = json.dumps(item, ensure_ascii=False)
                    f.write(line + "\n")
        except Exception as e:
            logger.error(f"写入失败: {e}")
            # results 文件都写不进去时，直接抛给上层（让线程退出）
            raise

        # 同步写入本地备份文件（在 executor 线程中执行）
        try:
            self._write_devices_to_backup_files(batch)
        except Exception as e:
            logger.critical(f"[file] 致命：写入本地备份失败: {e}")
            # 备份打开时：视为致命，避免“跑了但没落地”
            raise

        # 同步写入 Redis（在 executor 线程中执行，不阻塞 event loop）
        if self.redis_pool is not None:
            # Redis 打开时：任何写入失败都应该终止程序（避免“跑了但没入库”）
            only_devices = [d for _, d in batch]
            _, evicted = self.redis_pool.add_devices(only_devices)
            if evicted:
                # 不刷屏，只在发生淘汰时提示一次
                logger.info(f"[redis] 设备池已满，已淘汰 {evicted} 条（按 use_count 最大）")

    async def _consumer(self):
        batch: list[tuple[int | None, Dict[str, Any]]] = []
        while self.running or not self.queue.empty():
            try:
                item = await asyncio.wait_for(self.queue.get(), timeout=1.0)
                batch.append(item)
                self.queue.task_done()
            except asyncio.TimeoutError:
                pass
            if len(batch) >= 20 or (not self.running and batch):
                to_write = batch[:]
                batch.clear()
                try:
                    await asyncio.get_event_loop().run_in_executor(
                        self.executor, self._write_impl, to_write
                    )
                except Exception as e:
                    # 如果开启了 Redis，这里视为致命错误：立即退出
                    if self.redis_pool is not None or self.save_to_file:
                        logger.critical(f"[pipeline] 致命：写入失败，程序终止: {e}")
                        os._exit(1)
                    # 未开启 Redis：保留原行为，只记录错误继续
                    logger.error(f"写入线程异常: {e}")

    async def stop(self):
        self.running = False
        await self.queue.join()
        await self._writer_task
        self.executor.shutdown()
        # 关闭本地备份文件句柄
        if self._file_fps is not None:
            try:
                for fp in self._file_fps:
                    fp.close()
            except Exception:
                pass


# ================= 4. 核心：同步解析逻辑 (在线程中运行) =================
'''
def sync_parsing_logic(type,resp,device, *args):
    """
    【注意】这是一个普通的 def，不是 async def。
    这里放所有 CPU 密集型操作：
    1. 正则 / XPath / PyQuery 解析
    2. 复杂的解密算法 (AES/RSA...)
    3. 数据清洗
    """
    # 模拟一个耗时的解析过程
    # 如果这段代码在 async def 里直接跑，会卡死整个爬虫
    # 但现在它在线程池里跑，所以不会影响网络请求
    # time.sleep(0.1) # 模拟 CPU 计算耗时
    # 假设解析出了结果
    # parsed_data = {
    #     "id": task_id,
    #     "title": f"Title found in length {len(html_content)}",
    #     "extra_calc": sum(i for i in range(10000))  # 模拟计算
    # }
    return parse_logic(type,resp,device, *args)

这里就直接注释掉了，因为现在我们把解析逻辑直接放倒了主类里面去
'''


# ================= 5. 核心：异步业务流程 (在主循环运行) =================
async def user_custom_logic(session: AsyncSession, task_id, task_params, proxy, pipeline, thread_pool):
    """
    这里负责指挥：
    1. 遇到 IO (网络) -> await curl_cffi
    2. 遇到 CPU (计算) -> run_in_executor (扔给线程)
    """
    try:
        # 直接把 thread_pool 传给业务层
        device1 = await run_registration_flow(session, proxy, thread_pool, task_id)
        if type(device1)==dict:
            # 传 task_id 用于“按线程数分片写文件”：idx = task_id % FILE_SHARDS
            await pipeline.save(device1, shard_key=task_id)
            logger.info(f"[{task_id}] 注册成功")
        else:
            logger.warning(f"[{task_id}] 注册失败 (返回 None),{device1}")

    except Exception as e:
        logger.error(f"[{task_id}] 致命报错: {e}")
        logger.error(traceback.format_exc())


class _SessionHolder:
    def __init__(self, idx: int):
        self.idx = idx
        self.session: AsyncSession | None = None
        self.used_tasks = 0

    async def ensure(self) -> AsyncSession:
        if self.session is None:
            self.session = AsyncSession(impersonate=Config.IMPERSONATE)
            self.used_tasks = 0
        return self.session

    async def recycle(self) -> None:
        if self.session is not None:
            try:
                await self.session.close()
            except Exception:
                pass
        self.session = None
        self.used_tasks = 0


class SessionPool:
    """
    keep-alive Session 池：
    - 每个 worker 从池里借一个 session（独占），用完归还
    - 每个 session 使用次数达到 SESSION_MAX_REQUESTS 后自动淘汰重建
    """
    def __init__(self, size: int):
        self.size = max(1, int(size))
        self.q: asyncio.Queue[_SessionHolder] = asyncio.Queue()
        for i in range(self.size):
            self.q.put_nowait(_SessionHolder(i))

    async def acquire(self) -> _SessionHolder:
        return await self.q.get()

    async def release(self, h: _SessionHolder) -> None:
        # 达到最大次数：淘汰并重建（下次 ensure 会新建）
        if Config.SESSION_MAX_REQUESTS > 0 and h.used_tasks >= Config.SESSION_MAX_REQUESTS:
            logger.info(f"[keepalive] recycle session idx={h.idx} used_tasks={h.used_tasks} max={Config.SESSION_MAX_REQUESTS}")
            await h.recycle()
        self.q.put_nowait(h)

    async def close(self) -> None:
        # 尽量关闭所有 session（把队列里的都拿出来关）
        items: list[_SessionHolder] = []
        while not self.q.empty():
            try:
                items.append(self.q.get_nowait())
            except Exception:
                break
        for h in items:
            await h.recycle()
        # 放回去（避免后续误用）
        for h in items:
            self.q.put_nowait(h)


# ================= 6. 引擎 =================
class SpiderEngine:
    def __init__(self):
        self.proxy_cycle = cycle(Config.PROXIES)

        redis_pool = None
        if Config.SAVE_TO_REDIS:
            try:
                rcfg = _get_redis_config(max_devices_default=Config.TASKS)
                redis_pool = RedisDevicePool(rcfg)
                redis_pool.ping()
                logger.info(f"[redis] 启用成功 key_prefix={rcfg.key_prefix} id_field={rcfg.id_field} max={rcfg.max_size}")
            except Exception as e:
                # Redis 打开时：连接失败直接终止程序
                logger.critical(f"[redis] 启用失败，程序终止: {e}")
                raise SystemExit(1)

        self.pipeline = DataPipeline(Config.RESULT_FILE, redis_pool=redis_pool, save_to_file=Config.SAVE_TO_FILE)
        self.sem = asyncio.Semaphore(Config.MAX_CONCURRENCY)

        # 【新增】计算型线程池
        # max_workers 决定了同一时刻最多有多少个解析任务在跑
        self.cpu_pool = ThreadPoolExecutor(max_workers=Config.THREAD_POOL_SIZE, thread_name_prefix="CpuWorker")
        self.session_pool: SessionPool | None = None
        if Config.KEEPALIVE:
            self.session_pool = SessionPool(size=max(1, int(Config.SESSION_POOL_SIZE)))
            logger.info(f"[keepalive] enabled pool_size={self.session_pool.size} max_requests={Config.SESSION_MAX_REQUESTS} impersonate={Config.IMPERSONATE}")

    def get_proxy(self):
        return next(self.proxy_cycle)

    async def _worker_wrapper(self, task_id, task_params):
        async with self.sem:
            try:
                proxy = self.get_proxy()
                if self.session_pool is None:
                    async with AsyncSession(impersonate=Config.IMPERSONATE) as session:
                        await user_custom_logic(session, task_id, task_params, proxy, self.pipeline, self.cpu_pool)
                    return

                holder = await self.session_pool.acquire()
                try:
                    session = await holder.ensure()
                    holder.used_tasks += 1
                    await user_custom_logic(session, task_id, task_params, proxy, self.pipeline, self.cpu_pool)
                finally:
                    await self.session_pool.release(holder)
            except Exception as e:
                logger.error(f"Wrapper error: {e}")

    async def run(self, tasks_n: int | None = None):
        await self.pipeline.start()
        coroutines: list[asyncio.Task] = []
        try:
            # 生成任务
            n = int(tasks_n) if tasks_n is not None else int(Config.TASKS)
            n = max(0, n)
            tasks_data = [{"id": i} for i in range(n)]
            logger.info(
                f"开始任务，任务数: {n}, 网络并发: {Config.MAX_CONCURRENCY}, 解析线程: {Config.THREAD_POOL_SIZE} "
                f"(file_backup={Config.SAVE_TO_FILE} dir={Config.FILE_BACKUP_DIR} shards={Config.FILE_SHARDS} fsync={Config.FILE_FSYNC})"
            )

            for i, params in enumerate(tasks_data):
                coroutines.append(asyncio.create_task(self._worker_wrapper(i, params)))

            # 等待完成（可被取消）
            try:
                from tqdm.asyncio import tqdm
                _ = [await f for f in tqdm.as_completed(coroutines)]
            except ImportError:
                await asyncio.gather(*coroutines)

        except asyncio.CancelledError:
            logger.warning("收到取消信号，准备优雅退出（先落盘队列）...")
            raise
        finally:
            # 先取消 worker，避免继续往 pipeline 塞数据
            for t in coroutines:
                if not t.done():
                    t.cancel()
            if coroutines:
                _ = await asyncio.gather(*coroutines, return_exceptions=True)

            logger.info("任务结束，清理中（将等待写入队列落盘）...")
            try:
                await self.pipeline.stop()
            except Exception as e:
                logger.critical(f"[pipeline] stop 失败: {e}")
                # stop 失败时不硬退出，让上层看日志

            # 关闭线程池 / session
            try:
                self.cpu_pool.shutdown()
            except Exception:
                pass
            if self.session_pool is not None:
                try:
                    await self.session_pool.close()
                except Exception:
                    pass


async def _poll_fill_loop() -> None:
    """
    轮询补齐模式：
    - 仅在 SAVE_TO_REDIS=1 时生效
    - 每隔 POLL_INTERVAL_SEC 秒检查 Redis 设备池数量，若不足目标则补齐
    """
    if not Config.SAVE_TO_REDIS:
        logger.critical("[poll] MWZZZH_POLL_MODE 打开时必须同时打开 SAVE_TO_REDIS=1（需要检查 Redis 设备池数量）")
        raise SystemExit(1)

    interval = max(1, int(Config.POLL_INTERVAL_SEC))
    # 目标数量：统一使用 REDIS_MAX_DEVICES
    target = max(0, int(Config.REDIS_MAX_DEVICES))
    batch_max = max(1, int(Config.POLL_BATCH_MAX))
    shards = max(1, int(Config.DEVICE_POOL_SHARDS))

    if target <= 0:
        logger.critical("[poll] REDIS_MAX_DEVICES 需要 > 0")
        raise SystemExit(1)

    # REDIS_TARGET_DEVICES 已废弃：若用户仍配置，提示但忽略
    legacy_target = os.getenv("REDIS_TARGET_DEVICES")
    if legacy_target is not None and legacy_target.strip():
        logger.warning("[poll] 检测到已废弃配置 REDIS_TARGET_DEVICES，将忽略并以 REDIS_MAX_DEVICES 为准")

    base_prefix = _normalize_pool_base(os.getenv("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool").strip() or "tiktok:device_pool")
    logger.info(f"[poll] 启动：interval={interval}s target={target} batch_max={batch_max} shards={shards} base_prefix={base_prefix}")

    while True:
        try:
            # 连接 Redis，读取每个池大小（失败视为致命）
            # 规则：0号池为 base_prefix，其它池为 base_prefix:{idx}
            pool_counts: list[tuple[int, int, str]] = []
            for i in range(shards):
                key_prefix = base_prefix if i == 0 else f"{base_prefix}:{i}"
                os.environ["REDIS_DEVICE_POOL_KEY"] = key_prefix
                rcfg = _get_redis_config(max_devices_default=target)
                pool = RedisDevicePool(rcfg)
                pool.ping()
                cur = pool.count()
                pool_counts.append((cur, i, key_prefix))
        except Exception as e:
            logger.critical(f"[poll] Redis 连接/读取失败，程序终止: {e}")
            raise SystemExit(1)

        # 选择一个“未满”的池子进行补齐：优先选择当前数量最少的池（达到平均分配）
        pool_counts.sort(key=lambda x: x[0])
        chosen: tuple[int, int, str] | None = None
        for cur, idx, prefix in pool_counts:
            if cur < target:
                chosen = (cur, idx, prefix)
                break

        if chosen is None:
            logger.info(f"[poll] 所有设备池已满（每池 target={target}）sleep {interval}s")
            await asyncio.sleep(interval)
            continue

        cur, idx, key_prefix = chosen
        missing = target - cur
        fill_n = min(missing, batch_max)
        logger.info(f"[poll] 选择池 idx={idx} key_prefix={key_prefix} cur={cur} target={target} missing={missing} -> 本轮补齐 {fill_n}")

        # 本轮写入到选中的池子
        os.environ["REDIS_DEVICE_POOL_KEY"] = key_prefix
        engine = SpiderEngine()
        await engine.run(tasks_n=fill_n)
        # 立即回读一次数量，帮助定位“为什么每轮缺口都不变”
        try:
            rcfg2 = _get_redis_config(max_devices_default=target)
            pool2 = RedisDevicePool(rcfg2)
            cur2 = pool2.count()
            logger.info(f"[poll] 本轮结束：key_prefix={rcfg2.key_prefix} cur={cur2} target={target} missing={max(0, target - cur2)}")
        except Exception as e:
            logger.warning(f"[poll] 本轮结束回读失败: {e}")
        # 本轮跑完立即进入下一轮检查（不额外 sleep）


if __name__ == "__main__":
    t = time.time()
    import sys

    # 【修正】代理加载逻辑
    proxy_file_path = "proxies.txt"  # 替换为你的真实路径

    if os.path.exists(proxy_file_path):
        with open(proxy_file_path, "r", encoding="utf-8") as f:
            # 过滤空行和空白字符
            Config.PROXIES = [line.strip() for line in f if line.strip()]
        print(f"已加载 {len(Config.PROXIES)} 个代理")
    else:
        print(f"警告：未找到代理文件 {proxy_file_path}，将使用空列表")
        # 这里可以放几个测试代理
        Config.PROXIES = []

    if not Config.PROXIES:
        print("错误：代理列表为空，程序退出")
        sys.exit(1)

    if sys.platform.startswith('win'):
        asyncio.set_event_loop_policy(asyncio.WindowsSelectorEventLoopPolicy())

    try:
        if Config.POLL_MODE:
            asyncio.run(_poll_fill_loop())
        else:
            engine = SpiderEngine()
            asyncio.run(engine.run())
    except KeyboardInterrupt:
        print("收到 Ctrl+C，已退出（建议开启 MWZZZH_FILE_FSYNC=1 提升异常退出不丢数据概率）")
    t1 = time.time()
    print("总耗时===>",t1-t)