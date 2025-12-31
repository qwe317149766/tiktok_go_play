import argparse
import json
import os
import platform
from pathlib import Path
from typing import Iterable, List, Tuple, Dict, Any


def _pick_env_file() -> str | None:
    sysname = platform.system().lower()
    if "windows" in sysname:
        candidates = [".env.windows", "env.windows"]
    else:
        candidates = [".env.linux", "env.linux"]
    for p in candidates:
        if os.path.exists(p):
            return p
    return None


def _load_env() -> str | None:
    env_path = _pick_env_file()
    if not env_path:
        return None
    try:
        from dotenv import load_dotenv  # type: ignore
    except Exception:
        return env_path
    load_dotenv(env_path, override=True)
    return env_path


def _get_int(name: str, default: int) -> int:
    v = os.getenv(name)
    if v is None:
        return default
    v = v.strip()
    if not v:
        return default
    try:
        return int(v)
    except Exception:
        return default


def _normalize_pool_base(prefix: str) -> str:
    prefix = (prefix or "").strip()
    if not prefix:
        return prefix
    parts = prefix.split(":")
    if len(parts) >= 2 and parts[-1].isdigit():
        base = ":".join(parts[:-1]).strip()
        return base or prefix
    return prefix


def _redis_client():
    try:
        import redis  # type: ignore
    except Exception as e:
        raise RuntimeError("缺少依赖 redis，请先 pip install -r requirements.txt") from e

    url = (os.getenv("REDIS_URL") or "").strip()
    if url:
        return redis.Redis.from_url(url, decode_responses=True)

    host = (os.getenv("REDIS_HOST") or "127.0.0.1").strip()
    port = _get_int("REDIS_PORT", 6379)
    db = _get_int("REDIS_DB", 0)
    username = (os.getenv("REDIS_USERNAME") or "").strip() or None
    password = (os.getenv("REDIS_PASSWORD") or "").strip() or None
    use_ssl = (os.getenv("REDIS_SSL") or "").strip().lower() in {"1", "true", "yes", "y", "on"}

    return redis.Redis(
        host=host,
        port=port,
        db=db,
        username=username,
        password=password,
        ssl=use_ssl,
        decode_responses=True,
    )


def _iter_cookie_ids(r, ids_key: str, batch: int = 2000) -> Iterable[str]:
    cursor = 0
    while True:
        cursor, ids = r.sscan(ids_key, cursor=cursor, match="*", count=batch)
        for _id in ids:
            if _id:
                yield str(_id)
        if cursor == 0:
            break


def _hmget_chunks(r, data_key: str, ids: List[str], chunk: int) -> Iterable[Tuple[str, str]]:
    for i in range(0, len(ids), chunk):
        sub = ids[i : i + chunk]
        vals = r.hmget(data_key, sub)
        for idx, raw in enumerate(vals):
            if raw is None:
                continue
            s = str(raw).strip()
            if not s:
                continue
            yield sub[idx], s


def export_one_prefix(r, prefix: str, out_fp, limit: int, hmget_chunk: int, mode: str) -> int:
    ids_key = f"{prefix}:ids"
    data_key = f"{prefix}:data"

    ids: List[str] = []
    for _id in _iter_cookie_ids(r, ids_key):
        ids.append(_id)
        if limit > 0 and len(ids) >= limit:
            break

    wrote = 0
    for cid, raw in _hmget_chunks(r, data_key, ids, hmget_chunk):
        try:
            ck = json.loads(raw)
        except Exception:
            continue
        if not isinstance(ck, dict) or not ck:
            continue
        # 默认导出“账号原始 JSON”（与 signup 落盘 devices12_20.txt 一致：完整 device 字段 + create_time + cookies 字段）
        # 旧格式兼容：cookie_record -> {"id": "...", "cookies": {...}}
        if mode == "cookie_record":
            out_fp.write(json.dumps({"id": cid, "cookies": ck}, ensure_ascii=False) + "\n")
        else:
            # account：直接输出 Redis 中的 value（ck 本身就是完整账号 JSON）
            out_fp.write(json.dumps(ck, ensure_ascii=False) + "\n")
        wrote += 1
        if limit > 0 and wrote >= limit:
            break
    return wrote


def main():
    _load_env()

    ap = argparse.ArgumentParser(description="导出 Redis 的 startup_cookie_pool 到文件（JSONL，一行一个）")
    ap.add_argument("--out", default="startup_accounts.jsonl", help="输出文件路径（默认 startup_accounts.jsonl）")
    ap.add_argument("--limit", type=int, default=0, help="最多导出多少条（0=不限制）")
    ap.add_argument("--hmget-chunk", type=int, default=_get_int("REDIS_HMGET_CHUNK", 500), help="HMGET 分批大小")
    ap.add_argument("--shards", type=int, default=_get_int("REDIS_COOKIE_POOL_SHARDS", 1), help="cookies 分库数（默认取 env）")
    ap.add_argument("--prefix", default=os.getenv("REDIS_STARTUP_COOKIE_POOL_KEY", "tiktok:startup_cookie_pool"),
                    help="cookies 池前缀（默认取 env: REDIS_STARTUP_COOKIE_POOL_KEY）")
    ap.add_argument("--mode", default="account", choices=["account", "cookie_record"],
                    help="导出格式：account=直接导出账号原始JSON（默认，推荐）；cookie_record=导出 {id,cookies} 包装格式（兼容旧用法）")
    args = ap.parse_args()

    prefix_base = _normalize_pool_base(args.prefix or "tiktok:startup_cookie_pool")
    shards = max(1, int(args.shards or 1))
    hmget_chunk = max(10, int(args.hmget_chunk or 500))
    limit = int(args.limit or 0)

    out_path = Path(args.out).expanduser().resolve()
    out_path.parent.mkdir(parents=True, exist_ok=True)

    r = _redis_client()
    # 连接校验
    r.ping()

    total = 0
    with out_path.open("w", encoding="utf-8") as f:
        for i in range(shards):
            p = prefix_base if i == 0 else f"{prefix_base}:{i}"
            n = export_one_prefix(r, p, f, limit=0, hmget_chunk=hmget_chunk, mode=str(args.mode))
            total += n
            if limit > 0 and total >= limit:
                break

    print(f"已导出 records={total} 到文件: {out_path} (mode={args.mode})")


if __name__ == "__main__":
    main()


