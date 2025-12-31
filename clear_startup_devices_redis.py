import argparse
import os
import platform
import sys
from typing import Optional, Tuple, List


def _load_env_for_runtime() -> str | None:
    """
    自动加载环境文件（让脚本能拿到 REDIS_PASSWORD 等配置）：
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

        load_dotenv(env_path, override=True)
        return env_path
    except Exception:
        try:
            with open(env_path, "r", encoding="utf-8", errors="ignore") as f:
                for line in f:
                    line = line.strip()
                    if not line or line.startswith("#") or "=" not in line:
                        continue
                    k, v = line.split("=", 1)
                    k = k.strip()
                    v = v.strip()
                    if k and k not in os.environ:
                        os.environ[k] = v
            return env_path
        except Exception:
            return env_path


_ENV_FILE = _load_env_for_runtime()


def _parse_bool(v: Optional[str], default: bool) -> bool:
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
    若传入形如 "tiktok:device_pool:3"，则回退为 "tiktok:device_pool"
    """
    prefix = (prefix or "").strip()
    if not prefix:
        return prefix
    parts = prefix.split(":")
    if len(parts) >= 2 and parts[-1].isdigit():
        base = ":".join(parts[:-1]).strip()
        return base or prefix
    return prefix


def _get_redis_conn_kwargs() -> Tuple[dict, str]:
    """
    支持：
    - REDIS_URL
    - 或 host/port/db/username/password/ssl
    """
    url = (os.getenv("REDIS_URL") or "").strip()
    if url:
        return {"from_url": url}, f"REDIS_URL={url}"

    host = os.getenv("REDIS_HOST", "127.0.0.1").strip()
    port = int(os.getenv("REDIS_PORT", "6379"))
    db = int(os.getenv("REDIS_DB", "0"))
    username = (os.getenv("REDIS_USERNAME") or "").strip() or None
    password = (os.getenv("REDIS_PASSWORD") or "").strip() or None
    ssl = _parse_bool(os.getenv("REDIS_SSL"), False)
    info = f"{host}:{port} db={db} ssl={ssl}"
    return {
        "host": host,
        "port": port,
        "db": db,
        "username": username,
        "password": password,
        "ssl": ssl,
        "decode_responses": True,
    }, info


def _build_prefixes(base_prefix: str, shards: int) -> List[str]:
    base_prefix = base_prefix.strip()
    if shards <= 1:
        return [base_prefix]
    out = [base_prefix]
    # 约定：0 号池为 base，其它为 base:{idx}
    for i in range(1, shards):
        out.append(f"{base_prefix}:{i}")
    return out


def _keys_for_prefix(prefix: str) -> List[str]:
    # 与 Go/Python 设备池一致
    return [
        f"{prefix}:ids",
        f"{prefix}:data",
        f"{prefix}:use",
        f"{prefix}:fail",
        f"{prefix}:play",
        f"{prefix}:attempt",
        f"{prefix}:seq",
        f"{prefix}:in",
    ]


def main() -> int:
    parser = argparse.ArgumentParser(
        description="清空 signup 注册成功写入的 Redis 设备池（ids/data/use/fail/play/attempt/seq/in）。"
    )
    parser.add_argument(
        "--prefix",
        default="",
        help="设备池 base prefix（默认读取 REDIS_STARTUP_DEVICE_POOL_KEY，其次 REDIS_DEVICE_POOL_KEY，最后 tiktok:device_pool）",
    )
    parser.add_argument(
        "--shards",
        type=int,
        default=0,
        help="分库数量（默认读取 REDIS_DEVICE_POOL_SHARDS；<=1 表示不分库）",
    )
    parser.add_argument(
        "--password",
        default="",
        help="可选：覆盖 REDIS_PASSWORD（临时用）。不填则从环境变量/环境文件读取。",
    )
    parser.add_argument(
        "--yes",
        action="store_true",
        help="确认执行删除（不带 --yes 只打印将删除哪些 key）",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="只打印，不删除（与不带 --yes 类似，优先级更高）",
    )

    args = parser.parse_args()

    try:
        import redis  # type: ignore
    except Exception as e:
        print("缺少依赖 redis，请先：pip install -r requirements.txt", file=sys.stderr)
        raise SystemExit(1) from e

    prefix = (args.prefix or "").strip()
    if not prefix:
        prefix = (os.getenv("REDIS_STARTUP_DEVICE_POOL_KEY") or "").strip()
    if not prefix:
        prefix = (os.getenv("REDIS_DEVICE_POOL_KEY") or "").strip()
    if not prefix:
        prefix = "tiktok:device_pool"

    # 避免用户传了 :idx，结果只清一个 shard：这里统一按 base 来清
    base_prefix = _normalize_pool_base(prefix)

    shards = int(args.shards or 0)
    if shards <= 0:
        shards = int(os.getenv("REDIS_DEVICE_POOL_SHARDS", "1") or "1")
    if shards <= 0:
        shards = 1

    prefixes = _build_prefixes(base_prefix, shards)
    keys: List[str] = []
    for p in prefixes:
        keys.extend(_keys_for_prefix(p))

    if args.password.strip():
        os.environ["REDIS_PASSWORD"] = args.password.strip()

    conn_kwargs, conn_info = _get_redis_conn_kwargs()
    if "from_url" in conn_kwargs:
        r = redis.Redis.from_url(conn_kwargs["from_url"], decode_responses=True)
    else:
        r = redis.Redis(**conn_kwargs)

    print(f"[redis] {conn_info}")
    print(f"[pool] base_prefix={base_prefix} shards={shards} prefixes={prefixes}")
    print(f"[plan] keys_to_delete ({len(keys)}):")
    for k in keys:
        print(f"  - {k}")

    if args.dry_run or not args.yes:
        print("[done] dry-run（未删除）。如确认删除，请加 --yes")
        return 0

    # 执行删除
    pipe = r.pipeline(transaction=False)
    for k in keys:
        pipe.delete(k)
    results = pipe.execute()

    deleted = 0
    for x in results:
        try:
            deleted += int(x)
        except Exception:
            pass

    print(f"[done] DEL 执行完成，返回删除计数累计={deleted}（Redis DEL 对不同类型 key 都返回 0/1）")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())


