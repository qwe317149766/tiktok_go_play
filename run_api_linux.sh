#!/usr/bin/env bash
set -euo pipefail

echo "=== GO API SERVER (Linux) ==="

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT/api_server"

go mod tidy
# 允许远程访问：默认监听 0.0.0.0:8080（可通过 env.linux 或 ENV_FILE 覆盖）
export API_ADDR="${API_ADDR:-0.0.0.0:8080}"
go run .


