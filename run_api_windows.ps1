$ErrorActionPreference = "Stop"

$ROOT = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $ROOT

Write-Host "=== GO API SERVER (Windows) ==="
Set-Location .\api_server

go mod tidy
go run .


