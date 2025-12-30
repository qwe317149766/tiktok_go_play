import hashlib
import json
import os
import platform
from dataclasses import dataclass
from typing import Any


def _parse_bool(v: str | None, default: bool) -> bool:
    if v is None:
        return default
    v = v.strip().lower()
    if v in {"1", "true", "yes", "y", "on"}:
        return True
    if v in {"0", "false", "no", "n", "off"}:
        return False
    return default


def _load_env(env_file: str | None) -> str | None:
    """
    载入 env（默认按系统选择 env.windows / env.linux），以文件为准 override=True。
    """
    if env_file:
        path = env_file
    else:
        sysname = platform.system().lower()
        if "windows" in sysname:
            candidates = [".env.windows", "env.windows"]
        else:
            candidates = [".env.linux", "env.linux"]
        path = next((p for p in candidates if os.path.exists(p)), None)

    if not path:
        return None

    try:
        from dotenv import load_dotenv  # type: ignore

        load_dotenv(path, override=True)
    except Exception:
        # 没装 python-dotenv 也不致命：用户也可以通过系统环境变量提供
        pass
    return path


def _load_json_config() -> tuple[str, dict[str, Any]]:
    """
    零命令行参数：默认读取 cookies_import_config.json
    也可用环境变量 COOKIES_IMPORT_CONFIG 指定配置文件路径。
    """
    cfg_path = (os.getenv("COOKIES_IMPORT_CONFIG") or "cookies_import_config.json").strip()
    if not cfg_path:
        cfg_path = "cookies_import_config.json"
    if not os.path.exists(cfg_path):
        raise FileNotFoundError(
            f"未找到配置文件：{cfg_path}\n"
            f"请在项目根目录创建 cookies_import_config.json（可参考 cookies_import_config.example.json）。\n"
            f"也可通过环境变量 COOKIES_IMPORT_CONFIG 指定配置文件路径。"
        )
    # Windows 下 PowerShell/记事本常带 BOM，这里用 utf-8-sig 兼容
    with open(cfg_path, "r", encoding="utf-8-sig", errors="ignore") as f:
        cfg = json.load(f)
    if not isinstance(cfg, dict):
        raise ValueError(f"配置文件必须是 JSON object：{cfg_path}")
    return cfg_path, cfg


@dataclass(frozen=True)
class RedisCfg:
    url: str | None
    host: str
    port: int
    db: int
    username: str | None
    password: str | None
    ssl: bool
    cookie_pool_prefix: str


def _get_redis_cfg() -> RedisCfg:
    url = os.getenv("REDIS_URL") or None
    host = os.getenv("REDIS_HOST", "127.0.0.1")
    port = int(os.getenv("REDIS_PORT", "6379"))
    db = int(os.getenv("REDIS_DB", "0"))
    username = os.getenv("REDIS_USERNAME") or None
    password = os.getenv("REDIS_PASSWORD") or None
    ssl = _parse_bool(os.getenv("REDIS_SSL"), False)
    cookie_pool_prefix = os.getenv("REDIS_STARTUP_COOKIE_POOL_KEY", "tiktok:startup_cookie_pool")
    return RedisCfg(
        url=url,
        host=host,
        port=port,
        db=db,
        username=username,
        password=password,
        ssl=ssl,
        cookie_pool_prefix=cookie_pool_prefix,
    )


def _get_redis_client(cfg: RedisCfg):
    try:
        import redis  # type: ignore
    except Exception as e:
        raise RuntimeError("缺少依赖：redis，请先 pip install -r requirements.txt") from e

    if cfg.url:
        r = redis.Redis.from_url(cfg.url, decode_responses=True)
    else:
        r = redis.Redis(
            host=cfg.host,
            port=cfg.port,
            db=cfg.db,
            username=cfg.username,
            password=cfg.password,
            ssl=cfg.ssl,
            decode_responses=True,
        )
    r.ping()
    return r


def _cookie_id(cookies: dict[str, str]) -> str:
    # 与 Go 侧逻辑对齐：优先 sessionid / sid_tt，否则 sha1(json)
    sid = (cookies.get("sessionid") or "").strip()
    if sid:
        return sid
    sid_tt = (cookies.get("sid_tt") or "").strip()
    if sid_tt:
        return sid_tt
    b = json.dumps(cookies, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")
    return hashlib.sha1(b).hexdigest()


def _parse_cookie_kv_string(s: str) -> dict[str, str]:
    """
    解析 cookie 字符串：
      "k=v; k2=v2" / "k=v;k2=v2"
    """
    out: dict[str, str] = {}
    for part in s.split(";"):
        part = part.strip()
        if not part or "=" not in part:
            continue
        k, v = part.split("=", 1)
        k = k.strip()
        v = v.strip()
        if not k:
            continue
        out[k] = v
    return out


def _parse_cookie_line(line: str) -> dict[str, str] | None:
    # 兼容 BOM：某些文件首行可能带 \ufeff
    line = line.lstrip("\ufeff").strip()
    if not line or line.startswith("#"):
        return None

    # 1) JSON 对象
    if line.startswith("{") and line.endswith("}"):
        try:
            obj: Any = json.loads(line)
        except Exception:
            return None
        if not isinstance(obj, dict):
            return None
        cookies: dict[str, str] = {}
        for k, v in obj.items():
            if k is None:
                continue
            kk = str(k).strip()
            if kk == "":
                continue
            if v is None:
                vv = ""
            elif isinstance(v, (str, int, float, bool)):
                vv = str(v)
            else:
                # 复杂结构：直接 json 化
                vv = json.dumps(v, ensure_ascii=False, separators=(",", ":"))
            cookies[kk] = vv
        return cookies if cookies else None

    # 2) cookie string
    cookies = _parse_cookie_kv_string(line)
    return cookies if cookies else None


def main() -> None:
    cfg_path, json_cfg = _load_json_config()

    # env_file 可省略：省略时自动选择 env.windows/env.linux
    env_raw = str(json_cfg.get("env_file") or "").strip()
    if env_raw.lower() in {"auto", "default"}:
        env_raw = ""
    env_path = _load_env(env_raw or None)
    redis_cfg = _get_redis_cfg()

    # config: input/limit/dry_run/flush
    input_path = str((json_cfg.get("input") or json_cfg.get("input_file") or "")).strip()
    if not input_path:
        raise ValueError(f"配置缺少 input（cookies txt 文件路径）：{cfg_path}")

    limit = int(json_cfg.get("limit") or 0)
    dry_run = bool(json_cfg.get("dry_run") or False)
    flush = bool(json_cfg.get("flush") or False)

    # 可选覆盖 cookie 池前缀
    if isinstance(json_cfg.get("redis"), dict):
        rp = str(json_cfg["redis"].get("cookie_pool_prefix") or "").strip()
        if rp:
            redis_cfg = RedisCfg(
                url=redis_cfg.url,
                host=redis_cfg.host,
                port=redis_cfg.port,
                db=redis_cfg.db,
                username=redis_cfg.username,
                password=redis_cfg.password,
                ssl=redis_cfg.ssl,
                cookie_pool_prefix=rp,
            )

    print(f"[config] file: {cfg_path}")
    print(f"[config] input={input_path} limit={limit} dry_run={dry_run} flush={flush}")
    print(f"[env] loaded: {env_path}")
    print(
        f"[redis] host={redis_cfg.host} port={redis_cfg.port} db={redis_cfg.db} ssl={redis_cfg.ssl} "
        f"url={'set' if redis_cfg.url else 'not_set'} prefix={redis_cfg.cookie_pool_prefix}"
    )

    ids_key = f"{redis_cfg.cookie_pool_prefix}:ids"
    data_key = f"{redis_cfg.cookie_pool_prefix}:data"

    with open(input_path, "r", encoding="utf-8", errors="ignore") as f:
        lines = [ln.rstrip("\n") for ln in f]

    parsed: list[dict[str, str]] = []
    bad = 0
    for ln in lines:
        ck = _parse_cookie_line(ln)
        if ck is None:
            if ln.strip() and not ln.strip().startswith("#"):
                bad += 1
            continue
        parsed.append(ck)
        if limit > 0 and len(parsed) >= limit:
            break

    print(f"[parse] total_lines={len(lines)} ok={len(parsed)} bad={bad}")
    if not parsed:
        print("[parse] 没有可导入的 cookies（请检查格式：JSON 或 k=v; k2=v2）")
        return

    if dry_run:
        sample = parsed[0]
        print("[dry-run] sample keys:", sorted(sample.keys())[:20])
        print("[dry-run] sample id:", _cookie_id(sample))
        return

    r = _get_redis_client(redis_cfg)

    if flush:
        r.delete(ids_key, data_key)
        print(f"[redis] flushed: {ids_key}, {data_key}")

    write_n = 0
    exist_n = 0
    pipe = r.pipeline(transaction=True)
    for ck in parsed:
        cid = _cookie_id(ck)
        existed = r.sismember(ids_key, cid)
        if existed:
            exist_n += 1
        pipe.sadd(ids_key, cid)
        pipe.hset(data_key, cid, json.dumps(ck, ensure_ascii=False, separators=(",", ":")))
        write_n += 1
        if write_n % 500 == 0:
            pipe.execute()
            pipe = r.pipeline(transaction=True)
    pipe.execute()

    print(f"[redis] wrote={write_n} existed={exist_n}")
    print(f"[redis] SCARD ids={r.scard(ids_key)} HLEN data={r.hlen(data_key)}")


if __name__ == "__main__":
    main()


