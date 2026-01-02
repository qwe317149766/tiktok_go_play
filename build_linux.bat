@echo off
setlocal enabledelayedexpansion

:: 1. Force switch to script root
cd /d "%~dp0"
set "ROOT_DIR=%cd%"
set "BIN_DIR=%ROOT_DIR%\bin"

echo ==========================================
echo Starting Cross-Compilation for Linux (AMD64)
echo Mode: Absolute Directory Build
echo ==========================================

if not exist "%BIN_DIR%" (
    echo Creating bin directory...
    mkdir "%BIN_DIR%"
)

set GOOS=linux
set GOARCH=amd64
set CGO_ENABLED=0

:: Check Go environment
echo Debug: Checking Go version...
go version
if %ERRORLEVEL% NEQ 0 (
    echo ❌ FATAL: 'go' command not found. Please install Go.
    pause
    exit /b 1
)

echo.
echo [1/2] Building Stats (dgmain3)...
:: Navigate directly to source directory
if exist "goPlay\demos\stats\dgmain3" (
    cd "goPlay\demos\stats\dgmain3"
) else (
    echo ❌ Source directory 'goPlay\demos\stats\dgmain3' not found!
    pause
    exit /b 1
)

echo Current Dir: !cd!
echo Compiling...
go build -ldflags "-s -w" -o "%BIN_DIR%\stats_linux" .
if %ERRORLEVEL% NEQ 0 (
    echo ❌ Stats build failed!
    cd "%ROOT_DIR%"
    pause
    exit /b %ERRORLEVEL%
)
echo ✅ Success.

echo.
echo [2/2] Building Signup (dgemail)...
cd "%ROOT_DIR%"
if exist "goPlay\demos\signup\dgemail" (
    cd "goPlay\demos\signup\dgemail"
) else (
    echo ❌ Source directory 'goPlay\demos\signup\dgemail' not found!
    pause
    exit /b 1
)

echo Current Dir: !cd!
echo Compiling...
go build -ldflags "-s -w" -o "%BIN_DIR%\signup_linux" .
if %ERRORLEVEL% NEQ 0 (
    echo ❌ Signup build failed!
    cd "%ROOT_DIR%"
    pause
    exit /b %ERRORLEVEL%
)
echo ✅ Success.

echo.
echo [3/3] Building API Server (api_server)...
cd "%ROOT_DIR%"
if exist "api_server" (
    cd "api_server"
) else (
    echo ❌ Source directory 'api_server' not found!
    pause
    exit /b 1
)

echo Current Dir: !cd!
echo Compiling...
go build -ldflags "-s -w" -o "%BIN_DIR%\api_server_linux" .
if %ERRORLEVEL% NEQ 0 (
    echo ❌ API Server build failed!
    cd "%ROOT_DIR%"
    pause
    exit /b %ERRORLEVEL%
)
echo ✅ Success.

echo.


echo.
echo ==========================================
echo All builds completed successfully!
echo Binaries: %BIN_DIR%
echo ==========================================

cd "%ROOT_DIR%"
pause
