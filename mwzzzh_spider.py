import asyncio
import logging
import json
import hashlib
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


def _simple_load_env_file(path: str, override: bool = True) -> None:
    """
    轻量级 .env loader（作为 python-dotenv 的兜底）：
    - 支持 KEY=VALUE
    - 忽略空行 / # 注释
    - 去掉首尾空白与一层引号（'...' 或 "..."）
    """
    try:
        with open(path, "r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith("#"):
                    continue
                if "=" not in line:
                    continue
                k, v = line.split("=", 1)
                k = k.strip()
                v = v.strip()
                if not k:
                    continue
                # strip single/double quotes once
                if len(v) >= 2 and ((v[0] == v[-1] == '"') or (v[0] == v[-1] == "'")):
                    v = v[1:-1]
                if not override and k in os.environ:
                    continue
                os.environ[k] = v
    except Exception:
        # best-effort: 不影响主流程
        return


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
            _simple_load_env_file(explicit, override=True)
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
        _simple_load_env_file(env_path, override=True)
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
class MySQLConfig:
    host: str
    port: int
    user: str
    password: str
    db: str
    table: str
    id_field: str
    shards: int
    force_shard: int | None


def _get_mysql_config() -> MySQLConfig:
    # 复用 env.linux / env.windows 的 DB_*（stats 抢单模式也在用）
    host = os.getenv("DB_HOST", "127.0.0.1")
    port = int(os.getenv("DB_PORT", "3306"))
    user = os.getenv("DB_USER", "root")
    password = os.getenv("DB_PASSWORD", "123456")
    db = os.getenv("DB_NAME", "tiktok_go_play")

    table = (os.getenv("DB_DEVICE_POOL_TABLE") or "device_pool_devices").strip() or "device_pool_devices"
    # 设备唯一字段：默认 device_id（可覆盖）
    id_field = (os.getenv("DEVICE_ID_FIELD") or "device_id").strip() or "device_id"

    shards = int(os.getenv("DB_DEVICE_POOL_SHARDS", "1"))
    if shards <= 0:
        shards = 1
    force_shard_raw = (os.getenv("MWZZZH_DB_DEVICE_SHARD") or "").strip()
    force_shard: int | None = None
    if force_shard_raw:
        try:
            force_shard = int(force_shard_raw)
        except Exception:
            force_shard = None

    return MySQLConfig(
        host=host,
        port=port,
        user=user,
        password=password,
        db=db,
        table=table,
        id_field=id_field,
        shards=shards,
        force_shard=force_shard,
    )


def _stable_shard(device_id: str, shards: int) -> int:
    if shards <= 1:
        return 0
    device_id = (device_id or "").strip()
    if not device_id:
        return 0
    h = hashlib.sha1(device_id.encode("utf-8", errors="ignore")).hexdigest()
    return int(h[:8], 16) % shards


class MySQLDevicePool:
    """
    MySQL 设备池：注册成功设备写入 MySQL，替代 Redis device_pool。

    表结构见：api_server/schema.sql -> device_pool_devices
    """

    def __init__(self, cfg: MySQLConfig):
        self.cfg = cfg
        try:
            import pymysql  # type: ignore
        except Exception as e:
            raise RuntimeError("缺少依赖：PyMySQL，请先 pip install -r requirements.txt") from e
        self._pymysql = pymysql
        self._conn = None

    def _conn_open(self):
        if self._conn is not None:
            return self._conn
        self._conn = self._pymysql.connect(
            host=self.cfg.host,
            port=int(self.cfg.port),
            user=self.cfg.user,
            password=self.cfg.password,
            database=self.cfg.db,
            charset="utf8mb4",
            autocommit=True,
        )
        return self._conn

    def ping(self) -> None:
        c = self._conn_open()
        with c.cursor() as cur:
            cur.execute("SELECT 1")
            _ = cur.fetchone()

    def count(self) -> int:
        c = self._conn_open()
        table = self.cfg.table
        with c.cursor() as cur:
            cur.execute(f"SELECT COUNT(*) FROM `{table}`")
            row = cur.fetchone()
        try:
            return int(row[0]) if row else 0
        except Exception:
            return 0

    def count_shard(self, shard_id: int) -> int:
        c = self._conn_open()
        table = self.cfg.table
        shard_id = int(shard_id) % int(self.cfg.shards or 1)
        with c.cursor() as cur:
            cur.execute(f"SELECT COUNT(*) FROM `{table}` WHERE shard_id=%s", (shard_id,))
            row = cur.fetchone()
        try:
            return int(row[0]) if row else 0
        except Exception:
            return 0

    def _extract_id(self, device: Dict[str, Any]) -> str:
        raw = device.get(self.cfg.id_field)
        if isinstance(raw, str) and raw.strip():
            return raw.strip()
        for k in ("cdid", "clientudid", "openudid", "device_id", "install_id"):
            v = device.get(k)
            if isinstance(v, str) and v.strip():
                return v.strip()
        return f"anon:{time.time_ns()}"

    def add_devices(self, devices: list[Dict[str, Any]]) -> int:
        if not devices:
            return 0
        c = self._conn_open()
        table = self.cfg.table
        rows = []
        for dev in devices:
            did = self._extract_id(dev)
            if self.cfg.force_shard is not None:
                shard = int(self.cfg.force_shard) % int(self.cfg.shards or 1)
            else:
                shard = _stable_shard(did, int(self.cfg.shards))
            raw = json.dumps(dev, ensure_ascii=False, separators=(",", ":"))
            rows.append((shard, did, raw))

        sql = (
            f"INSERT INTO `{table}` (shard_id, device_id, device_json) "
            f"VALUES (%s,%s,%s) "
            f"ON DUPLICATE KEY UPDATE device_json=VALUES(device_json), updated_at=CURRENT_TIMESTAMP"
        )
        with c.cursor() as cur:
            cur.executemany(sql, rows)
        return len(rows)


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

    # 结果文件（可选）：不需要写入可通过 MWZZZH_SAVE_RESULTS_FILE=0 关闭
    RESULT_FILE = os.getenv("MWZZZH_RESULT_FILE", os.getenv("RESULT_FILE", "results12_21_5.jsonl"))
    ERROR_FILE = "error.log"

    # 是否写入结果文件（results*.jsonl）
    # - 默认开启（保持兼容）
    # - 如果你不需要 results 文件，设置 MWZZZH_SAVE_RESULTS_FILE=0 可避免打开该文件
    SAVE_RESULTS_FILE = _parse_bool(os.getenv("MWZZZH_SAVE_RESULTS_FILE"), True)
    # results 写入失败是否视为致命错误：
    # - 默认 False：results 只是辅助日志，不应影响入库
    # - 如需强一致（results 必须落盘），可设置 MWZZZH_RESULTS_FATAL=1
    RESULTS_FATAL = _parse_bool(os.getenv("MWZZZH_RESULTS_FATAL"), False)

    # 设备池后端：
    # - db/mysql：写入 MySQL（全系统唯一来源）
    DEVICE_POOL_BACKEND = (os.getenv("DEVICE_POOL_BACKEND") or "db").strip().lower()

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

    # 轮询补齐模式（设备池补齐，run-once 友好，建议配合 cron）
    POLL_MODE = _parse_bool(
        os.getenv("MWZZZH_POLL_MODE"),
        default=False,
    )
    # 轮询间隔（秒）
    POLL_INTERVAL_SEC = _get_int_from_env("MWZZZH_POLL_INTERVAL_SEC", default=10)
    # 每个 shard 的目标数量：统一使用 DB_MAX_DEVICES（作为“池子最终数量/容量”）
    DB_MAX_DEVICES = _get_int_from_env("DB_MAX_DEVICES", default=TASKS)
    # 单轮补齐最大注册数量（避免一次补太多）
    POLL_BATCH_MAX = _get_int_from_env("MWZZZH_POLL_BATCH_MAX", default=TASKS)
    # 设备池分片数量：对应 device_pool_devices.shard_id
    DEVICE_POOL_SHARDS = _get_int_from_env("DB_DEVICE_POOL_SHARDS", default=1)

    # run-once：补齐后退出（适合 cron）
    POLL_ONCE = _parse_bool(os.getenv("MWZZZH_POLL_ONCE"), True)
    # 单次 run 最大补齐总量（0=不限制）
    POLL_MAX_TOTAL = _get_int_from_env("MWZZZH_POLL_MAX_TOTAL", default=0)

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

# 启动可观测性：告诉你 env 是否加载成功、是否会写 DB
try:
    logger.info(
        "[env] loaded=%s",
        _MWZZZH_ENV_FILE,
    )
    logger.info(
        "[env] DEVICE_POOL_BACKEND=%s | DB_HOST=%s | DB_PORT=%s | DB_NAME=%s | DB_DEVICE_POOL_TABLE=%s | DB_DEVICE_POOL_SHARDS=%s | DEVICE_ID_FIELD=%s",
        os.getenv("DEVICE_POOL_BACKEND"),
        os.getenv("DB_HOST"),
        os.getenv("DB_PORT"),
        os.getenv("DB_NAME"),
        os.getenv("DB_DEVICE_POOL_TABLE"),
        os.getenv("DB_DEVICE_POOL_SHARDS"),
        os.getenv("DEVICE_ID_FIELD") or "device_id",
    )
except Exception:
    pass


# ================= 3. 数据管道 (保持不变) =================
class DataPipeline:
    def __init__(
        self,
        filename,
        db_pool: MySQLDevicePool | None = None,
        save_to_file: bool = False,
        save_results_file: bool = True,
    ):
        self.filename = filename
        self.queue = asyncio.Queue()
        self.executor = ThreadPoolExecutor(max_workers=1)
        self.running = True
        self._writer_task = None
        self.db_pool = db_pool
        self.save_to_file = save_to_file
        self.save_results_file = bool(save_results_file) and bool((filename or "").strip())

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
        # 0) 同步写入设备池（最重要；先做，保证入库优先）
        if self.db_pool is None:
            raise RuntimeError("db_pool is required (Redis backend removed)")
        only_devices = [d for _, d in batch]
        _ = self.db_pool.add_devices(only_devices)

        # 1) 同步写入本地备份文件（可选）
        try:
            self._write_devices_to_backup_files(batch)
        except Exception as e:
            logger.critical(f"[file] 致命：写入本地备份失败: {e}")
            raise

        # 2) 写 results 文件（可选；最后做，避免影响入库）
        if self.save_results_file:
            try:
                with open(self.filename, 'a', encoding='utf-8') as f:
                    for _, item in batch:
                        line = json.dumps(item, ensure_ascii=False)
                        f.write(line + "\n")
            except Exception as e:
                # 默认不致命：results 只是辅助日志；不要因为它阻塞入库
                logger.error(f"[results] 写入失败（已忽略）: {e}")
                if Config.RESULTS_FATAL:
                    raise

    async def _consumer(self):
        # ✅ 严格写入语义：
        # - 只有当“写入成功”后，才对队列调用 task_done()
        # - 写入失败不会丢数据/不会提前 task_done，而是原批次重试直到成功
        batch_size = max(1, int(os.getenv("MWZZZH_PIPELINE_BATCH", "20") or "20"))
        retry_base = float(os.getenv("MWZZZH_PIPELINE_RETRY_SEC", "1") or "1")
        if retry_base <= 0:
            retry_base = 1.0
        retry_max = float(os.getenv("MWZZZH_PIPELINE_RETRY_MAX_SEC", "30") or "30")
        if retry_max <= 0:
            retry_max = 30.0

        batch: list[tuple[int | None, Dict[str, Any]]] = []
        retry = 0

        async def flush_batch(items: list[tuple[int | None, Dict[str, Any]]]) -> None:
            nonlocal retry
            if not items:
                return
            while True:
                try:
                    await asyncio.get_event_loop().run_in_executor(self.executor, self._write_impl, items)
                    # 写入成功：再统一 task_done（保证 queue.join() 真正代表“已落库/落盘”）
                    for _ in items:
                        self.queue.task_done()
                    retry = 0
                    return
                except Exception as e:
                    retry += 1
                    # 有设备池写入/备份时：必须保证成功写入，因此无限重试（退避）
                    delay = min(retry_base * (2 ** min(retry, 6)), retry_max)  # 上限约 64x
                    logger.error(f"[pipeline] 写入失败(将重试) retry={retry} sleep={delay:.1f}s err={e}")
                    await asyncio.sleep(delay)

        while self.running or not self.queue.empty() or batch:
            try:
                item = await asyncio.wait_for(self.queue.get(), timeout=1.0)
                batch.append(item)
            except asyncio.TimeoutError:
                pass

            if len(batch) >= batch_size or (not self.running and batch):
                to_write = batch[:]
                batch.clear()
                await flush_batch(to_write)

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
        backend = Config.DEVICE_POOL_BACKEND
        if backend not in {"db", "mysql"}:
            logger.critical(f"[cfg] DEVICE_POOL_BACKEND={backend} 不支持（已移除 Redis），请设置为 db/mysql")
            raise SystemExit(1)

        try:
            mcfg = _get_mysql_config()
            db_pool = MySQLDevicePool(mcfg)
            db_pool.ping()
            logger.info(
                f"[db] 启用成功 host={mcfg.host}:{mcfg.port} db={mcfg.db} table={mcfg.table} "
                f"id_field={mcfg.id_field} shards={mcfg.shards} force_shard={mcfg.force_shard} cur={db_pool.count()}"
            )
        except Exception as e:
            logger.critical(f"[db] 启用失败，程序终止: {e}")
            raise SystemExit(1)

        self.pipeline = DataPipeline(
            Config.RESULT_FILE,
            db_pool=db_pool,
            save_to_file=Config.SAVE_TO_FILE,
            save_results_file=Config.SAVE_RESULTS_FILE,
        )
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
    - 全 DB：每隔 POLL_INTERVAL_SEC 秒检查 MySQL 设备池各 shard 数量，若不足目标则补齐
    - 默认 run-once（MWZZZH_POLL_ONCE=1）：补齐后退出，适合 cron
    """
    backend = Config.DEVICE_POOL_BACKEND
    if backend not in {"db", "mysql"}:
        logger.critical(f"[poll] DEVICE_POOL_BACKEND={backend} 不支持（已移除 Redis），请设置为 db/mysql")
        raise SystemExit(1)

    interval = max(1, int(Config.POLL_INTERVAL_SEC))
    # 目标数量：每个 shard 的 target（与 dgemail 对齐）
    target = max(0, int(Config.DB_MAX_DEVICES))
    batch_max = max(1, int(Config.POLL_BATCH_MAX))
    shards = max(1, int(Config.DEVICE_POOL_SHARDS))
    run_once = bool(Config.POLL_ONCE)
    max_total = max(0, int(Config.POLL_MAX_TOTAL))
    filled_total = 0

    if target <= 0:
        logger.critical("[poll] DB_MAX_DEVICES 需要 > 0（每个 shard 的目标数量）")
        raise SystemExit(1)
    try:
        base_cfg = _get_mysql_config()
        db_pool = MySQLDevicePool(base_cfg)
        db_pool.ping()
    except Exception as e:
        logger.critical(f"[poll] MySQL 连接/读取失败，程序终止: {e}")
        raise SystemExit(1)

    logger.info(
        f"[poll] 启动：interval={interval}s target_per_shard={target} batch_max={batch_max} "
        f"shards={shards} run_once={run_once} max_total={max_total}"
    )

    while True:
        pool_counts: list[tuple[int, int]] = []
        for i in range(shards):
            pool_counts.append((db_pool.count_shard(i), i))

        # 选择一个“未满”的 shard：优先选择当前数量最少的（达到平均分配）
        pool_counts.sort(key=lambda x: x[0])
        chosen: tuple[int, int] | None = None
        for cur, idx in pool_counts:
            if cur < target:
                chosen = (cur, idx)
                break

        if chosen is None:
            logger.info(f"[poll] 所有 shard 已满（每 shard target={target}）")
            if run_once:
                return
            await asyncio.sleep(interval)
            continue

        cur, idx = chosen
        missing = target - cur
        fill_n = min(missing, batch_max)
        if max_total > 0:
            remain = max_total - filled_total
            if remain <= 0:
                logger.info(f"[poll] 达到本次补齐上限 max_total={max_total}，退出")
                return
            if fill_n > remain:
                fill_n = remain

        logger.info(f"[poll] 选择 shard_id={idx} cur={cur} target={target} missing={missing} -> 本轮补齐 {fill_n}")

        # 本轮写入到选中的 shard（仅用于补齐；设备唯一仍按 device_id）
        os.environ["MWZZZH_DB_DEVICE_SHARD"] = str(idx)
        engine = SpiderEngine()
        await engine.run(tasks_n=fill_n)
        filled_total += fill_n

        cur2 = db_pool.count_shard(idx)
        logger.info(f"[poll] 本轮结束：shard_id={idx} cur={cur2} target={target} missing={max(0, target - cur2)} filled_total={filled_total}")

        if not run_once:
            await asyncio.sleep(interval)


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