#!/usr/bin/env bash
set -euo pipefail

echo "=== GO API SERVER (Linux) ==="

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT/api_server"

go mod tidy
go run .


