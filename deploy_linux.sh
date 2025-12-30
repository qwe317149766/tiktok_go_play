#!/usr/bin/env bash
set -euo pipefail

# 一键部署（Linux）：
# - Python: 创建 .venv 并安装 requirements.txt
# - Go: 在 goPlay 模块内编译 demos 二进制到 ./bin，并生成运行脚本（自动切目录）
#
# 用法：
#   chmod +x deploy_linux.sh
#   ./deploy_linux.sh

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

log() { printf "[deploy] %s\n" "$*"; }
die() { printf "[deploy][ERROR] %s\n" "$*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "缺少命令：$1（请先安装）"
}

need_cmd python3
need_cmd pip3
need_cmd go

log "Repo root: $ROOT_DIR"

PY_VENV_DIR="$ROOT_DIR/.venv"
REQ_FILE="$ROOT_DIR/requirements.txt"

if [[ ! -f "$REQ_FILE" ]]; then
  die "未找到 $REQ_FILE"
fi

log "1/3 创建 Python 虚拟环境并安装依赖"
if [[ ! -d "$PY_VENV_DIR" ]]; then
  python3 -m venv "$PY_VENV_DIR"
  log "已创建 venv: $PY_VENV_DIR"
else
  log "venv 已存在，跳过创建: $PY_VENV_DIR"
fi

# shellcheck disable=SC1091
source "$PY_VENV_DIR/bin/activate"
python -m pip install -U pip
python -m pip install -r "$REQ_FILE"
python -c "import curl_cffi,requests,urllib3,Crypto,cryptography,gmssl,ecdsa,google.protobuf; print('PY_OK')"
deactivate || true

log "2/3 编译 Go 程序（goPlay module）"
GOPLAY_DIR="$ROOT_DIR/goPlay"
if [[ ! -d "$GOPLAY_DIR" ]]; then
  die "未找到 goPlay 目录：$GOPLAY_DIR"
fi
if [[ ! -f "$GOPLAY_DIR/go.mod" ]]; then
  die "未找到 goPlay/go.mod（请先补齐 Go module）"
fi

BIN_DIR="$ROOT_DIR/bin"
mkdir -p "$BIN_DIR"

export GO111MODULE=on

pushd "$GOPLAY_DIR" >/dev/null
go mod download

log " - build: demos/stats/dgmain3"
go build -o "$BIN_DIR/dgmain3" ./demos/stats/dgmain3

if [[ -d "$GOPLAY_DIR/demos/signup/email" ]]; then
  log " - build: demos/signup/dgemail"
  go build -o "$BIN_DIR/dgemail" ./demos/signup/dgemail
else
  log " - skip: demos/signup/dgemail（缺少包：goPlay/demos/signup/email，对应 import tt_code/demos/signup/email）"
fi
popd >/dev/null

log "3/3 生成运行脚本（自动切换工作目录）"

cat > "$BIN_DIR/run_dgmain3.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR/goPlay/demos/stats/dgmain3"
exec "$ROOT_DIR/bin/dgmain3"
EOF

cat > "$BIN_DIR/run_dgemail.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR/goPlay/demos/signup/dgemail"
exec "$ROOT_DIR/bin/dgemail"
EOF

chmod +x "$BIN_DIR/run_dgmain3.sh" || true
if [[ -f "$BIN_DIR/dgemail" ]]; then
  chmod +x "$BIN_DIR/run_dgemail.sh" || true
else
  rm -f "$BIN_DIR/run_dgemail.sh" || true
fi

log "完成。"
log "Go 二进制：$BIN_DIR/dgmain3"
if [[ -f "$BIN_DIR/dgemail" ]]; then
  log "Go 二进制：$BIN_DIR/dgemail"
fi
log "运行：$BIN_DIR/run_dgmain3.sh"
if [[ -f "$BIN_DIR/dgemail" ]]; then
  log "运行：$BIN_DIR/run_dgemail.sh"
fi


