import argparse
import json
import os
import platform
import threading
import time
import uuid
import zlib
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass
from typing import Any, Dict, List, Optional, Tuple

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
    sysname = platform.system().lower()
    if "windows" in sysname:
        candidates = [".env.windows", "env.windows"]
    else:
        candidates = [".env.linux", "env.linux"]

    for p in candidates:
        if os.path.exists(p):
            return p
    return candidates[0]


def _load_env(env_file: Optional[str]) -> str:
    env_path = env_file or _pick_env_file()
    try:
        from dotenv import load_dotenv  # type: ignore
    except Exception:
        return env_path
    load_dotenv(env_path, override=True)
    return env_path


def _env_int(name: str, default: int) -> int:
    v = (os.getenv(name) or "").strip()
    if not v:
        return default
    try:
        return int(v)
    except Exception:
        return default


def _env_str(name: str, default: str) -> str:
    v = (os.getenv(name) or "").strip()
    return v or default


@dataclass(frozen=True)
class DBConfig:
    host: str
    port: int
    user: str
    password: str
    db: str
    table: str
    shards: int


def _get_db_cfg() -> DBConfig:
    host = _env_str("DB_HOST", "127.0.0.1")
    port = _env_int("DB_PORT", 3306)
    user = _env_str("DB_USER", "root")
    password = _env_str("DB_PASSWORD", "123456")
    db = _env_str("DB_NAME", "tiktok_go_play")
    table = _env_str("DB_DEVICE_POOL_TABLE", "device_pool_devices")
    shards = _env_int("DB_DEVICE_POOL_SHARDS", 1)
    if shards <= 0:
        shards = 1
    return DBConfig(host, port, user, password, db, table, shards)


def _open_conn(cfg: DBConfig):
    try:
        import pymysql
    except Exception as e:
        raise RuntimeError("Missing PyMySQL, please pip install pymysql") from e
    return pymysql.connect(
        host=cfg.host,
        port=int(cfg.port),
        user=cfg.user,
        password=cfg.password,
        database=cfg.db,
        charset="utf8mb4",
        autocommit=True,
    )


def _stable_shard(key: str, shards: int) -> int:
    if shards <= 1:
        return 0
    key = (key or "").strip()
    if not key:
        return 0
    return int(zlib.crc32(key.encode("utf-8", errors="ignore")) & 0xFFFFFFFF) % shards


def _count_total_devices(cur, cfg: DBConfig) -> int:
    total = 0
    for s in range(cfg.shards):
        cur.execute(f"SELECT COUNT(*) FROM `{cfg.table}` WHERE shard_id=%s", (s,))
        row = cur.fetchone()
        if row:
            total += int(row[0])
    return total


def _gen_one_device() -> Dict[str, Any]:
    d = getANewDevice()
    d.setdefault("device_uid", d.get("cdid") or d.get("clientudid") or uuid.uuid4().hex)
    return d


def generate_devices_batch(n: int, concurrency: int) -> List[Dict[str, Any]]:
    n = int(n)
    if n <= 0:
        return []
    concurrency = max(1, int(concurrency))
    out: List[Dict[str, Any]] = []
    
    with ThreadPoolExecutor(max_workers=concurrency) as ex:
        futures = {ex.submit(_gen_one_device) for _ in range(n)}
        for fut in as_completed(futures):
            dev = fut.result()
            if dev:
                out.append(dev)
    return out


def insert_devices_to_db(conn, cfg: DBConfig, devices: List[Dict[str, Any]]) -> int:
    if not devices:
        return 0
    inserted = 0
    sql = (
        f"INSERT IGNORE INTO `{cfg.table}` (shard_id, device_id, device_json) "
        f"VALUES (%s, %s, %s)"
    )
    
    vals = []
    for d in devices:
        did = str(d.get("device_uid") or "")
        shard = _stable_shard(did, cfg.shards)
        raw = json.dumps(d, ensure_ascii=False, separators=(",", ":"))
        vals.append((shard, did, raw))
    
    with conn.cursor() as cur:
        # Batch insert
        batch_size = 200
        for i in range(0, len(vals), batch_size):
            chunk = vals[i : i + batch_size]
            cur.executemany(sql, chunk)
            inserted += len(chunk)  # Approximate, IGNORE might skip
    return inserted


def main():
    parser = argparse.ArgumentParser(description="Generate devices and insert directly into DB.")
    parser.add_argument("--env-file", default=None)
    args = parser.parse_args()

    _load_env(args.env_file)

    # 1. Config
    target_count = _env_int("DB_MAX_DEVICES", 0)
    if target_count <= 0:
        # Fallback to MAX_GENERATE if DB_MAX_DEVICES not set
        target_count = _env_int("MAX_GENERATE", 10000)
    
    concurrency = _env_int("GEN_CONCURRENCY", 200)
    if concurrency <= 0:
        concurrency = 200

    print(f"[config] Target DB Count: {target_count}")
    print(f"[config] Concurrency: {concurrency}")

    # 2. DB Connection
    cfg = _get_db_cfg()
    try:
        conn = _open_conn(cfg)
    except Exception as e:
        print(f"[error] DB Connection failed: {e}")
        return

    try:
        with conn.cursor() as cur:
            # Create table if not exists (simplified check)
            pass 
        
        # 3. Check current count
        with conn.cursor() as cur:
            current_count = _count_total_devices(cur, cfg)
        
        print(f"[status] Current devices in DB: {current_count}")
        
        if current_count >= target_count:
            print("[status] DB has enough devices. Exiting.")
            return

        needed = target_count - current_count
        print(f"[action] Need to generate {needed} devices.")

        # 4. Loop generate and insert
        batch_size = 1000
        generated_total = 0
        
        while generated_total < needed:
            batch_n = min(batch_size, needed - generated_total)
            print(f"[gen] Generating batch of {batch_n}...")
            
            devices = generate_devices_batch(batch_n, concurrency)
            
            print(f"[db] Inserting batch of {len(devices)}...")
            insert_devices_to_db(conn, cfg, devices)
            
            generated_total += len(devices)
            print(f"[status] Progress: {generated_total}/{needed}")

        print("[done] Generation and insertion complete.")

    finally:
        conn.close()


if __name__ == "__main__":
    main()


