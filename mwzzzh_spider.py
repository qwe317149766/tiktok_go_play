import asyncio
import logging
import json
import os
import random
import traceback
import time
import platform
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
    sysname = platform.system().lower()
    if "windows" in sysname:
        candidates = [".env.windows", "env.windows"]
    else:
        candidates = [".env.linux", "env.linux"]

    env_path = None
    for p in candidates:
        if os.path.exists(p):
            env_path = p
            break

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
    key_prefix = os.getenv("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool")
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
                pipe.zadd(self.use_key, {dev_id: 0})
                pipe.zadd(self.fail_key, {dev_id: 0})
                pipe.zadd(self.play_key, {dev_id: 0})
                pipe.zadd(self.attempt_key, {dev_id: 0})
            pipe.execute()
            write_n += 1

        evicted = self.evict_if_needed()
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
    # 任务数量：
    # - 优先 MWZZZH_TASKS
    # - 若未配置，则复用 MAX_GENERATE（与你的设备生成配置保持一致）
    # - 再否则默认 1000
    TASKS = _get_int_from_env("MWZZZH_TASKS", "MAX_GENERATE", default=1000)


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
async def user_custom_logic(task_id, task_params, proxy, pipeline, thread_pool):
    """
    这里负责指挥：
    1. 遇到 IO (网络) -> await curl_cffi
    2. 遇到 CPU (计算) -> run_in_executor (扔给线程)
    """
    async with AsyncSession(impersonate="chrome131_android") as session:
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

    def get_proxy(self):
        return next(self.proxy_cycle)

    async def _worker_wrapper(self, task_id, task_params):
        async with self.sem:
            try:
                # 把线程池传进去
                await user_custom_logic(task_id, task_params, self.get_proxy(), self.pipeline, self.cpu_pool)
            except Exception as e:
                logger.error(f"Wrapper error: {e}")

    async def run(self):
        await self.pipeline.start()

        # 生成任务
        tasks_data = [{"id": i} for i in range(Config.TASKS)]
        logger.info(f"开始任务，网络并发: {Config.MAX_CONCURRENCY}, 解析线程: {Config.THREAD_POOL_SIZE}")

        coroutines = []
        for i, params in enumerate(tasks_data):
            task = asyncio.create_task(self._worker_wrapper(i, params))
            coroutines.append(task)

        # 等待完成
        try:
            from tqdm.asyncio import tqdm
            _ = [await f for f in tqdm.as_completed(coroutines)]
        except ImportError:
            await asyncio.gather(*coroutines)

        logger.info("任务完成，清理中...")

        # 关闭线程池
        self.cpu_pool.shutdown()
        await self.pipeline.stop()


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

    engine = SpiderEngine()
    asyncio.run(engine.run())
    t1 = time.time()
    print("总耗时===>",t1-t)