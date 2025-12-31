import argparse
import json
from pathlib import Path


def iter_cache_lines(p: Path):
    if not p.exists():
        return
    with p.open("r", encoding="utf-8", errors="ignore") as f:
        for line in f:
            line = (line or "").strip()
            if not line or line.startswith("#"):
                continue
            yield line


def parse_cache_line(line: str):
    # 格式: device_id:seed,seed_type,token
    if ":" not in line:
        return None
    device_id, rest = line.split(":", 1)
    device_id = device_id.strip()
    parts = [x.strip() for x in rest.split(",")]
    if not device_id or len(parts) != 3:
        return None
    seed = parts[0]
    try:
        seed_type = int(parts[1])
    except Exception:
        return None
    token = parts[2]
    if not seed or not token:
        return None
    return {
        "device_id": device_id,
        "seed": seed,
        "seed_type": seed_type,
        "token": token,
    }


def main():
    ap = argparse.ArgumentParser(description="导出 stats/dgmain3 的 device_cache.txt（device_id:seed,seed_type,token）")
    ap.add_argument(
        "--in",
        dest="inp",
        default="goPlay/demos/stats/dgmain3/device_cache.txt",
        help="输入缓存文件路径（默认 goPlay/demos/stats/dgmain3/device_cache.txt）",
    )
    ap.add_argument(
        "--out",
        dest="out",
        default="device_cache_export.jsonl",
        help="输出 JSONL 路径（默认 device_cache_export.jsonl）",
    )
    ap.add_argument(
        "--ids-only",
        action="store_true",
        help="只导出 device_id（一行一个），忽略 seed/token",
    )
    args = ap.parse_args()

    in_path = Path(args.inp).expanduser().resolve()
    out_path = Path(args.out).expanduser().resolve()
    out_path.parent.mkdir(parents=True, exist_ok=True)

    wrote = 0
    bad = 0
    seen = set()

    with out_path.open("w", encoding="utf-8") as out_fp:
        for line in iter_cache_lines(in_path):
            if args.ids_only:
                if ":" not in line:
                    bad += 1
                    continue
                device_id = line.split(":", 1)[0].strip()
                if not device_id:
                    bad += 1
                    continue
                if device_id in seen:
                    continue
                seen.add(device_id)
                out_fp.write(device_id + "\n")
                wrote += 1
                continue

            rec = parse_cache_line(line)
            if not rec:
                bad += 1
                continue
            if rec["device_id"] in seen:
                continue
            seen.add(rec["device_id"])
            out_fp.write(json.dumps(rec, ensure_ascii=False) + "\n")
            wrote += 1

    print(f"输入: {in_path}")
    print(f"输出: {out_path}")
    print(f"导出: {wrote} 条（去重后），跳过坏行: {bad}")


if __name__ == "__main__":
    main()


