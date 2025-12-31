#!/bin/bash
set -e

############################
# 可配置项
############################

GO_VERSION="1.22.1"
INSTALL_DIR="/usr/local"
PROFILE_FILE="/etc/profile.d/go.sh"
GOPATH_DIR="/opt/go"

############################
# 必须使用 root
############################

if [ "$EUID" -ne 0 ]; then
    echo "❌ 请使用 root 或 sudo 运行此脚本"
    exit 1
fi

############################
# 安装基础依赖
############################

echo "==> 安装基础工具"
apt update -y
apt install -y curl wget git tar ca-certificates

############################
# 检测架构
############################

ARCH=$(uname -m)
case $ARCH in
    x86_64) GO_ARCH="amd64" ;;
    aarch64) GO_ARCH="arm64" ;;
    *)
        echo "❌ 不支持的架构: $ARCH"
        exit 1
        ;;
esac

############################
# 检测是否已安装同版本
############################

if [ -x "${INSTALL_DIR}/go/bin/go" ]; then
    INSTALLED_VERSION=$(${INSTALL_DIR}/go/bin/go version | awk '{print $3}' | sed 's/go//')
    if [ "$INSTALLED_VERSION" = "$GO_VERSION" ]; then
        echo "✅ Go ${GO_VERSION} 已安装，跳过安装步骤"
    else
        echo "⚠️ 检测到已安装 Go ${INSTALLED_VERSION}，将升级到 ${GO_VERSION}"
    fi
fi

############################
# 下载 Go
############################

GO_TAR="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
GO_URL="https://go.dev/dl/${GO_TAR}"

echo "==> 下载 Go ${GO_VERSION} (${GO_ARCH})"
cd /tmp
rm -f ${GO_TAR}

wget -q --show-progress ${GO_URL}
if [ ! -f "${GO_TAR}" ]; then
    echo "❌ Go 下载失败"
    exit 1
fi

############################
# 安装 Go
############################

echo "==> 安装 Go 到 ${INSTALL_DIR}/go"
rm -rf ${INSTALL_DIR}/go
tar -C ${INSTALL_DIR} -xzf ${GO_TAR}
rm -f ${GO_TAR}

############################
# 配置环境变量（全局）
############################

echo "==> 配置环境变量"

cat > ${PROFILE_FILE} <<EOF
# Go environment
export GOROOT=${INSTALL_DIR}/go
export GOPATH=${GOPATH_DIR}
export PATH=\$GOROOT/bin:\$GOPATH/bin:\$PATH

# Go Modules
export GO111MODULE=on
export GOPROXY=https://goproxy.cn,direct
EOF

chmod 644 ${PROFILE_FILE}

############################
# 兼容 root 非登录 shell
############################

if ! grep -q "/etc/profile.d/go.sh" /root/.bashrc 2>/dev/null; then
    echo "source /etc/profile.d/go.sh" >> /root/.bashrc
fi

############################
# 创建 GOPATH
############################

mkdir -p ${GOPATH_DIR}/{bin,pkg,src}
chmod -R 755 ${GOPATH_DIR}

############################
# 立即生效（当前 shell）
############################

source /etc/profile
source ${PROFILE_FILE}

############################
# 验证安装
############################

echo "==> 验证 Go 安装"
echo "GOROOT=$(go env GOROOT)"
echo "GOPATH=$(go env GOPATH)"
echo "PATH=$PATH"
go version
go env GOPROXY

echo "🎉 Go ${GO_VERSION} 安装完成（环境变量已全局生效）"
