@echo off
setlocal enabledelayedexpansion

set "BAT_DIR=%~dp0"
cd /d "%BAT_DIR%"

echo -----------------------------------------
echo   ZPUI Build Script (Wails GUI)
echo   v2.12.0
echo -----------------------------------------
echo.

REM --- Find required tools ---
where go > nul 2>&1
if %errorlevel% neq 0 (
    echo [ERROR] Go not found in PATH. Install from: https://go.dev/dl/
    pause
    exit /b 1
)

where node > nul 2>&1
if %errorlevel% neq 0 (
    echo [ERROR] Node.js not found in PATH. Install from: https://nodejs.org/
    pause
    exit /b 1
)

REM Try to find wails: PATH, GOPATH/bin, user profile
set "WAILS="
where wails > nul 2>&1
if %errorlevel% equ 0 (
    for /f "delims=" %%A in ('where wails') do (
        set "WAILS=%%A"
        goto :wails_found
    )
)
if defined GOPATH if exist "%GOPATH%\bin\wails.exe" (
    set "WAILS=%GOPATH%\bin\wails.exe"
    goto :wails_found
)
if exist "%USERPROFILE%\go\bin\wails.exe" (
    set "WAILS=%USERPROFILE%\go\bin\wails.exe"
    goto :wails_found
)
if exist "C:\Users\Suzuc\go\bin\wails.exe" (
    set "WAILS=C:\Users\Suzuc\go\bin\wails.exe"
    goto :wails_found
)

echo [ERROR] wails CLI not found
echo Install: go install github.com/wailsapp/wails/v2/cmd/wails@latest
pause
exit /b 1

:wails_found
echo [INFO] Wails: %WAILS%
echo [INFO] Go:    %GOPATH%
echo.

REM --- Clean previous build ---
echo [1/4] Cleaning previous build...
if exist zpui.exe del /f /q zpui.exe > nul 2>&1
if exist build\bin\zpui.exe del /f /q build\bin\zpui.exe > nul 2>&1
if exist build\bin\config.json del /f /q build\bin\config.json > nul 2>&1
echo Done.

REM --- Build frontend ---
echo [2/4] Installing web dependencies...
pushd web
call npm install
if %errorlevel% neq 0 (
    popd
    echo [ERROR] npm install failed
    pause
    exit /b 1
)

echo [3/4] Building web interface...
call npm run build
if %errorlevel% neq 0 (
    popd
    echo [ERROR] Web build failed
    pause
    exit /b 1
)
popd

REM --- Build Go binary with Wails ---
echo [4/4] Building ZPUI via Wails...

"%WAILS%" build ^
    -platform windows/amd64 ^
    -s ^
    -skipbindings ^
    -skipembedcreate ^
    -o zpui.exe ^
    -ldflags "-s -w -H windowsgui -X main.version=1.0.0" ^
    -trimpath

if %errorlevel% neq 0 (
    echo [ERROR] Wails build failed
    pause
    exit /b 1
)

REM --- Copy to project root ---
copy /y build\bin\zpui.exe zpui.exe > nul

echo.
echo Build successful!
echo Output: %CD%\zpui.exe
echo.
for %%f in (zpui.exe) do echo   Size: %%~zf bytes
if exist build\bin\zpui.exe for %%f in (build\bin\zpui.exe) do echo   Built: %%~tf
echo.
echo Wails v2 GUI + System Tray
echo CGO_ENABLED=0, GOARCH=amd64
echo.
pause
