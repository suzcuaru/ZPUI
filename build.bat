@echo off
cd /d "%~dp0"
set APP_NAME=zpui
set PLATFORM=windows/amd64

if /i "%1"=="release" (
    set DEVTOOLS=-s -trimpath
    echo [BUILD] Release build
) else if /i "%1"=="clean" (
    if exist "build\bin" rmdir /s /q "build\bin"
    if exist "web\dist" rmdir /s /q "web\dist"
    echo [BUILD] Cleaned
    exit /b 0
) else (
    set DEVTOOLS=-devtools
    echo [BUILD] DevTools build (default)
)

if not exist "build\bin" mkdir "build\bin"

echo [BUILD] Frontend...
cd web
call npm run build
if %errorlevel% neq 0 (
    echo [BUILD] Frontend error
    cd ..
    exit /b 1
)
cd ..

echo [BUILD] Backend (%PLATFORM%)...
set GOARCH=amd64
set GOOS=windows
wails build -platform %PLATFORM% %DEVTOOLS%
if %errorlevel% neq 0 (
    echo [BUILD] Build error
    exit /b 1
)

echo [BUILD] Done: build\bin\%APP_NAME%.exe
exit /b 0
