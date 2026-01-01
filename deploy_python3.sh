#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

echo "=== PYTHON3 ONE-CLICK DEPLOY ==="

# 0) 载入 env（可选）
ENV_FILE="${ENV_FILE:-}"
if [[ -n "${ENV_FILE}" && -f "${ENV_FILE}" ]]; then
  echo "[env] loading: ${ENV_FILE}"
  set -a
  # shellcheck disable=SC1090
  source "${ENV_FILE}"
  set +a
elif [[ -f "./.env.linux" ]]; then
  echo "[env] loading: ./.env.linux"
  set -a
  # shellcheck disable=SC1091
  source "./.env.linux"
  set +a
elif [[ -f "./env.linux" ]]; then
  echo "[env] loading: ./env.linux"
  set -a
  # shellcheck disable=SC1091
  source "./env.linux"
  set +a
else
  echo "[env] skip (no ENV_FILE/.env.linux/env.linux)"
fi

# 1) Python3 检查
PY="${PYTHON:-python3}"
if ! command -v "${PY}" >/dev/null 2>&1; then
  echo "ERROR: 找不到 ${PY}。请先安装 Python3（建议 3.10+）" >&2
  exit 1
fi

echo "[py] $(${PY} --version 2>/dev/null || true)"

# 2) venv
VENV_DIR="${VENV_DIR:-.venv}"
if [[ ! -d "${VENV_DIR}" ]]; then
  echo "[venv] create: ${VENV_DIR}"
  "${PY}" -m venv "${VENV_DIR}"
else
  echo "[venv] reuse: ${VENV_DIR}"
fi

# shellcheck disable=SC1091
source "${VENV_DIR}/bin/activate"

# 3) pip 升级 + 安装依赖
REQ_FILE="${REQ_FILE:-requirements.txt}"
if [[ ! -f "${REQ_FILE}" ]]; then
  echo "ERROR: 缺少 ${REQ_FILE}，无法安装依赖" >&2
  exit 1
fi

python -m pip install --upgrade pip setuptools wheel
python -m pip install -r "${REQ_FILE}"

echo
echo "✅ 部署完成（venv=${VENV_DIR}）"
echo
echo "常用命令："
echo "  source ${VENV_DIR}/bin/activate"
echo "  python mwzzzh_spider.py"
echo "  python clear_startup_cookies_redis.py --yes"
echo "  python export_startup_cookies_to_file.py --out startup_accounts.jsonl --mode account"





