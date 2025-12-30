import argparse
import json
import os
import platform
import threading
import time
import uuid
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass
from typing import Any, Dict, Iterable, List, Optional, Tuple

from devices import getANewDevice


def _parse_bool(v: str | None, default: bool) -> bool:
    if v is None:
        return default
    v = v.strip().lower()
    if v in {"1", "true", "yes", "y", "on"}:
        return True
    if v in {"0", "false", "no", "n", "off"}:
        return False
    return default


def _pick_env_file() -> str:
    """
    按系统选择 env：
    - Windows: .env.windows
    - Linux:   .env.linux
    若不存在则 fallback 到 .env
    """
    sysname = platform.system().lower()
    if "windows" in sysname:
        candidates = [".env.windows", "env.windows"]
    else:
        candidates = [".env.linux", "env.linux"]

    for p in candidates:
        if os.path.exists(p):
            return p

    # 都不存在也返回默认候选，方便提示用户
    return candidates[0]


def _load_env(env_file: Optional[str]) -> str:
    env_path = env_file or _pick_env_file()
    # 延迟 import，避免用户不使用 env 时强依赖
    try:
        from dotenv import load_dotenv  # type: ignore
    except Exception:
        return env_path
    # 以 env 文件为准：避免系统环境变量里残留的 MAX_GENERATE 等配置“悄悄覆盖”文件配置
    load_dotenv(env_path, override=True)
    return env_path


@dataclass(frozen=True)
class RedisConfig:
    url: Optional[str]
    host: str
    port: int
    db: int
    username: Optional[str]
    password: Optional[str]
    ssl: bool
    key_prefix: str
    id_field: str
    max_size: int


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
    )


class RedisDevicePool:
    """
    Redis 设备池结构（全部 key 都基于 key_prefix）：
    - {prefix}:ids        (SET)    所有设备 id
    - {prefix}:data       (HASH)   id -> device_json
    - {prefix}:use        (ZSET)   id -> use_count
    - {prefix}:fail       (ZSET)   id -> fail_count

    淘汰策略：
    - 当 ids 数量 > max_size：按 use_count 最大（最常用）淘汰
    """

    def __init__(self, cfg: RedisConfig):
        self.cfg = cfg
        self.ids_key = f"{cfg.key_prefix}:ids"
        self.data_key = f"{cfg.key_prefix}:data"
        self.use_key = f"{cfg.key_prefix}:use"
        self.fail_key = f"{cfg.key_prefix}:fail"

        try:
            import redis  # type: ignore
        except Exception as e:
            raise RuntimeError("缺少依赖：redis，请先 pip install redis") from e

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
        # fallback：保证一定有唯一 id
        return device.get("clientudid") or device.get("cdid") or uuid.uuid4().hex

    def add_devices(self, devices: Iterable[Dict[str, Any]]) -> Tuple[int, int]:
        """
        批量写入：
        - 新增设备：初始化 use=0 fail=0
        - 已存在：只更新 data，不重置计数
        返回：(写入数量, 淘汰数量)
        """
        write_n = 0
        for dev in devices:
            dev_id = self._extract_id(dev)
            dev_json = json.dumps(dev, ensure_ascii=False, separators=(",", ":"))

            pipe = self.r.pipeline(transaction=True)
            # 先看是否已有
            pipe.sismember(self.ids_key, dev_id)
            existed = pipe.execute()[0]

            pipe = self.r.pipeline(transaction=True)
            pipe.sadd(self.ids_key, dev_id)
            pipe.hset(self.data_key, dev_id, dev_json)
            if not existed:
                pipe.zadd(self.use_key, {dev_id: 0})
                pipe.zadd(self.fail_key, {dev_id: 0})
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
        # use_count 最大的（最常用）先淘汰：ZREVRANGE 0..excess-1
        ids = self.r.zrevrange(self.use_key, 0, excess - 1)
        if not ids:
            return 0
        pipe = self.r.pipeline(transaction=True)
        for dev_id in ids:
            pipe.srem(self.ids_key, dev_id)
            pipe.hdel(self.data_key, dev_id)
            pipe.zrem(self.use_key, dev_id)
            pipe.zrem(self.fail_key, dev_id)
        pipe.execute()
        return len(ids)


def _gen_one_device() -> Dict[str, Any]:
    d = getANewDevice()
    # 额外补一个稳定唯一 id（不影响既有字段）
    d.setdefault("device_uid", d.get("cdid") or d.get("clientudid") or uuid.uuid4().hex)
    return d


def generate_devices(n: int, concurrency: int) -> List[Dict[str, Any]]:
    """
    并发生成 n 个 device，尽量保证 device_uid 唯一。
    """
    n = int(n)
    if n <= 0:
        return []

    concurrency = max(1, int(concurrency))
    out: List[Dict[str, Any]] = []
    seen: set[str] = set()
    lock = threading.Lock()

    def accept(dev: Dict[str, Any]) -> bool:
        uid = str(dev.get("device_uid") or "")
        if not uid:
            return False
        with lock:
            if uid in seen:
                return False
            seen.add(uid)
            out.append(dev)
            return True

    with ThreadPoolExecutor(max_workers=concurrency) as ex:
        # 先灌满并发
        futures = {ex.submit(_gen_one_device) for _ in range(min(concurrency, n))}
        while futures and len(out) < n:
            for fut in as_completed(list(futures), timeout=None):
                futures.remove(fut)
                dev = fut.result()
                accept(dev)
                if len(out) >= n:
                    break
                futures.add(ex.submit(_gen_one_device))
    return out[:n]


def write_devices_to_10_files(
    devices: List[Dict[str, Any]],
    out_dir: str,
    file_prefix: str,
    per_file_max: int,
) -> List[str]:
    """
    平均分配到 10 个文件：round-robin 写入。
    """
    os.makedirs(out_dir, exist_ok=True)
    per_file_max = max(0, int(per_file_max))
    file_count = 10
    paths = [os.path.join(out_dir, f"{file_prefix}_{i}.txt") for i in range(file_count)]
    counts = [0] * file_count

    fps = [open(p, "a", encoding="utf-8") for p in paths]
    try:
        for idx, dev in enumerate(devices):
            fidx = idx % file_count
            if per_file_max and counts[fidx] >= per_file_max:
                # 该文件满了，寻找下一个未满文件
                found = False
                for j in range(file_count):
                    k = (fidx + j) % file_count
                    if counts[k] < per_file_max:
                        fidx = k
                        found = True
                        break
                if not found:
                    break  # 所有文件都满了
            line = json.dumps(dev, ensure_ascii=False, separators=(",", ":"))
            fps[fidx].write(line + "\n")
            counts[fidx] += 1
    finally:
        for fp in fps:
            fp.close()

    return paths


def main():
    parser = argparse.ArgumentParser(
        description="批量生成设备：可选写 Redis（含 use/fail 计数+淘汰），可选备份写入 10 个 txt 文件。"
    )
    parser.add_argument("--env-file", default=None, help="指定 env 文件（默认自动选择 .env.windows / .env.linux）")

    parser.add_argument("--save-redis", action="store_true", help="保存到 Redis（默认由 env 决定）")
    parser.add_argument("--no-save-redis", action="store_true", help="不保存到 Redis（覆盖 env）")

    parser.add_argument("--save-file", action="store_true", help="保存到文件（默认由 env 决定）")
    parser.add_argument("--no-save-file", action="store_true", help="不保存到文件（覆盖 env）")

    parser.add_argument("--max-generate", type=int, default=None, help="最多生成设备数量（上限 100000）")
    parser.add_argument("--concurrency", type=int, default=None, help="生成并发数")

    parser.add_argument("--out-dir", default="device_backups", help="备份文件输出目录")
    parser.add_argument("--file-prefix", default="devices", help="备份文件前缀，如 devices_0.txt")
    parser.add_argument("--per-file-max", type=int, default=None, help="每个文件最多保存条数")

    args = parser.parse_args()

    env_path = _load_env(args.env_file)

    # 配置：是否写 redis / 文件
    env_save_redis = _parse_bool(os.getenv("SAVE_TO_REDIS"), True)
    env_save_file = _parse_bool(os.getenv("SAVE_TO_FILE"), True)
    save_redis = (not args.no_save_redis) and (args.save_redis or env_save_redis)
    save_file = (not args.no_save_file) and (args.save_file or env_save_file)

    max_generate = int(args.max_generate or os.getenv("MAX_GENERATE", "100000"))
    max_generate = max(0, min(100000, max_generate))

    concurrency = int(args.concurrency or os.getenv("GEN_CONCURRENCY", "200"))
    concurrency = max(1, concurrency)

    per_file_max = int(args.per_file_max or os.getenv("PER_FILE_MAX", "10000"))
    per_file_max = max(0, per_file_max)

    # 文件：固定 10 个文件，所以总容量也有限制
    if save_file and per_file_max > 0:
        max_generate = min(max_generate, per_file_max * 10)

    print(f"[env] 使用 env 文件: {env_path}")
    print(f"[env] MAX_GENERATE(raw)={os.getenv('MAX_GENERATE')}, PER_FILE_MAX(raw)={os.getenv('PER_FILE_MAX')}, "
          f"GEN_CONCURRENCY(raw)={os.getenv('GEN_CONCURRENCY')}")
    print(f"[cfg] save_redis={save_redis}, save_file={save_file}, max_generate={max_generate}, concurrency={concurrency}")
    if save_file:
        print(f"[cfg] out_dir={args.out_dir}, file_prefix={args.file_prefix}, per_file_max={per_file_max} (10个文件)")

    pool: Optional[RedisDevicePool] = None
    if save_redis:
        rcfg = _get_redis_config(max_devices_default=max_generate)
        pool = RedisDevicePool(rcfg)
        pool.ping()
        print(
            f"[redis] ok key_prefix={rcfg.key_prefix}, id_field={rcfg.id_field}, max_size={rcfg.max_size} "
            f"(url={'set' if rcfg.url else 'not_set'})"
        )

    t0 = time.time()
    devices = generate_devices(max_generate, concurrency=concurrency)
    print(f"[gen] 生成完成: {len(devices)} 条，用时 {time.time()-t0:.2f}s")

    if save_redis and pool is not None:
        w, ev = pool.add_devices(devices)
        print(f"[redis] 写入完成: {w} 条，淘汰 {ev} 条")

    if save_file:
        paths = write_devices_to_10_files(
            devices=devices,
            out_dir=args.out_dir,
            file_prefix=args.file_prefix,
            per_file_max=per_file_max,
        )
        print(f"[file] 写入完成: {len(paths)} 个文件（追加写入）")


if __name__ == "__main__":
    main()


