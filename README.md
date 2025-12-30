# 一键启动（根目录）

> 说明：这套一键启动会依次执行 **2 个环节**：
> - Python 注册设备（将注册成功的设备写入 Redis 设备池）
> - Go signup(startUp) 注册（写入 startUp cookies 池）
> - Go stats 播放（从 Redis 读取设备池 + startUp cookies 池）

## Windows

在仓库根目录 PowerShell 执行：

```powershell
.\run_all_windows.ps1
```

## Linux

在仓库根目录执行：

```bash
chmod +x ./run_all_linux.sh
./run_all_linux.sh
```

"# tiktok_go_play" 
