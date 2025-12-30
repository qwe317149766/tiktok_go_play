#!/usr/bin/env bash
set -euo pipefail

echo "=== ONE-CLICK RUN (Linux) ==="

# 0) 进入脚本所在目录（仓库根目录）
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

# 1) Python 注册设备（写入 Redis 设备池）
# 注意：mwzzzh_spider.py 会检查 proxies.txt（为空会退出）
echo
echo "[1/3] mwzzzh_spider.py"
python3 ./mwzzzh_spider.py

# 2) Go signup(startUp)（从 Redis 读取设备池，注册并写入 cookies 池）
echo
echo "[2/3] goPlay/demos/signup/dgemail"
cd "$ROOT/goPlay/demos/signup/dgemail"
if [[ -x "./dgemail.exe" ]]; then
  ./dgemail.exe
else
  go run .
fi

# 3) Go stats（从 Redis 读取设备池 + startUp cookies 池，执行播放/统计）
echo
echo "[3/3] goPlay/demos/stats/dgmain3"
cd "$ROOT/goPlay/demos/stats/dgmain3"
go run .


