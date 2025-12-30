@echo off
setlocal
cd /d "%~dp0"

rem 关键点：不要用 go run main.go（只编译单文件）
go run .

endlocal


