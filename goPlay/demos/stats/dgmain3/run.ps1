$ErrorActionPreference = "Stop"

Set-Location $PSScriptRoot

# 关键点：
# - 不要用 `go run main.go`（只编译单文件）
# - 用 `go run .` 编译并运行当前目录整个 package
go run .


