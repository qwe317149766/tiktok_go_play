#!/usr/bin/env python3
"""
补全 device_pool_devices 表的 device_create_time 字段

从 device_json 中的 create_time 字段提取注册时间戳，并更新到 device_create_time 字段。
支持多种时间格式：
- "2006-01-02 15:04:05"
- "2006-01-02T15:04:05Z"
- Unix 时间戳（秒或毫秒）
"""

import argparse
import json
import os
import sys
from datetime import datetime
from pathlib import Path
from typing import Any

try:
    import pymysql
except ImportError:
    print("错误：缺少依赖 PyMySQL，请先 pip install -r requirements.txt", file=sys.stderr)
    sys.exit(1)


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


def _env_str(name: str, default: str) -> str:
    v = (os.getenv(name) or "").strip()
    return v or default


def _env_int(name: str, default: int) -> int:
    v = (os.getenv(name) or "").strip()
    if not v:
        return default
    try:
        return int(v)
    except Exception:
        return default


def parse_create_time(create_time_val: Any) -> datetime | None:
    """解析 create_time 字段，返回 datetime 对象"""
    if create_time_val is None:
        return None

    # 字符串格式
    if isinstance(create_time_val, str):
        create_time_str = create_time_val.strip()
        if not create_time_str:
            return None

        # 尝试常见时间格式
        formats = [
            "%Y-%m-%d %H:%M:%S",
            "%Y-%m-%dT%H:%M:%S",
            "%Y-%m-%dT%H:%M:%SZ",
            "%Y-%m-%dT%H:%M:%S.%f",
            "%Y-%m-%dT%H:%M:%S.%fZ",
        ]
        for fmt in formats:
            try:
                return datetime.strptime(create_time_str, fmt)
            except ValueError:
                continue

        # 尝试 Unix 时间戳（秒或毫秒）
        try:
            ts = float(create_time_str)
            if ts > 1e10:  # 毫秒时间戳
                ts = ts / 1000
            return datetime.fromtimestamp(ts)
        except (ValueError, OSError):
            pass

    # 数字格式（Unix 时间戳）
    if isinstance(create_time_val, (int, float)):
        ts = float(create_time_val)
        if ts > 1e10:  # 毫秒时间戳
            ts = ts / 1000
        try:
            return datetime.fromtimestamp(ts)
        except (OSError, ValueError):
            pass

    return None


def main() -> int:
    parser = argparse.ArgumentParser(description="补全 device_pool_devices 表的 device_create_time 字段")
    parser.add_argument("--env-file", default=None, help="指定 env 文件路径（默认自动查找）")
    parser.add_argument("--dry-run", action="store_true", help="仅检查，不实际更新数据库")
    parser.add_argument("--limit", type=int, default=0, help="最多处理多少条记录（0=不限制）")
    parser.add_argument("--batch-size", type=int, default=1000, help="批量处理大小（默认 1000）")
    args = parser.parse_args()

    env_path = _load_env(args.env_file)
    if env_path:
        print(f"[env] 已加载: {env_path}")

    # 连接数据库
    host = _env_str("DB_HOST", "127.0.0.1")
    port = _env_int("DB_PORT", 3306)
    user = _env_str("DB_USER", "root")
    password = _env_str("DB_PASSWORD", "123456")
    db_name = _env_str("DB_NAME", "tiktok_go_play")
    table = _env_str("DB_DEVICE_POOL_TABLE", "device_pool_devices")

    try:
        conn = pymysql.connect(
            host=host,
            port=port,
            user=user,
            password=password,
            database=db_name,
            charset="utf8mb4",
            autocommit=False,
        )
    except Exception as e:
        print(f"[err] 数据库连接失败: {e}", file=sys.stderr)
        return 1

    try:
        with conn.cursor() as cur:
            # 检查字段是否存在
            cur.execute(f"SHOW COLUMNS FROM `{table}` LIKE 'device_create_time'")
            if not cur.fetchone():
                print(f"[err] 表 {table} 中不存在 device_create_time 字段，请先执行 ALTER TABLE 添加该字段", file=sys.stderr)
                print(f"[hint] ALTER TABLE `{table}` ADD COLUMN device_create_time TIMESTAMP NULL DEFAULT NULL COMMENT 'Device registration time';", file=sys.stderr)
                return 1

            # 查询需要补全的记录（device_create_time 为 NULL 的记录）
            query = f"SELECT id, device_json FROM `{table}` WHERE device_create_time IS NULL"
            if args.limit > 0:
                query += f" LIMIT {args.limit}"
            cur.execute(query)
            rows = cur.fetchall()

            if not rows:
                print(f"[ok] 所有记录的 device_create_time 字段已补全")
                return 0

            print(f"[info] 找到 {len(rows)} 条需要补全的记录")

            updated = 0
            failed = 0
            skipped = 0

            batch_size = max(1, args.batch_size)
            update_sql = f"UPDATE `{table}` SET device_create_time=%s WHERE id=%s"

            for i, (record_id, device_json_str) in enumerate(rows):
                try:
                    device_json = json.loads(device_json_str)
                    create_time_val = device_json.get("create_time")
                    dt = parse_create_time(create_time_val)

                    if dt is None:
                        skipped += 1
                        if (i + 1) % 100 == 0:
                            print(f"[progress] 已处理 {i+1}/{len(rows)} 条，更新 {updated} 条，跳过 {skipped} 条，失败 {failed} 条")
                        continue

                    if not args.dry_run:
                        cur.execute(update_sql, (dt, record_id))
                    updated += 1

                    if (i + 1) % batch_size == 0:
                        if not args.dry_run:
                            conn.commit()
                        print(f"[progress] 已处理 {i+1}/{len(rows)} 条，更新 {updated} 条，跳过 {skipped} 条，失败 {failed} 条")

                except json.JSONDecodeError as e:
                    failed += 1
                    print(f"[warn] 记录 id={record_id} 的 device_json 解析失败: {e}", file=sys.stderr)
                except Exception as e:
                    failed += 1
                    print(f"[warn] 记录 id={record_id} 处理失败: {e}", file=sys.stderr)

            # 提交最后一批
            if not args.dry_run and updated > 0:
                conn.commit()

            print(f"\n[ok] 处理完成：")
            print(f"  - 总记录数: {len(rows)}")
            print(f"  - 成功更新: {updated}")
            print(f"  - 跳过（无 create_time）: {skipped}")
            print(f"  - 失败: {failed}")
            if args.dry_run:
                print(f"  - 模式: 仅检查（dry-run），未实际更新数据库")

    except Exception as e:
        print(f"[err] 处理失败: {e}", file=sys.stderr)
        conn.rollback()
        return 1
    finally:
        conn.close()

    return 0


if __name__ == "__main__":
    sys.exit(main())

