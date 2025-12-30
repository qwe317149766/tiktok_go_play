$ErrorActionPreference = "Stop"

Write-Host "=== ONE-CLICK RUN (Windows) ==="

# 0) 进入脚本所在目录（仓库根目录）
$ROOT = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $ROOT

# 1) Python 注册设备（写入 Redis 设备池）
# 注意：mwzzzh_spider.py 会检查 proxies.txt（为空会退出）
Write-Host "`n[1/3] mwzzzh_spider.py"
python .\mwzzzh_spider.py

# 2) Go signup(startUp)（从 Redis 读取设备池，注册并写入 cookies 池）
Write-Host "`n[2/3] goPlay/demos/signup/dgemail"
Set-Location .\goPlay\demos\signup\dgemail
if (Test-Path .\dgemail.exe) {
  .\dgemail.exe
} else {
  go run .
}

# 3) Go stats（从 Redis 读取设备池 + startUp cookies 池，执行播放/统计）
Write-Host "`n[3/3] goPlay/demos/stats/dgmain3"
Set-Location ..\..\stats\dgmain3
go run .


