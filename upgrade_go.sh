#!/bin/bash

# -------------------------------
# 自动升级 Go 到 1.25
# -------------------------------

GO_VERSION="1.25.0"
GO_URL="https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
INSTALL_DIR="/usr/local/go"

echo "==============================="
echo "开始升级 Go 到版本 $GO_VERSION"
echo "==============================="

# 1️⃣ 删除旧版本
if [ -d "$INSTALL_DIR" ]; then
    echo "检测到旧版本 Go，正在删除..."
    sudo rm -rf $INSTALL_DIR
fi

# 2️⃣ 下载 Go 1.25
echo "下载 Go $GO_VERSION..."
wget -O /tmp/go${GO_VERSION}.linux-amd64.tar.gz $GO_URL
if [ $? -ne 0 ]; then
    echo "下载失败，请检查网络！"
    exit 1
fi

# 3️⃣ 解压到 /usr/local
echo "解压 Go..."
sudo tar -C /usr/local -xzf /tmp/go${GO_VERSION}.linux-amd64.tar.gz

# 4️⃣ 配置环境变量
echo "配置 PATH 环境变量..."
if ! grep -q 'export PATH=/usr/local/go/bin:$PATH' ~/.bashrc; then
    echo 'export PATH=/usr/local/go/bin:$PATH' >> ~/.bashrc
fi
export PATH=/usr/local/go/bin:$PATH

# 5️⃣ 验证
echo "==============================="
echo "Go 版本："
go version
if [ $? -eq 0 ]; then
    echo "Go 已成功升级到 $GO_VERSION"
else
    echo "Go 升级失败，请检查安装过程"
fi
echo "==============================="

# 6️⃣ 可选：设置国内代理
read -p "是否设置国内 Go 模块代理（goproxy.cn）？[y/n] " yn
if [ "$yn" = "y" ]; then
    if ! grep -q 'export GOPROXY=https://goproxy.cn,direct' ~/.bashrc; then
        echo 'export GOPROXY=https://goproxy.cn,direct' >> ~/.bashrc
    fi
    export GOPROXY=https://goproxy.cn,direct
    echo "Go 模块代理已设置为 goproxy.cn"
fi

echo "升级完成！请重新登录终端使 PATH 永久生效"
