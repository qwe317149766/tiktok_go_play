## Linux 一键部署（Go + Python）

本仓库同时包含：
- Go 代码：`goPlay/`（module 名为 `tt_code`）
- Python 代码：仓库根目录下（依赖见 `requirements.txt`）

### 依赖要求

- **Python**: `python3`, `pip3`
- **Go**: `go`（建议 1.20+；你当前 Windows 环境是 1.25.x 也可）

### 一键部署

在仓库根目录执行：

```bash
chmod +x deploy_linux.sh
./deploy_linux.sh
```

脚本会做三件事：
- 创建 `.venv/` 并安装 `requirements.txt`
- 在 `goPlay/` 内编译 Go 程序到 `./bin/`
- 生成运行脚本 `./bin/run_dgmain3.sh`、`./bin/run_dgemail.sh`

> 注意：当前仓库里 `goPlay/demos/signup/dgemail` 依赖 `tt_code/demos/signup/email`，但该包未在仓库中提供。
> 因此脚本会 **默认编译 dgmain3**，并对 dgemail **自动跳过并提示**（除非你后续把 `goPlay/demos/signup/email` 补齐）。

### 一键运行

```bash
./bin/run_dgmain3.sh
```

或：

```bash
./bin/run_dgemail.sh
```

### 常见问题

#### Q: 为什么要用 run_*.sh？
因为 demo 里有不少文件使用相对路径（例如 `proxies.txt/devices.txt`），运行脚本会先 `cd` 到对应目录，避免找不到文件。

#### Q: pip 安装冲突怎么办？
请使用脚本创建的 `.venv` 虚拟环境，避免与系统 Python 其它包冲突。


