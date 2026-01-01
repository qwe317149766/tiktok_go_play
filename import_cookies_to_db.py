import json
import os
import sys
import zlib
import ast
from dataclasses import dataclass
from pathlib import Path
from typing import Any


"""
一键导入（目录模式）

把待导入 cookies/账号文件放到 `IMPORT_COOKIES_DIR` 指定的目录下（支持多文件）：
- 支持：*.txt / *.jsonl / *.json（递归扫描）
- 每“行”可以是：
  - cookie header: k=v; k2=v2
  - cookies JSON: {"sessionid":"...","sid_tt":"..."}
  - account JSON: {"device_id":"...","cookies":{...}, ...}

默认开启：文件内去重 + 跨文件(DB)去重 + 分片写入 + （可选）导入后按 use_count 淘汰

说明：DB 连接参数仍从 env 文件/环境变量读取（DB_HOST/DB_USER/...），这里不做硬编码。
"""

# =========================
# 硬编码参数（按需改这里）
# =========================
SCRIPT_DIR = Path(__file__).resolve().parent

# env 文件：留空=自动查找 repo 默认（.env.windows/env.windows/.env.linux/env.linux）
ENV_FILE = ""  # 例如 "env.windows"

# 导入目录（相对脚本目录；也可写绝对路径）
IMPORT_COOKIES_DIR = "import_cookies"

# 只读取这些扩展名的文件（递归）
IMPORT_COOKIE_EXTS = {".txt", ".jsonl", ".json"}

# 导入模式：append=追加/覆盖单条；overwrite=清空表后导入；evict=导入后按 use_count 最大淘汰到 DB_MAX_COOKIES
MODE = "append"

# 文件内去重（按 device_key）
DEDUPE_FILE = True

# 跨文件增量去重（按 device_key 查询 DB 已存在则跳过写入；减少 UPSERT 压力）
# 注意：大数据量时建议设为 False（DB UNIQUE KEY 已保证唯一性，UPSERT 可能更快）
DEDUPE_DB = False

# 最多处理多少“行”（0=不限制；.json 数组会按元素计数）
LIMIT_LINES = 0

# batch 大小（影响 DB 查询 IN (...) 与写入节奏）
BATCH_SIZE = 1000

# 进度输出频率（每处理 N 行打印一次进度；0=不输出）
PROGRESS_EVERY_LINES = 10000

# 批量写入参数（大数据量建议开启）
# - 使用 cursor.executemany 将一批 rows 合并成多值 INSERT（PyMySQL 会做优化），明显快于逐行 execute
USE_EXECUTEMANY = True
EXECUTEMANY_BATCH_SIZE = 200  # 每次 executemany 的行数（account_json 较大时建议 50~300，避免 max_allowed_packet）

# 提交频率（减少 commit 次数会更快，但太大可能占用更多内存/事务时间）
COMMIT_EVERY_BATCHES = 10  # 每处理多少个 batch（BATCH_SIZE）提交一次


def _iter_input_files(base: Path) -> list[Path]:
    if base.is_file():
        return [base]
    if not base.exists():
        return []
    files: list[Path] = []
    for p in base.rglob("*"):
        if p.is_file() and p.suffix.lower() in IMPORT_COOKIE_EXTS:
            files.append(p)
    return sorted(files)


def _iter_lines_from_file(fp: Path):
    """
    统一产出“行字符串”：
    - .txt/.jsonl: 按行
    - .json: 若是数组 => 每个元素转成一行（str/dict）
    """
    suf = fp.suffix.lower()
    if suf == ".json":
        try:
            raw = fp.read_text(encoding="utf-8", errors="ignore").strip()
        except Exception:
            return
        if not raw:
            return
        try:
            obj = json.loads(raw)
        except Exception:
            for line in raw.splitlines():
                yield line
            return
        if isinstance(obj, list):
            for it in obj:
                if it is None:
                    continue
                if isinstance(it, str):
                    yield it
                elif isinstance(it, dict):
                    yield json.dumps(it, ensure_ascii=False, separators=(",", ":"))
                else:
                    yield str(it)
            return
        if isinstance(obj, dict):
            yield json.dumps(obj, ensure_ascii=False, separators=(",", ":"))
            return
        yield str(obj)
        return

    try:
        with fp.open("r", encoding="utf-8", errors="ignore") as f:
            for line in f:
                yield line
    except Exception:
        return

def _chunks(items: list[str], size: int) -> list[list[str]]:
    if size <= 0:
        size = 500
    out: list[list[str]] = []
    for i in range(0, len(items), size):
        out.append(items[i : i + size])
    return out


def _fetch_existing_device_keys(cur, table: str, keys: list[str], chunk_size: int = 500) -> set[str]:
    """
    从 DB 查询哪些 device_key 已存在，用于跨文件增量去重。
    - 只用于减少重复 UPSERT 写入；DB 侧仍依赖 UNIQUE KEY 做最终兜底。
    """
    exists: set[str] = set()
    keys = [x for x in keys if (x or "").strip()]
    if not keys:
        return exists
    for chunk in _chunks(keys, chunk_size):
        ph = ",".join(["%s"] * len(chunk))
        cur.execute(f"SELECT device_key FROM `{table}` WHERE device_key IN ({ph})", tuple(chunk))
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


def _parse_cookie_header(line: str) -> dict[str, str]:
    out: dict[str, str] = {}
    parts = line.split(";")
    for p in parts:
        p = p.strip()
        if not p:
            continue
        kv = p.split("=", 1)
        if len(kv) != 2:
            continue
        k = kv[0].strip()
        v = kv[1].strip()
        if not k:
            continue
        out[k] = v
    return out


def _cookie_id(cookies: dict[str, str]) -> str:
    sid = (cookies.get("sessionid") or "").strip()
    if sid:
        return sid
    sid_tt = (cookies.get("sid_tt") or "").strip()
    if sid_tt:
        return sid_tt
    # stable hash
    b = json.dumps(cookies, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8", errors="ignore")
    return str(int(zlib.crc32(b) & 0xFFFFFFFF))


def _parse_line_to_account_json(line: str) -> tuple[str, str] | None:
    """
    Return (device_key, account_json).
    Supported line formats:
    - cookie header: "k=v; k2=v2"
    - json cookies: {"sessionid":"...","sid_tt":"..."}
    - account json: {"device_id":"...","cookies":{...}, ...}
    """
    line = line.strip()
    if not line:
        return None

    if line.startswith("{") and line.endswith("}"):
        try:
            obj = json.loads(line)
        except Exception:
            return None
        if isinstance(obj, dict):
            # account json?
            if "cookies" in obj:
                ck_raw = obj.get("cookies")
                if isinstance(ck_raw, dict):
                    cookies = {str(k).strip(): str(v).strip() for k, v in ck_raw.items() if str(k).strip()}
                elif isinstance(ck_raw, str):
                    # 尝试解析 JSON 字符串或 Python 字典字符串 (e.g. "{'a': 'b'}")
                    cookies = {}
                    s_cookies = ck_raw.strip()
                    if s_cookies.startswith("{") and s_cookies.endswith("}"):
                        try:
                            cookies = json.loads(s_cookies)
                        except:
                            try:
                                cookies = ast.literal_eval(s_cookies)
                            except:
                                pass
                    
                    # 如果不是字典，或是解析失败，尝试作为 header 解析
                    if not cookies or not isinstance(cookies, dict):
                        cookies = _parse_cookie_header(s_cookies)
                else:
                    cookies = {}
                if not cookies:
                    return None
                device_key = str(obj.get("device_key") or obj.get("device_id") or _cookie_id(cookies)).strip()
                if not device_key:
                    device_key = _cookie_id(cookies)
                obj["device_id"] = device_key
                obj["device_key"] = device_key
                obj["cookies"] = cookies
                raw = json.dumps(obj, ensure_ascii=False, separators=(",", ":"))
                return device_key, raw
            # cookies json only
            cookies = {str(k).strip(): str(v).strip() for k, v in obj.items() if str(k).strip()}
            if not cookies:
                return None
            device_key = _cookie_id(cookies)
            acc = {"device_id": device_key, "device_key": device_key, "cookies": cookies}
            raw = json.dumps(acc, ensure_ascii=False, separators=(",", ":"))
            return device_key, raw
        return None

    # cookie header
    if "=" in line:
        cookies = _parse_cookie_header(line)
        if not cookies:
            return None
        device_key = _cookie_id(cookies)
        acc = {"device_id": device_key, "device_key": device_key, "cookies": cookies}
        raw = json.dumps(acc, ensure_ascii=False, separators=(",", ":"))
        return device_key, raw

    return None


@dataclass(frozen=True)
class DBConfig:
    host: str
    port: int
    user: str
    password: str
    db: str
    table: str
    shards: int
    max_cookies: int


def _get_db_cfg() -> DBConfig:
    host = _env_str("DB_HOST", "127.0.0.1")
    port = _env_int("DB_PORT", 3306)
    user = _env_str("DB_USER", "root")
    password = _env_str("DB_PASSWORD", "123456")
    db = _env_str("DB_NAME", "tiktok_go_play")
    table = _env_str("DB_COOKIE_POOL_TABLE", "startup_cookie_accounts")
    shards = _env_int("DB_COOKIE_POOL_SHARDS", 1)
    if shards <= 0:
        shards = 1
    max_cookies = _env_int("DB_MAX_COOKIES", 0)
    if max_cookies < 0:
        max_cookies = 0
    return DBConfig(
        host=host,
        port=port,
        user=user,
        password=password,
        db=db,
        table=table,
        shards=shards,
        max_cookies=max_cookies,
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

    inp_base = Path(IMPORT_COOKIES_DIR.strip() or "import_cookies")
    if not inp_base.is_absolute():
        inp_base = (SCRIPT_DIR / inp_base).resolve()
    files = _iter_input_files(inp_base)
    if not files:
        print(f"[err] 导入目录不存在或为空：{inp_base}", file=sys.stderr)
        print(f"[hint] 请把 cookies/账号文件放到该目录下（支持扩展名：{sorted(IMPORT_COOKIE_EXTS)}）", file=sys.stderr)
        return 2

    mode = (MODE or "append").strip() or "append"
    dedupe = bool(DEDUPE_FILE)
    dedupe_db = bool(DEDUPE_DB)
    limit_lines = int(LIMIT_LINES or 0)
    batch_size = int(BATCH_SIZE or 500)
    if batch_size <= 0:
        batch_size = 500

    print(f"[env] loaded={env_path}")
    print(f"[cfg] input_dir={inp_base} files={len(files)} exts={sorted(IMPORT_COOKIE_EXTS)}")
    for fp in files[:20]:
        print(f"[file] {fp}")
    if len(files) > 20:
        print(f"[file] ... +{len(files)-20} more")
    print(f"[cfg] mode={mode} dedupe_file={dedupe} dedupe_db={dedupe_db} limit_lines={limit_lines}")

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
                f"INSERT INTO `{cfg.table}` (shard_id, device_key, account_json) "
                f"VALUES (%s,%s,%s) "
                f"ON DUPLICATE KEY UPDATE shard_id=VALUES(shard_id), account_json=VALUES(account_json)"
            )

            # batch：先文件内去重，再按 batch 查询 DB 已存在 key，然后只写入缺失的
            batch_keys: list[str] = []
            batch_rows: list[tuple[int, str, str]] = []
            pending_batches = 0
            db_query_counter = 0  # 用于降低 DB 查询频率

            def flush_batch(force_query: bool = False):
                nonlocal added, db_dup_skipped, batch_keys, batch_rows, pending_batches, db_query_counter
                if not batch_rows:
                    return
                exists: set[str] = set()
                # 降低 DB 查询频率：每 5 个 batch 才查一次（或强制查询）
                should_query = dedupe_db and mode != "overwrite" and (force_query or (db_query_counter % 5 == 0))
                if should_query:
                    exists = _fetch_existing_device_keys(cur, cfg.table, batch_keys, chunk_size=min(500, batch_size))
                    db_query_counter += 1

                # 过滤 DB 已存在（增量去重）
                to_write: list[tuple[int, str, str]] = []
                if exists:
                    for shard, device_key, acc_json in batch_rows:
                        if device_key in exists:
                            db_dup_skipped += 1
                            continue
                        to_write.append((shard, device_key, acc_json))
                else:
                    to_write = batch_rows

                if to_write:
                    use_executemany = bool(USE_EXECUTEMANY)
                    if use_executemany:
                        # executemany 会在 INSERT 场景下合并成多值写入（更快）
                        step = max(1, int(EXECUTEMANY_BATCH_SIZE or 1))
                        for i in range(0, len(to_write), step):
                            part = to_write[i : i + step]
                            cur.executemany(upsert_sql, part)
                            added += len(part)
                    else:
                        for shard, device_key, acc_json in to_write:
                            cur.execute(upsert_sql, (shard, device_key, acc_json))
                            added += 1

                pending_batches += 1
                if pending_batches >= max(1, int(COMMIT_EVERY_BATCHES or 1)):
                    conn.commit()
                    pending_batches = 0
                batch_keys = []
                batch_rows = []

            progress_every = int(PROGRESS_EVERY_LINES or 0)
            file_idx = 0
            for fp in files:
                file_idx += 1
                print(f"[progress] 处理文件 {file_idx}/{len(files)}: {fp.name} (已处理 {total} 行，已导入 {added} 条)")
                for line in _iter_lines_from_file(fp):
                    line = (line or "").strip()
                    if not line:
                        continue
                    total += 1
                    if limit_lines > 0 and total > limit_lines:
                        break
                    if progress_every > 0 and total % progress_every == 0:
                        print(f"[progress] 已处理 {total} 行，已导入 {added} 条，无效 {invalid} 条，文件内重复 {dup_skipped} 条，DB重复 {db_dup_skipped} 条")
                    parsed = _parse_line_to_account_json(line)
                    if not parsed:
                        invalid += 1
                        continue
                    device_key, acc_json = parsed
                    if dedupe:
                        if device_key in seen:
                            dup_skipped += 1
                            continue
                        seen.add(device_key)
                    shard = _stable_shard(device_key, cfg.shards)
                    batch_keys.append(device_key)
                    batch_rows.append((shard, device_key, acc_json))
                    if len(batch_rows) >= batch_size:
                        flush_batch()
                if limit_lines > 0 and total >= limit_lines:
                    break
            flush_batch(force_query=True)  # 最后一个 batch 强制查询
            if pending_batches > 0:
                conn.commit()
                pending_batches = 0

            if mode == "evict" and cfg.max_cookies > 0:
                for sh in range(cfg.shards):
                    cnt = _count_shard(cur, cfg.table, sh)
                    if cnt > cfg.max_cookies:
                        need = cnt - cfg.max_cookies
                        cur.execute(
                            f"DELETE FROM `{cfg.table}` WHERE shard_id=%s ORDER BY use_count DESC LIMIT {int(need)}",
                            (sh,),
                        )

            per = []
            total_cnt = 0
            for sh in range(cfg.shards):
                cnt = _count_shard(cur, cfg.table, sh)
                per.append((sh, cnt))
                total_cnt += cnt

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


