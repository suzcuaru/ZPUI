@echo off
chcp 65001 > nul
echo ========================================
echo   ZPUI Build Script
echo ========================================
echo:

where go > nul 2>&1
if %errorlevel% neq 0 (
    echo [ERROR] Go is not installed or not in PATH
    echo Download from: https://go.dev/dl/
    pause
    exit /b 1
)

where node > nul 2>&1
if %errorlevel% neq 0 (
    echo [ERROR] Node.js is not installed or not in PATH
    echo Download from: https://nodejs.org/
    pause
    exit /b 1
)

echo [1/5] Installing web dependencies...
cd web
call npm install
if %errorlevel% neq 0 (
    echo [ERROR] npm install failed
    cd ..
    pause
    exit /b 1
)

echo [2/5] Building web interface...
call npm run build
if %errorlevel% neq 0 (
    echo [ERROR] Web build failed
    cd ..
    pause
    exit /b 1
)
cd ..

echo [3/5] Downloading Go dependencies...
go mod tidy
if %errorlevel% neq 0 (
    echo [ERROR] Failed to download dependencies
    pause
    exit /b 1
)

echo [4/5] Building ZPUI...
set PATH=C:\msys64\mingw32\bin;%PATH%
set CGO_ENABLED=1
set CC=gcc
go build -ldflags="-s -w -H windowsgui -X main.version=1.0.0" -o zpui.exe .
if %errorlevel% neq 0 (
    echo [ERROR] Build failed
    pause
    exit /b 1
)

echo [5/5] Build successful!
echo Output: zpui.exe
echo:
echo You can now run zpui.exe
echo Place it in the same folder as your zapret installation
echo Or configure the path in the web UI.
echo:
pause
