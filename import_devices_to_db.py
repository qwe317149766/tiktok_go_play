import json
import os
import sys
import zlib
from dataclasses import dataclass
from pathlib import Path
from typing import Any


"""
一键导入（目录模式）

把待导入设备文件放到 `IMPORT_DEVICES_DIR` 指定的目录下（支持多文件）：
- 支持：*.jsonl / *.txt（按行解析 JSON，每行一个设备 dict）
- 默认开启：文件内去重 + 跨文件(DB)去重 + 分片写入 + 导入后按 use_count 淘汰

说明：DB 连接参数仍从 env 文件/环境变量读取（DB_HOST/DB_USER/...），这里不做硬编码。
"""

# =========================
# 硬编码参数（按需改这里）
# =========================
SCRIPT_DIR = Path(__file__).resolve().parent

# env 文件：留空=自动查找 repo 默认（.env.windows/env.windows/.env.linux/env.linux）
ENV_FILE = "env.windows"  # 例如 "env.windows"

# 导入目录（相对脚本目录；也可写绝对路径）
IMPORT_DEVICES_DIR = "device_backups"

# 只读取这些扩展名的文件（递归）
IMPORT_DEVICE_EXTS = {".jsonl", ".txt"}

# 导入模式：overwrite=清空表后导入；evict=导入后按 use_count 最大淘汰到 DB_MAX_DEVICES
MODE = "evict"

# 设备 ID 字段名（优先用该字段；若缺失会 fallback 到 device_id/cdid/...）
DEVICE_ID_FIELD = "device_id"

# 文件内去重（按 device_id）
DEDUPE_FILE = True

# 跨文件增量去重（按 device_id 查询 DB 已存在则跳过写入；减少 UPSERT 压力）
# 注意：大数据量时建议设为 False（DB UNIQUE KEY 已保证唯一性，UPSERT 可能更快）
DEDUPE_DB = False

# 最多处理多少行（0=不限制）
LIMIT_LINES = 0

# batch 大小（影响 DB 查询 IN (...) 与写入节奏）
BATCH_SIZE = 1000

# 进度输出频率（每处理 N 行打印一次进度；0=不输出）
PROGRESS_EVERY_LINES = 10000

# 批量写入参数（大数据量建议开启）
# - 使用 cursor.executemany 将一批 rows 合并成多值 INSERT（PyMySQL 会做优化），明显快于逐行 execute
USE_EXECUTEMANY = True
EXECUTEMANY_BATCH_SIZE = 200  # 每次 executemany 的行数（device_json 较大时建议 50~300，避免 max_allowed_packet）

# 提交频率（减少 commit 次数会更快，但太大可能占用更多内存/事务时间）
COMMIT_EVERY_BATCHES = 10  # 每处理多少个 batch（BATCH_SIZE）提交一次


def _iter_input_files(base: Path) -> list[Path]:
    if base.is_file():
        return [base]
    if not base.exists():
        return []
    files: list[Path] = []
    for p in base.rglob("*"):
        if p.is_file() and p.suffix.lower() in IMPORT_DEVICE_EXTS:
            files.append(p)
    return sorted(files)


def _chunks(items: list[str], size: int) -> list[list[str]]:
    if size <= 0:
        size = 500
    out: list[list[str]] = []
    for i in range(0, len(items), size):
        out.append(items[i : i + size])
    return out


def _fetch_existing_device_ids(cur, table: str, ids: list[str], chunk_size: int = 500) -> set[str]:
    """
    从 DB 查询哪些 device_id 已存在，用于跨文件增量去重。
    - 只用于减少重复 UPSERT 写入；DB 侧仍依赖 UNIQUE KEY 做最终兜底。
    """
    exists: set[str] = set()
    ids = [x for x in ids if (x or "").strip()]
    if not ids:
        return exists
    for chunk in _chunks(ids, chunk_size):
        ph = ",".join(["%s"] * len(chunk))
        cur.execute(f"SELECT device_id FROM `{table}` WHERE device_id IN ({ph})", tuple(chunk))
        rows = cur.fetchall() or []
        for r in rows:
            try:
                exists.add(str(r[0]))
            except Exception:
                continue
    return exists


def _simple_load_env_file(path: str, override: bool = True) -> None:
    try:
        with open(path, "r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith("#") or "=" not in line:
                    continue
                k, v = line.split("=", 1)
                k = k.strip()
                v = v.strip()
                if not k:
                    continue
                if len(v) >= 2 and ((v[0] == v[-1] == '"') or (v[0] == v[-1] == "'")):
                    v = v[1:-1]
                if not override and k in os.environ:
                    continue
                os.environ[k] = v
    except Exception:
        return


def _load_env(explicit: str | None) -> str | None:
    if explicit:
        p = explicit.strip()
        if p and os.path.exists(p):
            try:
                from dotenv import load_dotenv  # type: ignore

                load_dotenv(p, override=True)
            except Exception:
                _simple_load_env_file(p, override=True)
            return p

    # repo default: env.windows / env.linux (and dot variants)
    candidates = [".env.windows", "env.windows", ".env.linux", "env.linux"]
    for p in candidates:
        if os.path.exists(p):
            try:
                from dotenv import load_dotenv  # type: ignore

                load_dotenv(p, override=True)
            except Exception:
                _simple_load_env_file(p, override=True)
            return p
    return None


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


def _stable_shard(key: str, shards: int) -> int:
    if shards <= 1:
        return 0
    key = (key or "").strip()
    if not key:
        return 0
    return int(zlib.crc32(key.encode("utf-8", errors="ignore")) & 0xFFFFFFFF) % shards


def _extract_device_id(m: dict[str, Any], id_field: str) -> str:
    if not m:
        return ""
    raw = m.get(id_field)
    if isinstance(raw, str) and raw.strip():
        return raw.strip()
    if isinstance(raw, (int, float)):
        return str(int(raw))
    # fallback
    for k in ("device_id", "cdid", "clientudid", "openudid", "install_id"):
        v = m.get(k)
        if isinstance(v, str) and v.strip():
            return v.strip()
        if isinstance(v, (int, float)):
            return str(int(v))
    return ""


@dataclass(frozen=True)
class DBConfig:
    host: str
    port: int
    user: str
    password: str
    db: str
    table: str
    shards: int
    max_devices: int


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
    max_devices = _env_int("DB_MAX_DEVICES", 0)
    if max_devices < 0:
        max_devices = 0
    return DBConfig(
        host=host,
        port=port,
        user=user,
        password=password,
        db=db,
        table=table,
        shards=shards,
        max_devices=max_devices,
    )


def _open_conn(cfg: DBConfig):
    try:
        import pymysql  # type: ignore
    except Exception as e:
        raise RuntimeError("缺少依赖 PyMySQL，请先 pip install -r requirements.txt") from e
    return pymysql.connect(
        host=cfg.host,
        port=int(cfg.port),
        user=cfg.user,
        password=cfg.password,
        database=cfg.db,
        charset="utf8mb4",
        autocommit=False,  # 大批量导入：手动提交更快
    )


def _count_shard(cur, table: str, shard: int) -> int:
    cur.execute(f"SELECT COUNT(*) FROM `{table}` WHERE shard_id=%s", (int(shard),))
    row = cur.fetchone()
    return int(row[0]) if row else 0


def main() -> int:
    env_path = None
    if ENV_FILE.strip():
        p = Path(ENV_FILE.strip())
        if not p.is_absolute():
            p = (SCRIPT_DIR / p).resolve()
        env_path = _load_env(str(p))
    else:
        env_path = _load_env(None)

    cfg = _get_db_cfg()

    inp_base = Path(IMPORT_DEVICES_DIR.strip() or "import_devices")
    if not inp_base.is_absolute():
        inp_base = (SCRIPT_DIR / inp_base).resolve()
    files = _iter_input_files(inp_base)
    if not files:
        print(f"[err] 导入目录不存在或为空：{inp_base}", file=sys.stderr)
        print(f"[hint] 请把设备文件放到该目录下（支持扩展名：{sorted(IMPORT_DEVICE_EXTS)}）", file=sys.stderr)
        return 2

    id_field = (DEVICE_ID_FIELD or "device_id").strip() or "device_id"
    mode = (MODE or "evict").strip() or "evict"
    dedupe = bool(DEDUPE_FILE)
    dedupe_db = bool(DEDUPE_DB)
    limit_lines = int(LIMIT_LINES or 0)
    batch_size = int(BATCH_SIZE or 500)
    if batch_size <= 0:
        batch_size = 500

    print(f"[env] loaded={env_path}")
    print(f"[cfg] input_dir={inp_base} files={len(files)} exts={sorted(IMPORT_DEVICE_EXTS)}")
    for fp in files[:20]:
        print(f"[file] {fp}")
    if len(files) > 20:
        print(f"[file] ... +{len(files)-20} more")
    print(f"[cfg] mode={mode} id_field={id_field} dedupe_file={dedupe} dedupe_db={dedupe_db} limit_lines={limit_lines}")

    conn = _open_conn(cfg)
    try:
        with conn.cursor() as cur:
            if mode == "overwrite":
                cur.execute(f"DELETE FROM `{cfg.table}`")

            added = 0
            invalid = 0
            dup_skipped = 0
            db_dup_skipped = 0
            total = 0
            seen: set[str] = set()

            upsert_sql = (
                f"INSERT INTO `{cfg.table}` (shard_id, device_id, device_json) "
                f"VALUES (%s,%s,%s) "
                f"ON DUPLICATE KEY UPDATE shard_id=VALUES(shard_id), device_json=VALUES(device_json)"
            )

            # 以 batch 的方式处理：先文件内去重，再按 batch 查询 DB 已存在 ids，然后只写入缺失的
            batch_ids: list[str] = []
            batch_rows: list[tuple[int, str, str]] = []
            pending_batches = 0
            db_query_counter = 0  # 用于降低 DB 查询频率

            def flush_batch(force_query: bool = False):
                nonlocal added, db_dup_skipped, batch_ids, batch_rows, pending_batches, db_query_counter
                if not batch_rows:
                    return
                exists: set[str] = set()
                # 降低 DB 查询频率：每 5 个 batch 才查一次（或强制查询）
                should_query = dedupe_db and mode != "overwrite" and (force_query or (db_query_counter % 5 == 0))
                if should_query:
                    exists = _fetch_existing_device_ids(cur, cfg.table, batch_ids, chunk_size=min(500, batch_size))
                    db_query_counter += 1

                # 过滤 DB 已存在（增量去重）
                to_write: list[tuple[int, str, str]] = []
                if exists:
                    for shard, did, raw in batch_rows:
                        if did in exists:
                            db_dup_skipped += 1
                            continue
                        to_write.append((shard, did, raw))
                else:
                    to_write = batch_rows

                if to_write:
                    if USE_EXECUTEMANY:
                        # executemany 会在 INSERT 场景下合并成多值写入（更快）
                        step = max(1, int(EXECUTEMANY_BATCH_SIZE or 1))
                        for i in range(0, len(to_write), step):
                            part = to_write[i : i + step]
                            cur.executemany(upsert_sql, part)
                            added += len(part)
                    else:
                        for shard, did, raw in to_write:
                            cur.execute(upsert_sql, (shard, did, raw))
                            added += 1

                pending_batches += 1
                if pending_batches >= max(1, int(COMMIT_EVERY_BATCHES or 1)):
                    conn.commit()
                    pending_batches = 0
                batch_ids = []
                batch_rows = []

            progress_every = int(PROGRESS_EVERY_LINES or 0)
            file_idx = 0
            for fp in files:
                file_idx += 1
                print(f"[progress] 处理文件 {file_idx}/{len(files)}: {fp.name} (已处理 {total} 行，已导入 {added} 条)")
                with fp.open("r", encoding="utf-8", errors="ignore") as f:
                    for line in f:
                        line = line.strip()
                        if not line:
                            continue
                        total += 1
                        if limit_lines > 0 and total > limit_lines:
                            break
                        if progress_every > 0 and total % progress_every == 0:
                            print(f"[progress] 已处理 {total} 行，已导入 {added} 条，无效 {invalid} 条，文件内重复 {dup_skipped} 条，DB重复 {db_dup_skipped} 条")
                        try:
                            m = json.loads(line)
                            if not isinstance(m, dict):
                                invalid += 1
                                continue
                        except Exception:
                            invalid += 1
                            continue
                        did = _extract_device_id(m, id_field)
                        if not did:
                            invalid += 1
                            continue
                        if dedupe:
                            if did in seen:
                                dup_skipped += 1
                                continue
                            seen.add(did)
                        shard = _stable_shard(did, cfg.shards)
                        raw = json.dumps(m, ensure_ascii=False, separators=(",", ":"))
                        batch_ids.append(did)
                        batch_rows.append((shard, did, raw))
                        if len(batch_rows) >= batch_size:
                            flush_batch()
                if limit_lines > 0 and total >= limit_lines:
                    break
            flush_batch(force_query=True)  # 最后一个 batch 强制查询
            if pending_batches > 0:
                conn.commit()
                pending_batches = 0

            if mode == "evict" and cfg.max_devices > 0:
                for sh in range(cfg.shards):
                    cnt = _count_shard(cur, cfg.table, sh)
                    if cnt > cfg.max_devices:
                        need = cnt - cfg.max_devices
                        # 按 use_count 最大淘汰
                        cur.execute(
                            f"DELETE FROM `{cfg.table}` WHERE shard_id=%s ORDER BY use_count DESC LIMIT {int(need)}",
                            (sh,),
                        )
                conn.commit()

            per = []
            total_cnt = 0
            for sh in range(cfg.shards):
                cnt = _count_shard(cur, cfg.table, sh)
                per.append((sh, cnt))
                total_cnt += cnt
            conn.commit()

    finally:
        conn.close()

    print(f"[ok] table={cfg.table} db={cfg.db} shards={cfg.shards} mode={mode}")
    print(
        f"[ok] input_lines={total} imported={added} invalid={invalid} dup_skipped={dup_skipped} db_dup_skipped={db_dup_skipped}"
    )
    print(f"[ok] total_in_db={total_cnt}")
    for sh, cnt in per:
        print(f"[shard] {sh}: {cnt}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())


