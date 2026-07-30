@echo off
setlocal enabledelayedexpansion

set "BAT_DIR=%~dp0"
cd /d "%BAT_DIR%"

REM === Read ALL versions from versions.txt (NO auto-increment) ===
set "VERSION="
set "VER_SELFUPDATE="
set "VER_REPORT="
set "VER_SECURITY="

if not exist "%BAT_DIR%versions.txt" (
    echo [ERROR] versions.txt not found
    timeout /t 10 > nul
    exit /b 1
)

for /f "usebackq tokens=1,* delims==" %%a in ("%BAT_DIR%versions.txt") do (
    set "key=%%a"
    set "val=%%b"
    set "key=!key: =!"
    set "val=!val: =!"
    if "!key!"=="zpui" set "VERSION=!val!"
    if "!key!"=="selfupdate" set "VER_SELFUPDATE=!val!"
    if "!key!"=="report" set "VER_REPORT=!val!"
    if "!key!"=="security" set "VER_SECURITY=!val!"
)

if not defined VERSION (
    echo [ERROR] zpui version not found in versions.txt
    timeout /t 10 > nul
    exit /b 1
)

if not defined VER_SELFUPDATE set "VER_SELFUPDATE=3.0.4"
if not defined VER_REPORT set "VER_REPORT=1.0.0"
if not defined VER_SECURITY set "VER_SECURITY=1.0.0"

echo [INFO] Versions from versions.txt:
echo   ZPUI:       %VERSION%
echo   SelfUpdate: %VER_SELFUPDATE%
echo   Report:     %VER_REPORT%
echo   Security:   %VER_SECURITY%

REM --- Prepare build log (placeholder; real init after clean) ---
set "BUILDLOG=%BAT_DIR%dist\build-%VERSION%.log"

REM --- Sync wails.json productVersion ---
powershell -NoProfile -Command "& {$p='%BAT_DIR%wails.json'; $c=Get-Content $p -Raw | ConvertFrom-Json; $c.info.productVersion='%VERSION%'; $enc=New-Object System.Text.UTF8Encoding $false; [System.IO.File]::WriteAllText($p, ($c | ConvertTo-Json -Depth 10), $enc)}" > nul 2>&1
powershell -NoProfile -Command "& {$p='%BAT_DIR%cmd\selfupdate\wails.json'; $c=Get-Content $p -Raw | ConvertFrom-Json; $c.info.productVersion='%VER_SELFUPDATE%'; $enc=New-Object System.Text.UTF8Encoding $false; [System.IO.File]::WriteAllText($p, ($c | ConvertTo-Json -Depth 10), $enc)}" > nul 2>&1

set "DIST=%BAT_DIR%dist"
set "VERDIR=%DIST%\%VERSION%"

echo ========================================
echo   ZPUI Build System v%VERSION% (win32)
echo   Core + Modules + Installer
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

REM --- Find goversioninfo ---
set "GOVI="
where goversioninfo > nul 2>&1 && for /f "delims=" %%A in ('where goversioninfo') do (set "GOVI=%%A")
if not defined GOVI if exist "%USERPROFILE%\go\bin\goversioninfo.exe" set "GOVI=%USERPROFILE%\go\bin\goversioninfo.exe"
if not defined GOVI if defined GOPATH if exist "%GOPATH%\bin\goversioninfo.exe" set "GOVI=%GOPATH%\bin\goversioninfo.exe"
if not defined GOVI (
    echo [INFO] Installing goversioninfo...
    go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest 2>nul
    if exist "%USERPROFILE%\go\bin\goversioninfo.exe" set "GOVI=%USERPROFILE%\go\bin\goversioninfo.exe"
)
if defined GOVI (echo [INFO] goversioninfo: %GOVI%) else (echo [WARN] goversioninfo not found, version resources will be skipped)
echo.

REM === STEP 1: Clean ===
echo [1/10] Cleaning old builds...

taskkill /IM ZPUI.exe /F > nul 2>&1
taskkill /IM selfupdate.exe /F > nul 2>&1
taskkill /IM report.exe /F > nul 2>&1
taskkill /IM security.exe /F > nul 2>&1
timeout /t 1 /nobreak > nul

if exist "%DIST%" rmdir /s /q "%DIST%"
if exist "%BAT_DIR%build\bin\ZPUI.exe" del /f /q "%BAT_DIR%build\bin\ZPUI.exe"
del /f /q "%BAT_DIR%build\ZPUI-Setup-*.exe" 2>nul
mkdir "%VERDIR%"

REM Initialize build log (after dist cleanup, so it's not deleted)
echo ================================================ > "%BUILDLOG%"
echo   ZPUI Build Log  v%VERSION%                     >> "%BUILDLOG%"
echo   %DATE% %TIME%                                   >> "%BUILDLOG%"
echo ================================================ >> "%BUILDLOG%"
echo. >> "%BUILDLOG%"
echo Versions: ZPUI=%VERSION% selfupdate=%VER_SELFUPDATE% report=%VER_REPORT% security=%VER_SECURITY% >> "%BUILDLOG%"
echo. >> "%BUILDLOG%"
echo [1/10] Cleaning old builds... >> "%BUILDLOG%"
echo Done. >> "%BUILDLOG%"
echo.

REM === STEP 2: Build frontend ===
echo [2/10] Building frontend...
echo [2/10] Frontend >> "%BUILDLOG%"
pushd web
call npm install --silent 2>nul
call npm run build >> "%BUILDLOG%" 2>&1
if errorlevel 1 (popd & echo [ERROR] Web build failed >> "%BUILDLOG%" & echo [ERROR] Web build failed & timeout /t 10 > nul & exit /b 1)
echo Web build OK >> "%BUILDLOG%"
popd

pushd cmd\selfupdate\frontend
call npm install --silent 2>nul
call npm run build >> "%BUILDLOG%" 2>&1
if errorlevel 1 (popd & echo [ERROR] Selfupdate frontend build failed >> "%BUILDLOG%" & echo [ERROR] Selfupdate frontend build failed & timeout /t 10 > nul & exit /b 1)
echo Selfupdate frontend build OK >> "%BUILDLOG%"
popd
echo.

REM === STEP 3: Generate version info syso files ===
echo [3/10] Generating version resources...
echo [3/10] Version resources >> "%BUILDLOG%"

REM Parse ZPUI version into major.minor.patch
for /f "tokens=1-3 delims=." %%a in ("%VERSION%") do (
    set "PROD_MAJ=%%a"
    set "PROD_MIN=%%b"
    set "PROD_PAT=%%c"
)

if defined GOVI (
    call :gen_syso selfupdate   "%VER_SELFUPDATE%"   "ZPUI Updater"
    call :gen_syso report       "%VER_REPORT%"       "ZPUI Report Generator"
    call :gen_syso security     "%VER_SECURITY%"     "ZPUI Security"
) else (
    echo   [WARN] Skipping version resources
)
echo.

REM === STEP 4: Build main app (Wails, 32-bit) ===
echo [4/10] Building ZPUI core (win32)...
echo [4/10] ZPUI core >> "%BUILDLOG%"
"%WAILS%" build -platform windows/386 -s -skipbindings -o ZPUI.exe ^
    -ldflags "-s -w -H windowsgui -X main.version=%VERSION%" -trimpath
if errorlevel 1 (echo [ERROR] Wails build failed >> "%BUILDLOG%" & echo [ERROR] Wails build failed & timeout /t 10 > nul & exit /b 1)
copy /y "build\bin\ZPUI.exe" "%VERDIR%\ZPUI.exe" > nul
echo ZPUI.exe built >> "%BUILDLOG%"
echo.

REM === STEP 5: Build module exes (32-bit) ===
echo [5/10] Building module tools (win32)...
echo [5/10] Module tools >> "%BUILDLOG%"
set "GOARCH=386"

REM --- selfupdate is a Wails mini-app: build via wails CLI ---
if not exist "cmd\selfupdate\build\windows\icon.ico" (
    copy /y "%BAT_DIR%build\windows\icon.ico" "cmd\selfupdate\build\windows\icon.ico" > nul
)
pushd cmd\selfupdate
"%WAILS%" build -platform windows/386 -s -skipbindings -o selfupdate.exe ^
    -ldflags "-s -w -H windowsgui -X main.version=%VER_SELFUPDATE%" -trimpath
if errorlevel 1 (popd & echo [ERROR] selfupdate wails build failed >> "%BUILDLOG%" & echo [ERROR] selfupdate wails build failed & timeout /t 10 > nul & exit /b 1)
popd

REM --- Create module directories and copy files ---
mkdir "%VERDIR%\components\selfupdate"
copy /y "cmd\selfupdate\build\bin\selfupdate.exe" "%VERDIR%\components\selfupdate\selfupdate.exe" > nul
REM Also place alongside exe for backward compatibility
copy /y "cmd\selfupdate\build\bin\selfupdate.exe" "%VERDIR%\selfupdate.exe" > nul
echo   [OK] selfupdate.exe v%VER_SELFUPDATE% >> "%BUILDLOG%"
echo   [OK] selfupdate.exe v%VER_SELFUPDATE%

REM Build and copy report
mkdir "%VERDIR%\components\report"
go build -o "%VERDIR%\components\report\report.exe" -ldflags "-s -w -H windowsgui -X main.version=%VER_REPORT%" -trimpath ./cmd/report/
if errorlevel 1 (
    echo [WARN] report.exe build failed, skipping >> "%BUILDLOG%"
    echo [WARN] report.exe build failed, skipping
) else (
    copy /y "%VERDIR%\components\report\report.exe" "%VERDIR%\report.exe" > nul
    echo   [OK] report.exe v%VER_REPORT% >> "%BUILDLOG%"
    echo   [OK] report.exe v%VER_REPORT%
)

REM Build and copy security
mkdir "%VERDIR%\components\security"
go build -o "%VERDIR%\components\security\security.exe" -ldflags "-s -w -H windowsgui -X main.version=%VER_SECURITY%" -trimpath ./cmd/security/
if errorlevel 1 (
    echo [WARN] security.exe build failed, skipping >> "%BUILDLOG%"
    echo [WARN] security.exe build failed, skipping
) else (
    copy /y "%VERDIR%\components\security\security.exe" "%VERDIR%\security.exe" > nul
    echo   [OK] security.exe v%VER_SECURITY% >> "%BUILDLOG%"
    echo   [OK] security.exe v%VER_SECURITY%
)

REM Clean up generated syso files
for %%m in (selfupdate report security) do (
    del /f /q "cmd\%%m\resource_windows_386.syso" 2>nul
    del /f /q "cmd\%%m\resource_windows_amd64.syso" 2>nul
)

REM --- Generate component version.txt files ---
echo %VER_SELFUPDATE% > "%VERDIR%\components\selfupdate\version.txt"
echo %VER_REPORT% > "%VERDIR%\components\report\version.txt"
echo %VER_SECURITY% > "%VERDIR%\components\security\version.txt"
echo.
echo   [OK] component version.txt files generated
echo   [OK] component version.txt files generated >> "%BUILDLOG%"

REM --- Generate component checksums ---
powershell -NoProfile -Command ^
"$d='%VERDIR:\=\%%'; ^
 $mods=@('selfupdate','report','security'); ^
 foreach($m in $mods){ ^
   $p=Join-Path $d ('components\'+$m+'\'+$m+'.exe'); ^
   if(Test-Path $p){ ^
     $h=(Get-FileHash $p -Algorithm SHA256).Hash.ToLower(); ^
     [IO.File]::WriteAllText((Join-Path $d ('components\'+$m+'\checksum.sha256')),$h,(New-Object Text.UTF8Encoding $false)) ^
   } ^
}"
echo   [OK] component checksums generated
echo   [OK] component checksums generated >> "%BUILDLOG%"
echo.

REM === STEP 6: Generate versions.json ===
echo [6/10] Generating versions.json...
echo [6/10] versions.json >> "%BUILDLOG%"
powershell -NoProfile -Command "$j=[ordered]@{zpui='%VERSION%';selfupdate='%VER_SELFUPDATE%';report='%VER_REPORT%';security='%VER_SECURITY%'}|ConvertTo-Json; [IO.File]::WriteAllText('%VERDIR%\versions.json',$j,(New-Object Text.UTF8Encoding $false))"
if errorlevel 1 (echo [ERROR] versions.json generation failed >> "%BUILDLOG%" & echo [ERROR] versions.json generation failed & timeout /t 10 > nul & exit /b 1)
echo Done.
echo.

REM === STEP 7: Generate checksums.sha256 ===
echo [7/10] Generating checksums.sha256...
echo [7/10] checksums.sha256 >> "%BUILDLOG%"
REM Root-level checksums (flat exes + zip archive if exists)
powershell -NoProfile -Command ^
"$d='%VERDIR:\=\%%'; ^
 $sb=''; ^
 foreach($f in 'ZPUI.exe','selfupdate.exe','report.exe','security.exe'){ ^
   $p=Join-Path $d $f; ^
   if(Test-Path $p){ ^
     $h=(Get-FileHash $p -Algorithm SHA256).Hash.ToLower(); ^
     $sz=(Get-Item $p).Length; ^
     $sb+=\"`$h  `$f (`$sz bytes)`r`n\" ^
   } ^
 }; ^
 [IO.File]::WriteAllText((Join-Path $d 'checksums.sha256'),$sb,(New-Object Text.UTF8Encoding $false))"
echo Done.
echo.

REM === STEP 8: Copy zapret from build/ + mods + config files ===
echo [8/10] Copying zapret + mods + config files...
echo [8/10] Zapret + mods + config >> "%BUILDLOG%"

REM --- Copy mods (from repo root if exists) ---
if exist "%BAT_DIR%mods" (
    xcopy /e /i /y /q "%BAT_DIR%mods" "%VERDIR%\mods" > nul
    echo   [OK] mods copied
) else (
    mkdir "%VERDIR%\mods"
    echo   [INFO] No mods found
)

REM --- Copy zapret from build/ (user places it here manually) ---
echo   [INFO] Copying zapret from build/zapret/...
if exist "%BAT_DIR%build\zapret\bin\winws.exe" (
    xcopy /e /i /y /q "%BAT_DIR%build\zapret" "%VERDIR%\zapret" > nul
    echo   [OK] zapret copied from build/zapret/
) else (
    echo   [WARN] build\zapret\bin\winws.exe not found. Place zapret in build/zapret/
    echo   [WARN] build\zapret not found >> "%BUILDLOG%"
    if not exist "%VERDIR%\zapret\bin" mkdir "%VERDIR%\zapret\bin"
)

REM --- Copy DNS domains file ---
if exist "%BAT_DIR%build\domains.txt" (
    copy /y "%BAT_DIR%build\domains.txt" "%VERDIR%\domains.txt" > nul
    echo   [OK] domains.txt copied
) else (
    echo   [INFO] build\domains.txt not found, skipping
)

REM --- Copy resource check exclusion file ---
if exist "%BAT_DIR%build\skip-resources.txt" (
    copy /y "%BAT_DIR%build\skip-resources.txt" "%VERDIR%\skip-resources.txt" > nul
    echo   [OK] skip-resources.txt copied
) else if exist "%BAT_DIR%skip-resources.txt" (
    copy /y "%BAT_DIR%skip-resources.txt" "%VERDIR%\skip-resources.txt" > nul
    echo   [OK] skip-resources.txt copied from root
) else (
    echo   [INFO] skip-resources.txt not found, skipping
)

REM --- Copy AdGuard blocklist file ---
if exist "%BAT_DIR%build\adguard_filter.txt" (
    copy /y "%BAT_DIR%build\adguard_filter.txt" "%VERDIR%\adguard_filter.txt" > nul
    echo   [OK] adguard_filter.txt copied
) else (
    echo   [INFO] build\adguard_filter.txt not found, skipping
)

echo.

REM === STEP 9: Zapret checksums (generated) ===
echo [9/10] Generating zapret checksums...
echo [9/10] Zapret checksums >> "%BUILDLOG%"
if exist "%VERDIR%\zapret\bin\winws.exe" (
    powershell -NoProfile -Command "$h=(Get-FileHash '%VERDIR%\zapret\bin\winws.exe' -Algorithm SHA256).Hash.ToLower(); $sz=(Get-Item '%VERDIR%\zapret\bin\winws.exe').Length; [IO.File]::WriteAllText('%VERDIR%\zapret\checksum.sha256',$h,(New-Object Text.UTF8Encoding $false)); echo '  [OK] checksum.sha256 ('+$sz+' bytes)'"
    if exist "%BAT_DIR%build\zapret\version.txt" (
        copy /y "%BAT_DIR%build\zapret\version.txt" "%VERDIR%\zapret\version.txt" > nul
    )
    echo   [OK] zapret checksums generated
    echo   [OK] zapret checksums generated >> "%BUILDLOG%"
) else (
    echo   [SKIP] No zapret found, checksums skipped
)
echo.

REM === STEP 10: Build installer (NSIS) ===
echo [10/10] Building installer...
echo [10/10] NSIS installer >> "%BUILDLOG%"
set "MAKENSIS="
where makensis > nul 2>&1 && for /f "delims=" %%A in ('where makensis') do (set "MAKENSIS=%%A" & goto :nsis_found)
if exist "C:\Program Files (x86)\NSIS\makensis.exe" (set "MAKENSIS=C:\Program Files (x86)\NSIS\makensis.exe" & goto :nsis_found)
if exist "C:\Program Files\NSIS\makensis.exe" (set "MAKENSIS=C:\Program Files\NSIS\makensis.exe" & goto :nsis_found)
echo [WARN] NSIS not found, skipping installer
echo [WARN] NSIS not found, skipping installer >> "%BUILDLOG%"
goto :nsis_skip

:nsis_found
echo [INFO] NSIS: %MAKENSIS%
for /f "tokens=1-4 delims=." %%a in ("%VERSION%") do set "VERSION_NUM=%%a.%%b.%%c.%%d"
if "!VERSION_NUM:~-1!"=="." set "VERSION_NUM=!VERSION_NUM!0"
REM Pass ZAPRET_OK if zapret exists in dist
set "NSIS_ZAPRET="
if exist "%VERDIR%\zapret\bin\winws.exe" set "NSIS_ZAPRET=/DZAPRET_OK"
"%MAKENSIS%" /DVERSION=%VERSION% /DVERSION_NUM=%VERSION_NUM% /DDIST="%VERDIR%" /DICON="%BAT_DIR%build\windows\icon.ico" /DOUTDIR="%BAT_DIR%build" /DLICENSE="%BAT_DIR%LICENSE" /DARCH=win32 %NSIS_ZAPRET% installer\ZPUI.nsi >> "%BUILDLOG%" 2>&1
if errorlevel 1 (
    echo [ERROR] Installer build failed >> "%BUILDLOG%"
    echo [ERROR] Installer build failed
) else (
    if exist "%BAT_DIR%build\ZPUI-Setup-%VERSION%-win32.exe" (
        copy /y "%BAT_DIR%build\ZPUI-Setup-%VERSION%-win32.exe" "%VERDIR%\" > nul
        del /f /q "%BAT_DIR%build\ZPUI-Setup-%VERSION%-win32.exe" 2>nul
        echo   [OK] ZPUI-Setup-%VERSION%-win32.exe
        echo   [OK] ZPUI-Setup-%VERSION%-win32.exe >> "%BUILDLOG%"
    )
)
:nsis_skip
echo.

REM --- Summary ---
echo ========================================
echo   Output: dist\%VERSION%\
echo.
echo   Version folder: %VERSION%
echo.
echo   Binaries:
for %%f in ("%VERDIR%\*.exe") do (
    for %%s in ("%%f") do echo     %%~nxf  %%~zs bytes
)
echo.
echo   Components:
for /d %%d in ("%VERDIR%\components\*") do (
    for %%f in ("%%d\*.exe") do (
        for %%s in ("%%f") do echo     components\%%~nxd\%%~nxf  %%~zs bytes
    )
)
echo   versions.json
echo   checksums.sha256
echo.
if exist "%VERDIR%\ZPUI-Setup-%VERSION%-win32.exe" (
    echo   Installer:
    for %%s in ("%VERDIR%\ZPUI-Setup-%VERSION%-win32.exe") do echo     ZPUI-Setup-%VERSION%-win32.exe  %%~zs bytes
) else (
    echo   Installer: (not built)
)
echo.
echo   Zapret:
if exist "%VERDIR%\zapret\bin\winws.exe" (
    for /f "usebackq tokens=*" %%a in ("%VERDIR%\zapret\version.txt") do set "ZVER=%%a"
    echo     [OK] zapret\ (v!ZVER!)
) else (
    echo     (not included — place in build/zapret/)
)
echo.
echo   ZPUI v%VERSION% (win32) + modules
echo ========================================
echo.
echo ================================================ >> "%BUILDLOG%"
echo   Build completed: %DATE% %TIME%                  >> "%BUILDLOG%"
echo ================================================ >> "%BUILDLOG%"
echo   Build log: %BUILDLOG%
echo.
echo   Press any key to close...
timeout /t 5 /nobreak > nul
exit /b 0

REM === Subroutine: generate version info syso ===
:gen_syso
set "MOD_NAME=%~1"
set "MOD_VER=%~2"
set "MOD_DESC=%~3"
for /f "tokens=1-3 delims=." %%a in ("%MOD_VER%") do (
    set "MOD_MAJ=%%a"
    set "MOD_MIN=%%b"
    set "MOD_PAT=%%c"
)
if not defined MOD_MAJ set "MOD_MAJ=0"
if not defined MOD_MIN set "MOD_MIN=0"
if not defined MOD_PAT set "MOD_PAT=0"
pushd "cmd\%MOD_NAME%"
"%GOVI%" -ver-major=%MOD_MAJ% -ver-minor=%MOD_MIN% -ver-patch=%MOD_PAT% ^
    -product-ver-major=%PROD_MAJ% -product-ver-minor=%PROD_MIN% -product-ver-patch=%PROD_PAT% ^
    -file-description="%MOD_DESC%" -product-name="ZPUI" -company-name="SuzucaRU" ^
    -copyright="MIT License" -comments="Part of ZPUI" ^
    -icon="%BAT_DIR%build\windows\icon.ico" 2>nul
popd
if exist "cmd\%MOD_NAME%\resource_windows_386.syso" (
    echo   [OK] %MOD_NAME% syso (v%MOD_VER%)
) else (
    echo   [WARN] %MOD_NAME% syso generation failed
)
goto :eof
