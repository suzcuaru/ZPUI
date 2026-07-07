@echo off
setlocal enabledelayedexpansion

set "BAT_DIR=%~dp0"
cd /d "%BAT_DIR%"

REM --- Auto-increment version ---
if exist "%BAT_DIR%version.txt" (
    set /p OLD_VER=<"%BAT_DIR%version.txt"
) else (
    set "OLD_VER=1.0.0"
)
for /f "tokens=1-3 delims=." %%a in ("%OLD_VER%") do (
    set /a "PATCH=%%c+1"
    set "VERSION=%%a.%%b.!PATCH!"
)
echo %VERSION%>"%BAT_DIR%version.txt"
echo [INFO] Version updated: %OLD_VER% ^> %VERSION%

REM --- Sync wails.json productVersion ---
powershell -NoProfile -Command "& {$p='%BAT_DIR%wails.json'; $c=Get-Content $p -Raw | ConvertFrom-Json; $c.info.productVersion='%VERSION%'; $enc=New-Object System.Text.UTF8Encoding $false; [System.IO.File]::WriteAllText($p, ($c | ConvertTo-Json -Depth 10), $enc)}" > nul 2>&1

set "DIST=%BAT_DIR%build\dist"

echo ========================================
echo   ZPUI Build System v%VERSION%
echo   Core + Modules
echo ========================================
echo.

REM --- Find required tools ---
where go > nul 2>&1 || (echo [ERROR] Go not found & timeout /t 10 > nul & exit /b 1)
where node > nul 2>&1 || (echo [ERROR] Node.js not found & timeout /t 10 > nul & exit /b 1)

REM --- Find wails ---
set "WAILS="
where wails > nul 2>&1 && for /f "delims=" %%A in ('where wails') do (set "WAILS=%%A" & goto :wails_found)
if defined GOPATH if exist "%GOPATH%\bin\wails.exe" (set "WAILS=%GOPATH%\bin\wails.exe" & goto :wails_found)
if exist "%USERPROFILE%\go\bin\wails.exe" (set "WAILS=%USERPROFILE%\go\bin\wails.exe" & goto :wails_found)
echo [ERROR] wails CLI not found & timeout /t 10 > nul & exit /b 1
:wails_found
echo [INFO] Wails: %WAILS%
echo.

REM === STEP 1: Clean ===
echo [1/7] Cleaning old builds...

REM Kill running ZPUI processes first
taskkill /IM zpui.exe /F > nul 2>&1
taskkill /IM wizard.exe /F > nul 2>&1
taskkill /IM autoselect.exe /F > nul 2>&1
taskkill /IM selfupdate.exe /F > nul 2>&1
taskkill /IM zapretupdate.exe /F > nul 2>&1
timeout /t 1 /nobreak > nul

if exist "%DIST%" rmdir /s /q "%DIST%"
if exist "%BAT_DIR%build\bin\zpui.exe" del /f /q "%BAT_DIR%build\bin\zpui.exe"
del /f /q "%BAT_DIR%zpui.exe" 2>nul
del /f /q "%BAT_DIR%wizard.exe" 2>nul
del /f /q "%BAT_DIR%autoselect.exe" 2>nul
del /f /q "%BAT_DIR%selfupdate.exe" 2>nul
del /f /q "%BAT_DIR%zapretupdate.exe" 2>nul
echo Done.
echo.

REM === STEP 2: Build frontend ===
echo [2/7] Building frontend...
pushd web
call npm install --silent 2>nul
call npm run build
if errorlevel 1 (popd & echo [ERROR] Web build failed & timeout /t 10 > nul & exit /b 1)
popd
echo.

REM === STEP 3: Build main app (Wails) ===
echo [3/7] Building ZPUI core...
"%WAILS%" build -platform windows/amd64 -s -skipbindings -o zpui.exe ^
    -ldflags "-s -w -H windowsgui -X main.version=%VERSION%" -trimpath
if errorlevel 1 (echo [ERROR] Wails build failed & timeout /t 10 > nul & exit /b 1)
copy /y "build\bin\zpui.exe" "zpui.exe" > nul
echo.

REM === STEP 4: Build module exes ===
echo [4/7] Building module tools...

go build -o wizard.exe       -ldflags "-s -w -H windowsgui" -trimpath ./cmd/wizard/
if errorlevel 1 (echo [ERROR] wizard.exe build failed & timeout /t 10 > nul & exit /b 1)
echo   [OK] wizard.exe

go build -o autoselect.exe   -ldflags "-s -w -H windowsgui" -trimpath ./cmd/autoselect/
if errorlevel 1 (echo [ERROR] autoselect.exe build failed & timeout /t 10 > nul & exit /b 1)
echo   [OK] autoselect.exe

go build -o selfupdate.exe   -ldflags "-s -w -H windowsgui" -trimpath ./cmd/selfupdate/
if errorlevel 1 (echo [ERROR] selfupdate.exe build failed & timeout /t 10 > nul & exit /b 1)
echo   [OK] selfupdate.exe

go build -o zapretupdate.exe -ldflags "-s -w -H windowsgui" -trimpath ./cmd/zapretupdate/
if errorlevel 1 (echo [ERROR] zapretupdate.exe build failed & timeout /t 10 > nul & exit /b 1)
echo   [OK] zapretupdate.exe
echo.

REM === STEP 5: Assemble dist package ===
echo [5/7] Assembling dist package...
mkdir "%DIST%"

copy /y "zpui.exe"           "%DIST%\" > nul
copy /y "wizard.exe"         "%DIST%\" > nul
copy /y "autoselect.exe"     "%DIST%\" > nul
copy /y "selfupdate.exe"     "%DIST%\" > nul
copy /y "zapretupdate.exe"   "%DIST%\" > nul

REM --- Generate versions.json (module versions read from source code) ---
powershell -NoProfile -Command "$v='%VERSION%'; function g($p){ if((Get-Content $p -Raw) -match 'var version\s*=\s*\x22([^\x22]+)\x22'){ $matches[1].Trim() } else { '0.0.0' } }; $wz=g '%BAT_DIR%cmd\wizard\main.go'; $as=g '%BAT_DIR%cmd\autoselect\main.go'; $su=g '%BAT_DIR%cmd\selfupdate\main.go'; $zu=g '%BAT_DIR%cmd\zapretupdate\main.go'; $j=[ordered]@{zpui=$v;wizard=$wz;autoselect=$as;selfupdate=$su;zapretupdate=$zu}|ConvertTo-Json; [IO.File]::WriteAllText('%DIST%\versions.json',$j,(New-Object Text.UTF8Encoding $false))"
if errorlevel 1 (echo [ERROR] versions.json generation failed & timeout /t 10 > nul & exit /b 1)

echo Done.
echo.

REM === STEP 6: Copy mods ===
echo [6/7] Copying mods...
if exist "%BAT_DIR%mods" (
    xcopy /e /i /y /q "%BAT_DIR%mods" "%DIST%\mods" > nul
    echo   [OK] mods copied
) else (
    mkdir "%DIST%\mods"
    echo   [INFO] No mods directory found, created empty
)
echo.

REM === STEP 7: Done ===
echo [7/7] Build complete!
echo.

REM --- Summary ---
echo ========================================
echo   Output: %DIST%\
echo.
echo   Core:
for %%f in ("%DIST%\*.exe") do (
    for %%s in ("%%f") do echo     %%~nxf  %%~zs bytes
)
echo   versions.json
echo.
echo   Mods:
dir /b /ad "%DIST%\mods" 2>nul || echo     (none)
echo.
echo   ZPUI v%VERSION% + 4 modules
echo ========================================
echo.
echo   Press any key to close...
timeout /t 5 /nobreak > nul
