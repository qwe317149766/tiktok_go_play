#!/bin/bash
set -e

############################
# 可配置项
############################

GO_VERSION="1.22.1"
INSTALL_DIR="/usr/local"
PROFILE_FILE="/etc/profile.d/go.sh"

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
apt install -y curl wget git tar

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
# 下载 Go
############################

GO_TAR="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
GO_URL="https://go.dev/dl/${GO_TAR}"

echo "==> 下载 Go ${GO_VERSION} (${GO_ARCH})"
cd /tmp
rm -f ${GO_TAR}
wget -q ${GO_URL}

############################
# 安装 Go
############################

echo "==> 安装 Go 到 ${INSTALL_DIR}/go"
rm -rf ${INSTALL_DIR}/go
tar -C ${INSTALL_DIR} -xzf ${GO_TAR}

############################
# 配置环境变量
############################

echo "==> 配置环境变量"

cat > ${PROFILE_FILE} <<EOF
# Go environment
export GOROOT=${INSTALL_DIR}/go
export GOPATH=/opt/go
export PATH=\$PATH:\$GOROOT/bin:\$GOPATH/bin

# Go Modules
export GO111MODULE=on
export GOPROXY=https://goproxy.cn,direct
EOF

chmod +x ${PROFILE_FILE}

############################
# 创建 GOPATH
############################

mkdir -p /opt/go/{bin,pkg,src}
chmod -R 755 /opt/go

############################
# 立即生效
############################

source ${PROFILE_FILE}

############################
# 验证安装
############################

echo "==> 验证 Go 安装"
go version
go env GOPROXY

echo "✅ Go ${GO_VERSION} 安装完成"
