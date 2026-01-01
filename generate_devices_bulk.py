import argparse
import json
import os
import platform
import threading
import time
import uuid
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass
from typing import Any, Dict, List, Optional

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


#
# 全 DB 模式：不再支持写 Redis。


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
        description="批量生成设备：生成后写入 10 个 txt 备份文件（全 DB 模式：不再支持写 Redis）。"
    )
    parser.add_argument("--env-file", default=None, help="指定 env 文件（默认自动选择 .env.windows / .env.linux）")

    parser.add_argument("--save-file", action="store_true", help="保存到文件（默认由 env 决定）")
    parser.add_argument("--no-save-file", action="store_true", help="不保存到文件（覆盖 env）")

    parser.add_argument("--max-generate", type=int, default=None, help="最多生成设备数量（上限 100000）")
    parser.add_argument("--concurrency", type=int, default=None, help="生成并发数")

    parser.add_argument("--out-dir", default="device_backups", help="备份文件输出目录")
    parser.add_argument("--file-prefix", default="devices", help="备份文件前缀，如 devices_0.txt")
    parser.add_argument("--per-file-max", type=int, default=None, help="每个文件最多保存条数")

    args = parser.parse_args()

    env_path = _load_env(args.env_file)

    # 配置：是否写文件
    env_save_file = _parse_bool(os.getenv("SAVE_TO_FILE"), True)
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
    print(f"[cfg] save_file={save_file}, max_generate={max_generate}, concurrency={concurrency}")
    if save_file:
        print(f"[cfg] out_dir={args.out_dir}, file_prefix={args.file_prefix}, per_file_max={per_file_max} (10个文件)")

    t0 = time.time()
    devices = generate_devices(max_generate, concurrency=concurrency)
    print(f"[gen] 生成完成: {len(devices)} 条，用时 {time.time()-t0:.2f}s")

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


