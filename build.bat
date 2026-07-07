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
echo   ZPUI Build System v%VERSION% (win32)
echo   Core + Modules + Zapret + Installer
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
echo [1/9] Cleaning old builds...

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
del /f /q "%BAT_DIR%dist\ZPUI-Setup-*.exe" 2>nul
echo Done.
echo.

REM === STEP 2: Build frontend ===
echo [2/9] Building frontend...
pushd web
call npm install --silent 2>nul
call npm run build
if errorlevel 1 (popd & echo [ERROR] Web build failed & timeout /t 10 > nul & exit /b 1)
popd
echo.

REM === STEP 3: Build main app (Wails, 32-bit) ===
echo [3/9] Building ZPUI core (win32)...
"%WAILS%" build -platform windows/386 -s -skipbindings -o zpui.exe ^
    -ldflags "-s -w -H windowsgui -X main.version=%VERSION%" -trimpath
if errorlevel 1 (echo [ERROR] Wails build failed & timeout /t 10 > nul & exit /b 1)
copy /y "build\bin\zpui.exe" "zpui.exe" > nul
echo.

REM === STEP 4: Build module exes (32-bit) ===
echo [4/9] Building module tools (win32)...
set "GOARCH=386"
set "LDFLAGS=-s -w -H windowsgui"

go build -o wizard.exe       -ldflags "%LDFLAGS%" -trimpath ./cmd/wizard/
if errorlevel 1 (echo [ERROR] wizard.exe build failed & timeout /t 10 > nul & exit /b 1)
echo   [OK] wizard.exe

go build -o autoselect.exe   -ldflags "%LDFLAGS%" -trimpath ./cmd/autoselect/
if errorlevel 1 (echo [ERROR] autoselect.exe build failed & timeout /t 10 > nul & exit /b 1)
echo   [OK] autoselect.exe

go build -o selfupdate.exe   -ldflags "%LDFLAGS%" -trimpath ./cmd/selfupdate/
if errorlevel 1 (echo [ERROR] selfupdate.exe build failed & timeout /t 10 > nul & exit /b 1)
echo   [OK] selfupdate.exe

go build -o zapretupdate.exe -ldflags "%LDFLAGS%" -trimpath ./cmd/zapretupdate/
if errorlevel 1 (echo [ERROR] zapretupdate.exe build failed & timeout /t 10 > nul & exit /b 1)
echo   [OK] zapretupdate.exe
echo.

REM === STEP 5: Assemble dist package ===
echo [5/9] Assembling dist package...
mkdir "%DIST%"

copy /y "zpui.exe"           "%DIST%\" > nul
copy /y "wizard.exe"         "%DIST%\" > nul
copy /y "autoselect.exe"     "%DIST%\" > nul
copy /y "selfupdate.exe"     "%DIST%\" > nul
copy /y "zapretupdate.exe"   "%DIST%\" > nul

REM --- Generate versions.json ---
powershell -NoProfile -Command "$v='%VERSION%'; function g($p){ if((Get-Content $p -Raw) -match 'var version\s*=\s*\x22([^\x22]+)\x22'){ $matches[1].Trim() } else { '0.0.0' } }; $wz=g '%BAT_DIR%cmd\wizard\main.go'; $as=g '%BAT_DIR%cmd\autoselect\main.go'; $su=g '%BAT_DIR%cmd\selfupdate\main.go'; $zu=g '%BAT_DIR%cmd\zapretupdate\main.go'; $j=[ordered]@{zpui=$v;wizard=$wz;autoselect=$as;selfupdate=$su;zapretupdate=$zu}|ConvertTo-Json; [IO.File]::WriteAllText('%DIST%\versions.json',$j,(New-Object Text.UTF8Encoding $false))"
if errorlevel 1 (echo [ERROR] versions.json generation failed & timeout /t 10 > nul & exit /b 1)

echo Done.
echo.

REM === STEP 6: Download latest Zapret ===
echo [6/9] Downloading latest Zapret...
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$ProgressPreference='SilentlyContinue';" ^
  "$api='https://api.github.com/repos/Flowseal/zapret-discord-youtube/releases/latest';" ^
  "try { $rel = Invoke-RestMethod -Uri $api -Headers @{'User-Agent'='ZPUI'} -TimeoutSec 30 } catch { Write-Host '[WARN] GitHub API failed:' $_.Exception.Message; exit 0 };" ^
  "$tag = $rel.tag_name;" ^
  "$zipName = 'zapret-discord-youtube-' + $tag + '.zip';" ^
  "$asset = $rel.assets | Where-Object { $_.name -eq $zipName } | Select-Object -First 1;" ^
  "if (-not $asset) { Write-Host '[WARN] Asset not found:' $zipName; exit 0 };" ^
  "$tmp = Join-Path $env:TEMP 'zapret-build-download.zip';" ^
  "Write-Host '  Downloading' $asset.name '('$tag')...';" ^
  "Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $tmp -UseBasicParsing;" ^
  "Add-Type -AssemblyName System.IO.Compression.FileSystem;" ^
  "$z = [System.IO.Compression.ZipFile]::OpenRead($tmp);" ^
  "$entries = $z.Entries | Where-Object { $_.FullName.TrimEnd('/\') -ne '' };" ^
  "$dirs = ($entries | ForEach-Object { ($_.FullName -split '[/\\]')[0] } | Sort-Object -Unique);" ^
  "if (($dirs | Measure-Object).Count -eq 1 -and ($entries | ForEach-Object { $_.FullName -split '[/\\]' }).Count -gt 1) {" ^
  "  $root = ($dirs | Select-Object -First 1);" ^
  "} else { $root = '' };" ^
  "$dest = '%DIST%\zapret';" ^
  "New-Item -ItemType Directory -Force -Path $dest | Out-Null;" ^
  "foreach ($e in $entries) {" ^
  "  $rel = $e.FullName;" ^
  "  if ($root -and $rel.StartsWith($root)) { $rel = $rel.Substring($root.Length) };" ^
  "  $rel = $rel.TrimStart('/\');" ^
  "  if (-not $rel) { continue };" ^
  "  $outPath = Join-Path $dest ($rel -replace '/','\');" ^
  "  $dir = Split-Path $outPath -Parent;" ^
  "  if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null };" ^
  "  if (-not $e.FullName.EndsWith('/') -and -not $e.FullName.EndsWith('\')) {" ^
  "    [System.IO.Compression.ZipFileExtensions]::ExtractToFile($e, $outPath, $true);" ^
  "  };" ^
  "};" ^
  "$z.Dispose();" ^
  "Remove-Item $tmp -Force;" ^
  "$verFile = Join-Path $dest '.service\version.txt';" ^
  "if (-not (Test-Path (Split-Path $verFile -Parent))) { New-Item -ItemType Directory -Force -Path (Split-Path $verFile -Parent) | Out-Null };" ^
  "Set-Content -Path $verFile -Value $tag.TrimStart('v') -NoNewline -Encoding UTF8;" ^
  "Write-Host ('  [OK] Zapret ' + $tag + ' extracted to dist\zapret\')"
if errorlevel 1 (
    echo [WARN] Zapret download failed, build will continue without it
) else (
    echo Done.
)
echo.

REM === STEP 7: Copy mods ===
echo [7/9] Copying mods...
if exist "%BAT_DIR%mods" (
    xcopy /e /i /y /q "%BAT_DIR%mods" "%DIST%\mods" > nul
    echo   [OK] mods copied
) else (
    mkdir "%DIST%\mods"
    echo   [INFO] No mods directory found, created empty
)
echo.

REM === STEP 8: Build installer (NSIS) ===
echo [8/9] Building installer...

REM --- Find makensis ---
set "MAKENSIS="
where makensis > nul 2>&1 && for /f "delims=" %%A in ('where makensis') do (set "MAKENSIS=%%A" & goto :nsis_found)
if exist "C:\Program Files (x86)\NSIS\makensis.exe" (set "MAKENSIS=C:\Program Files (x86)\NSIS\makensis.exe" & goto :nsis_found)
if exist "C:\Program Files\NSIS\makensis.exe" (set "MAKENSIS=C:\Program Files\NSIS\makensis.exe" & goto :nsis_found)
echo [WARN] NSIS not found, skipping installer
goto :nsis_skip

:nsis_found
echo [INFO] NSIS: %MAKENSIS%
for /f "tokens=1-3 delims=." %%a in ("%VERSION%") do set "VERSION_NUM=%%a.%%b.%%c"
"%MAKENSIS%" /DVERSION=%VERSION% /DVERSION_NUM=%VERSION_NUM% /DDIST="%BAT_DIR%build\dist" /DICON="%BAT_DIR%build\windows\icon.ico" /DOUTDIR="%BAT_DIR%build" /DLICENSE="%BAT_DIR%LICENSE" /DARCH=win32 installer\ZPUI.nsi
if errorlevel 1 (
    echo [ERROR] Installer build failed
) else (
    if exist "%BAT_DIR%build\ZPUI-Setup-%VERSION%-win32.exe" (
        echo   [OK] ZPUI-Setup-%VERSION%-win32.exe
    )
)
:nsis_skip
echo.

REM === STEP 9: Copy to dist/ for release ===
echo [9/9] Copying to dist/ for release...
if exist "%BAT_DIR%dist" rmdir /s /q "%BAT_DIR%dist"
mkdir "%BAT_DIR%dist"
xcopy /e /i /y /q "%DIST%" "%BAT_DIR%dist" > nul
if exist "%BAT_DIR%build\ZPUI-Setup-%VERSION%-win32.exe" (
    copy /y "%BAT_DIR%build\ZPUI-Setup-%VERSION%-win32.exe" "%BAT_DIR%dist\" > nul
)
echo Done.
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
if exist "%BAT_DIR%build\ZPUI-Setup-%VERSION%-win32.exe" (
    echo   Installer:
    for %%s in ("%BAT_DIR%build\ZPUI-Setup-%VERSION%-win32.exe") do echo     ZPUI-Setup-%VERSION%-win32.exe  %%~zs bytes
) else (
    echo   Installer: (not built - NSIS not found)
)
echo.
echo   Mods:
dir /b /ad "%DIST%\mods" 2>nul || echo     (none)
echo.
echo   Zapret:
if exist "%DIST%\zapret\bin\winws.exe" (
    echo     [OK] included
) else (
    echo     (not included)
)
echo.
echo   ZPUI v%VERSION% (win32) + 4 modules
echo ========================================
echo.
echo   Press any key to close...
timeout /t 5 /nobreak > nul
