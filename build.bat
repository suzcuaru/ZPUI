@echo off
setlocal enabledelayedexpansion

set "BAT_DIR=%~dp0"
cd /d "%BAT_DIR%"

REM === Read ALL versions from versions.txt (NO auto-increment) ===
set "VERSION="
set "VER_TGPROXY="
set "VER_SERVICE="
set "VER_EXT="

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
    if "!key!"=="tgproxy" set "VER_TGPROXY=!val!"
    if "!key!"=="service" set "VER_SERVICE=!val!"
    if "!key!"=="extension" set "VER_EXT=!val!"
    if "!key!"=="server" set "SERVER_URL=!val!"
)

if not defined VERSION (
    echo [ERROR] zpui version not found in versions.txt
    timeout /t 10 > nul
    exit /b 1
)

if not defined VER_SERVICE set "VER_SERVICE=1.0.0"
if not defined VER_EXT set "VER_EXT=1.0.0"

REM === Мьютекс сборки: две одновременные сборки (параллельные сессии) молча
REM === ломают dist и payload — вторая должна валиться сразу.
set "BUILDLOCK=%BAT_DIR%.buildlock"
if exist "%BUILDLOCK%" (
    powershell -NoProfile -Command "if((Get-Date) - (Get-Item '%BUILDLOCK%').LastWriteTime -gt [TimeSpan]::FromMinutes(20)){ exit 0 } else { exit 1 }" > nul 2>nul
    if not errorlevel 1 (
        echo [INFO] stale build lock found - stealing
        rmdir /s /q "%BUILDLOCK%" 2>nul
    )
)
mkdir "%BUILDLOCK%" 2>nul
if errorlevel 1 (
    echo [ERROR] another ZPUI build is already running - wait for it or delete %BUILDLOCK%
    exit /b 1
)
echo building %RANDOM% > "%BUILDLOCK%\build.info"

REM --- Версия TgProxy — из родной кодовой базы компонента
REM     (..\ZPUI-TgProxy\tg-ws-proxy\proxy\__init__.py, __version__).
set "TGPROXY_SRC=%BAT_DIR%..\ZPUI-TgProxy\tg-ws-proxy"
set "VER_TGPROXY="
if exist "%TGPROXY_SRC%\proxy\__init__.py" (
    for /f "usebackq delims=" %%V in (`python "%BAT_DIR%..\ZPUI-TgProxy\integration\upstream_version.py"`) do set "VER_TGPROXY=%%V"
)
if not defined VER_TGPROXY (
    echo [WARN] tg-ws-proxy not found or version unreadable - TgProxy component will be skipped
    set "VER_TGPROXY=0.0.0"
)

echo [INFO] Versions from versions.txt / component sources:
echo   ZPUI:       %VERSION%
echo   TgProxy:    %VER_TGPROXY% (tg-ws-proxy upstream)
echo   Service:    %VER_SERVICE%

REM --- Номер сборки: счётчик в build\buildnumber.txt, +1 за сборку.
REM --- Пишется в лог (имя файла и шапка), versions.json и вшивается в exe.
set "BUILDNUM_FILE=%BAT_DIR%build\buildnumber.txt"
set BUILDNUM=0
if exist "%BUILDNUM_FILE%" set /p BUILDNUM=<"%BUILDNUM_FILE%"
set /a BUILDNUM+=1
> "%BUILDNUM_FILE%" echo %BUILDNUM%

REM --- Prepare build log (placeholder; real init after clean) ---
set "BUILDLOG=%BAT_DIR%dist\build-%VERSION%-b%BUILDNUM%.log"

REM --- Sync wails.json productVersion ---
powershell -NoProfile -Command "& {$p='%BAT_DIR%wails.json'; $c=Get-Content $p -Raw | ConvertFrom-Json; $c.info.productVersion='%VERSION%'; $enc=New-Object System.Text.UTF8Encoding $false; [System.IO.File]::WriteAllText($p, ($c | ConvertTo-Json -Depth 10), $enc)}" > nul 2>&1

set "DIST=%BAT_DIR%dist"
set "VERDIR=%DIST%\%VERSION%"

echo ========================================
echo   ZPUI Build System v%VERSION% (x86 + x64)
echo   BUILD #%BUILDNUM%   %DATE% %TIME%
echo   Core + Modules + Universal Installer
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
taskkill /IM ZPUI-Service.exe /F > nul 2>&1
taskkill /IM ZPUI-TgProxy.exe /F > nul 2>&1
timeout /t 1 /nobreak > nul

if exist "%DIST%" rmdir /s /q "%DIST%"
if exist "%BAT_DIR%build\bin\ZPUI.exe" del /f /q "%BAT_DIR%build\bin\ZPUI.exe"
del /f /q "%BAT_DIR%build\ZPUI-Setup-*.exe" 2>nul
del /f /q "%BAT_DIR%build\ZPUI-Update-*.exe" 2>nul
mkdir "%VERDIR%"

REM Initialize build log (after dist cleanup, so it's not deleted)
echo ================================================ > "%BUILDLOG%"
echo   ZPUI Build Log  v%VERSION%                     >> "%BUILDLOG%"
echo   %DATE% %TIME%                                   >> "%BUILDLOG%"
echo ================================================ >> "%BUILDLOG%"
echo. >> "%BUILDLOG%"
echo Versions: ZPUI=%VERSION% >> "%BUILDLOG%"
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
    call :gen_syso service      "%VER_SERVICE%"      "ZPUI Service"
) else (
    echo   [WARN] Skipping version resources
)
echo.

REM === STEP 4: Build main app (Wails, x86 + x64) ===
REM Два дерева дистрибутива: VERDIR\x86 и VERDIR\x64. Универсальный
REM установщик упаковывает оба и ставит подходящую битность по системе.
echo [4/10] Building ZPUI core (x86 + x64)...
echo [4/10] ZPUI core >> "%BUILDLOG%"
mkdir "%VERDIR%\x86" 2>nul
mkdir "%VERDIR%\x64" 2>nul

REM --- VERSIONINFO + иконка + манифест для ZPUI.exe.
REM     Wails-CLI не вкладывает ресурсы из build/windows/info.json (exe выходит
REM     «безликим» PE — триггер эвристик AV), поэтому генерим syso сами тем же
REM     goversioninfo, что и для инсталляторов. Оригинальное имя файла и
REM     описание — первое, что проверяет аналитик при разборе ложного
REM     срабатывания.
if defined GOVI (
    (
echo {
echo   "FixedFileInfo": {
echo     "FileVersion": {"Major": %PROD_MAJ%, "Minor": %PROD_MIN%, "Patch": %PROD_PAT%, "Build": 0},
echo     "ProductVersion": {"Major": %PROD_MAJ%, "Minor": %PROD_MIN%, "Patch": %PROD_PAT%, "Build": 0}
echo   },
echo   "StringFileInfo": {
echo     "FileDescription": "ZPUI — Zapret DPI bypass controller",
echo     "OriginalFilename": "ZPUI.exe",
echo     "ProductName": "ZPUI",
echo     "CompanyName": "SuzucaRU",
echo     "LegalCopyright": "Copyright (c) 2026 SuzucaRU. All rights reserved."
echo   },
echo   "IconPath": "build\\windows\\icon.ico",
echo   "ManifestPath": "installer\\app-manifest.xml"
echo }
    ) > versioninfo.json
    "%GOVI%" -platform-specific versioninfo.json >> "%BUILDLOG%" 2>&1
    if exist "resource_windows_386.syso" (
        echo   [OK] ZPUI.exe version resources
        echo   [OK] ZPUI.exe version resources >> "%BUILDLOG%"
    ) else (
        echo   [WARN] ZPUI.exe syso generation failed - exe stays without version info >> "%BUILDLOG%"
        echo   [WARN] ZPUI.exe syso generation failed
    )
)

"%WAILS%" build -platform windows/386 -s -o ZPUI.exe -nopackage ^
    -ldflags "-s -w -H windowsgui -X main.version=%VERSION% -X zpui/internal/app.BuildNum=%BUILDNUM% -X zpui/internal/tgproxy.Version=%VER_TGPROXY% -X zpui/internal/app.ServerURL=%SERVER_URL% -X zpui/internal/zsvc.BundleVersion=%VER_SERVICE%" -trimpath
if errorlevel 1 (echo [ERROR] Wails x86 build failed >> "%BUILDLOG%" & echo [ERROR] Wails x86 build failed & timeout /t 10 > nul & exit /b 1)
copy /y "build\bin\ZPUI.exe" "%VERDIR%\x86\ZPUI.exe" > nul
echo ZPUI.exe (x86) built >> "%BUILDLOG%"
"%WAILS%" build -platform windows/amd64 -s -o ZPUI.exe -nopackage ^
    -ldflags "-s -w -H windowsgui -X main.version=%VERSION% -X zpui/internal/app.BuildNum=%BUILDNUM% -X zpui/internal/tgproxy.Version=%VER_TGPROXY% -X zpui/internal/app.ServerURL=%SERVER_URL% -X zpui/internal/zsvc.BundleVersion=%VER_SERVICE%" -trimpath
if errorlevel 1 (echo [ERROR] Wails x64 build failed >> "%BUILDLOG%" & echo [ERROR] Wails x64 build failed & timeout /t 10 > nul & exit /b 1)
copy /y "build\bin\ZPUI.exe" "%VERDIR%\x64\ZPUI.exe" > nul
echo ZPUI.exe (x64) built >> "%BUILDLOG%"
del /f /q "resource_windows_386.syso" "resource_windows_amd64.syso" "resource_windows_arm.syso" "resource_windows_arm64.syso" "versioninfo.json" 2>nul
echo [INFO] Checking frontend bindings...
pushd web
node check-bindings.mjs
if errorlevel 1 echo [WARN] wailsjs bindings stale - runtime bindings are dynamic, continuing
popd
REM Корневые экземпляры — лицо дистрибутива (релиз/CI/портатив): ZPUI.exe = x64,
REM ZPUI-x86.exe = x86; деревья x86/x64 расходуются установщиком.
copy /y "%VERDIR%\x64\ZPUI.exe" "%VERDIR%\ZPUI.exe" > nul
copy /y "%VERDIR%\x86\ZPUI.exe" "%VERDIR%\ZPUI-x86.exe" > nul
echo.

REM === STEP 5: Build module exes ===
echo [5/10] Building module tools (win32, runs everywhere)...
echo [5/10] Module tools >> "%BUILDLOG%"
set "GOARCH=386"

REM --- Service exe: 32-битный (запускается на любой Windows): components\service\
REM     в обоих деревьях + копии в корни деревьев и корень дистрибутива.
REM     Регистрирует службу pkginst при установке.
REM     ВАЖНО: в скобочных блоках echo без круглых скобок в тексте.
mkdir "%VERDIR%\x86\components\service"
mkdir "%VERDIR%\x64\components\service"
go build -o "%VERDIR%\x86\components\service\ZPUI-Service.exe" -ldflags "-s -w -H windowsgui -X main.version=%VER_SERVICE%" -trimpath ./cmd/service/
if errorlevel 1 (
    echo [WARN] ZPUI-Service.exe build failed, skipping >> "%BUILDLOG%"
    echo [WARN] ZPUI-Service.exe build failed, skipping
    goto service_done
)
REM Служба живёт ТОЛЬКО в components\service: корневые копии — легаси,
REM убраны 31.10 (дублировали файл в 2 места без единого потребителя).
copy /y "%VERDIR%\x86\components\service\ZPUI-Service.exe" "%VERDIR%\x64\components\service\ZPUI-Service.exe" > nul
echo   [OK] ZPUI-Service.exe v%VER_SERVICE% - x86, in components only >> "%BUILDLOG%"
echo   [OK] ZPUI-Service.exe v%VER_SERVICE% - x86, in components only

:service_done

REM --- Browser extension: подписанный CRX3 из соседнего субпроекта
REM     ZPUI-Extension -> components\extension\ в обоих деревьях. Ставит его
REM     в браузеры приложение (internal\extension). Приватный ключ - в
REM     ZPUI-Extension\build\key.pem, в дистрибутив не попадает.
REM     ВАЖНО: в скобочных блоках echo без круглых скобок в тексте.
set "EXT_SRC=%BAT_DIR%..\ZPUI-Extension"
set "EXT_CRX=%VERDIR%\x64\components\extension\zpui-extension.crx"
if not exist "%EXT_SRC%\manifest.json" goto ext_done
if not exist "%EXT_SRC%\build\key.pem" goto ext_nokey
mkdir "%VERDIR%\x64\components\extension"
mkdir "%VERDIR%\x86\components\extension"
pushd "%EXT_SRC%"
go run ./tools/mkcrx.go -src . -key build/key.pem -version "%VER_EXT%" -out "%EXT_CRX%"
set "EXT_RC=!errorlevel!"
popd
if not "!EXT_RC!"=="0" goto ext_fail
copy /y "%EXT_CRX%" "%VERDIR%\x86\components\extension\zpui-extension.crx" > nul
echo   [OK] zpui-extension.crx v%VER_EXT% in components\extension >> "%BUILDLOG%"
echo   [OK] zpui-extension.crx v%VER_EXT% in components\extension
goto ext_done

:ext_nokey
echo [WARN] ZPUI-Extension key not found, extension not packaged >> "%BUILDLOG%"
echo [WARN] ZPUI-Extension key not found, extension not packaged
goto ext_done

:ext_fail
echo [WARN] ZPUI-Extension CRX build failed, skipping >> "%BUILDLOG%"
echo [WARN] ZPUI-Extension CRX build failed, skipping

:ext_done

REM --- Extension standalone installer: cmd\extsetup со встроенным свежим CRX
REM     (копия из x64 дерева). Приложение обновляет расширение, запуская этот
REM     exe тихо (-silent); вручную качают с сервера. Только amd64: Wails-GUI.
REM     ВАЖНО: в скобочных блоках echo без круглых скобок в тексте.
if not exist "%EXT_CRX%" goto extsetup_done
if not exist "%BAT_DIR%cmd\extsetup\main.go" goto extsetup_done
copy /y "%EXT_CRX%" "%BAT_DIR%cmd\extsetup\extension.crx" > nul
go build -trimpath -tags desktop,production -ldflags "-s -w -H windowsgui -X main.version=%VER_EXT%" -o "%VERDIR%\x64\components\extension\ZPUI-Extension-Setup.exe" .\cmd\extsetup >> "%BUILDLOG%" 2>&1
if errorlevel 1 goto extsetup_fail
echo   [OK] ZPUI-Extension-Setup.exe v%VER_EXT% in components\extension >> "%BUILDLOG%"
echo   [OK] ZPUI-Extension-Setup.exe v%VER_EXT% in components\extension
goto extsetup_done

:extsetup_fail
echo [WARN] ZPUI-Extension-Setup build failed, skipping >> "%BUILDLOG%"
echo [WARN] ZPUI-Extension-Setup build failed, skipping

:extsetup_done

REM --- TgProxy component: headless-мост Telegram из родной кодовой базы
REM     tg-ws-proxy, без правок их файлов. Собирается integration\build.bat
REM     проекта ..\ZPUI-TgProxy; версия берётся из proxy\__init__.py апстрима.
set "TGPROXY_BUILT=0"
if exist "%BAT_DIR%..\ZPUI-TgProxy\integration\build.bat" (
    echo [INFO] Building TgProxy component...
    call "%BAT_DIR%..\ZPUI-TgProxy\integration\build.bat"
    if not errorlevel 1 set "TGPROXY_BUILT=1"
)
if "%TGPROXY_BUILT%"=="1" goto tgproxy_have
echo [WARN] TgProxy component build failed - Telegram bridge will be unavailable >> "%BUILDLOG%"
echo [WARN] TgProxy component build failed - Telegram bridge will be unavailable
goto tgproxy_done

:tgproxy_have
REM Компонент собирается в двух битностях: dist\x86 -> дерево x86, dist\x64 -> x64.
REM Если одной битности нет (нет 32-битного Python) — кладём имеющуюся с WARN.
REM exe дополнительно проверяется на размер: half-written файл из чужой сборки
REM не должен попасть в дистрибутив.
set "TGX86_SRC=%BAT_DIR%..\ZPUI-TgProxy\integration\dist\x86"
set "TGX64_SRC=%BAT_DIR%..\ZPUI-TgProxy\integration\dist\x64"
set "TGX86_OK=0"
set "TGX64_OK=0"
for %%F in ("%TGX86_SRC%\ZPUI-TgProxy.exe") do if exist "%%F" if %%~zF GTR 8000000 set "TGX86_OK=1"
for %%F in ("%TGX64_SRC%\ZPUI-TgProxy.exe") do if exist "%%F" if %%~zF GTR 8000000 set "TGX64_OK=1"

for %%a in (x86 x64) do (
    mkdir "%VERDIR%\%%a\components\tgproxy" 2>nul
)
if "%TGX86_OK%"=="1" (
    copy /y "%TGX86_SRC%\*" "%VERDIR%\x86\components\tgproxy\" > nul
) else (
    if "%TGX64_OK%"=="1" copy /y "%TGX64_SRC%\*" "%VERDIR%\x86\components\tgproxy\" > nul
    echo [WARN] x86 TgProxy missing - x64 binary placed into x86 tree >> "%BUILDLOG%"
    echo [WARN] x86 TgProxy missing - x64 binary placed into x86 tree
)
if "%TGX64_OK%"=="1" (
    copy /y "%TGX64_SRC%\*" "%VERDIR%\x64\components\tgproxy\" > nul
) else (
    if "%TGX86_OK%"=="1" copy /y "%TGX86_SRC%\*" "%VERDIR%\x64\components\tgproxy\" > nul
    echo [WARN] x64 TgProxy missing - x86 binary placed into x64 tree >> "%BUILDLOG%"
    echo [WARN] x64 TgProxy missing - x86 binary placed into x64 tree
)
echo   [OK] ZPUI-TgProxy.exe v%VER_TGPROXY% (x86 + x64)
echo   [OK] ZPUI-TgProxy.exe v%VER_TGPROXY% (x86 + x64) >> "%BUILDLOG%"

:tgproxy_done

REM Clean up generated syso files
for %%m in (service) do (
    del /f /q "cmd\%%m\resource_windows_386.syso" 2>nul
    del /f /q "cmd\%%m\resource_windows_amd64.syso" 2>nul
    del /f /q "cmd\%%m\resource_windows_arm.syso" 2>nul
    del /f /q "cmd\%%m\resource_windows_arm64.syso" 2>nul
    del /f /q "cmd\%%m\versioninfo.json" 2>nul
)

REM --- Copy wintun driver (ZPUI VPN) into both trees ---
REM ВАЖНО: в текстах echo внутри скобочных блоков не должно быть круглых
REM скобок — закрывающая скобка обрывает блок и ломает ветвление.
set "WINTUN_FOUND=0"
if exist "components\wintun\wintun_386.dll" set "WINTUN_FOUND=1"
if exist "components\wintun\wintun_amd64.dll" set "WINTUN_FOUND=1"
if "%WINTUN_FOUND%"=="1" goto wintun_have
echo [WARN] components\wintun DLLs not found, VPN will be unavailable >> "%BUILDLOG%"
echo [WARN] components\wintun DLLs not found, VPN will be unavailable
goto wintun_done

:wintun_have
for %%a in (x86 x64) do (
    mkdir "%VERDIR%\%%a\components\wintun" 2>nul
    copy /y "components\wintun\wintun_*.dll" "%VERDIR%\%%a\components\wintun\" > nul
)
if not exist "%VERDIR%\x86\components\wintun\wintun_386.dll" (
    echo [WARN] wintun_386.dll missing - 32-bit VPN will fail, take x86 wintun.dll from wintun.net zip >> "%BUILDLOG%"
    echo [WARN] wintun_386.dll missing - 32-bit VPN will fail, take x86 wintun.dll from wintun.net zip
)
if not exist "%VERDIR%\x64\components\wintun\wintun_amd64.dll" (
    echo [WARN] wintun_amd64.dll missing - 64-bit VPN will fail, take x64 wintun.dll from wintun.net zip >> "%BUILDLOG%"
    echo [WARN] wintun_amd64.dll missing - 64-bit VPN will fail, take x64 wintun.dll from wintun.net zip
)
echo   [OK] wintun driver copied >> "%BUILDLOG%"
echo   [OK] wintun driver copied

:wintun_done

REM --- Generate component version.txt files ---
echo %VER_SERVICE% > "%VERDIR%\x86\components\service\version.txt"
echo %VER_SERVICE% > "%VERDIR%\x64\components\service\version.txt"
echo.
echo   [OK] component version.txt files generated
echo   [OK] component version.txt files generated >> "%BUILDLOG%"

REM --- Generate component checksums (both trees) ---
powershell -NoProfile -Command ^
"$d='%VERDIR:\=\%%'; ^
 $mods=@(); ^
 foreach($a in 'x86','x64'){ ^
   foreach($m in $mods){ ^
     $p=Join-Path $d ($a+'\components\'+$m+'\'+$m+'.exe'); ^
     if(Test-Path $p){ ^
       $h=(Get-FileHash $p -Algorithm SHA256).Hash.ToLower(); ^
       [IO.File]::WriteAllText((Join-Path $d ($a+'\components\'+$m+'\checksum.sha256')),$h,(New-Object Text.UTF8Encoding $false)) ^
     } ^
   } ^
 }"
REM Служба и TgProxy: имена exe отличаются от имени компонента, поэтому
REM checksum отдельным проходом — components\<модуль>\checksum.sha256.
powershell -NoProfile -Command ^
"$d='%VERDIR:\=\%%'; ^
 foreach($a in 'x86','x64'){ ^
   foreach($rel in @('components\service\ZPUI-Service.exe','components\tgproxy\ZPUI-TgProxy.exe')){ ^
     $p=Join-Path $d ($a+'\'+$rel); ^
     if(Test-Path $p){ ^
       $h=(Get-FileHash $p -Algorithm SHA256).Hash.ToLower(); ^
       [IO.File]::WriteAllText((Join-Path $d ($a+'\'+(Split-Path $rel -Parent)+'\checksum.sha256')),$h,(New-Object Text.UTF8Encoding $false)) ^
     } ^
   } ^
 }"
echo   [OK] component checksums generated
echo   [OK] component checksums generated >> "%BUILDLOG%"
echo.

REM === STEP 6: Generate versions.json ===
echo [6/10] Generating versions.json...
echo [6/10] versions.json >> "%BUILDLOG%"
if exist "%BAT_DIR%CHANGELOG.md" copy /y "%BAT_DIR%CHANGELOG.md" "%VERDIR%\CHANGELOG.md" > nul
REM Скилл zpui-mcp для ИИ-агентов: в оба дерева dist (файл на месте сразу после
REM установки, до первого запуска exe; дальше exe поддерживает его сам из embedded).
if exist "%BAT_DIR%.agents\skills\zpui-mcp\SKILL.md" (
    mkdir "%VERDIR%\x86\skills\zpui-mcp" 2>nul
    mkdir "%VERDIR%\x64\skills\zpui-mcp" 2>nul
    copy /y "%BAT_DIR%.agents\skills\zpui-mcp\SKILL.md" "%VERDIR%\x86\skills\zpui-mcp\SKILL.md" > nul
    copy /y "%BAT_DIR%.agents\skills\zpui-mcp\SKILL.md" "%VERDIR%\x64\skills\zpui-mcp\SKILL.md" > nul
)
powershell -NoProfile -Command "$j=[ordered]@{zpui='%VERSION%';build='%BUILDNUM%';tgproxy='%VER_TGPROXY%';service='%VER_SERVICE%';extension='%VER_EXT%'}|ConvertTo-Json; foreach($p in @('%VERDIR%\versions.json','%VERDIR%\x86\versions.json','%VERDIR%\x64\versions.json')){[IO.File]::WriteAllText($p,$j,(New-Object Text.UTF8Encoding $false))}"
if errorlevel 1 (echo [ERROR] versions.json generation failed >> "%BUILDLOG%" & echo [ERROR] versions.json generation failed & timeout /t 10 > nul & exit /b 1)
echo Done.
echo.

REM === STEP 7: Generate checksums.sha256 (root, release-facing) ===
echo [7/10] Generating checksums.sha256...
echo [7/10] checksums.sha256 >> "%BUILDLOG%"
REM Корневой checksums.sha256 уезжает на Яндекс.Диск: по нему приложение
REM проверяет скачанный установщик; попутно — хеши обоих exe.
powershell -NoProfile -Command ^
"$d='%VERDIR:\=\%%'; ^
 $sb=''; ^
 foreach($f in 'ZPUI.exe','ZPUI-x86.exe'){ ^
   $p=Join-Path $d $f; ^
   if(Test-Path $p){ ^
     $h=(Get-FileHash $p -Algorithm SHA256).Hash.ToLower(); ^
     $sz=(Get-Item $p).Length; ^
     $sb+=\"$h  $f ($sz bytes)`r`n\" ^
   } ^
 }; ^
 [IO.File]::WriteAllText((Join-Path $d 'checksums.sha256'),$sb,(New-Object Text.UTF8Encoding $false))"
echo Done.
echo.

REM === STEP 8: Copy zapret from build/ + config files (both trees) ===
echo [8/10] Copying zapret + config files...
echo [8/10] Zapret + config >> "%BUILDLOG%"

REM --- Copy zapret from build/ (user places it here manually) ---
echo   [INFO] Copying zapret from build/zapret/...
if exist "%BAT_DIR%build\zapret\bin\winws.exe" (
    for %%a in (x86 x64) do xcopy /e /i /y /q "%BAT_DIR%build\zapret" "%VERDIR%\%%a\zapret" > nul
    echo   [OK] zapret copied from build/zapret/
) else (
    echo   [WARN] build\zapret\bin\winws.exe not found. Place zapret in build/zapret/
    echo   [WARN] build\zapret not found >> "%BUILDLOG%"
    for %%a in (x86 x64) do (
        if not exist "%VERDIR%\%%a\zapret\bin" mkdir "%VERDIR%\%%a\zapret\bin"
    )
)

REM --- Copy Telegram links file (web page of the bridge) ---
if exist "%BAT_DIR%telegram_links.txt" (
    copy /y "%BAT_DIR%telegram_links.txt" "%VERDIR%\x86\telegram_links.txt" > nul
    copy /y "%BAT_DIR%telegram_links.txt" "%VERDIR%\x64\telegram_links.txt" > nul
    echo   [OK] telegram_links.txt copied
) else (
    echo   [INFO] telegram_links.txt not found, skipping
)

REM --- Copy DNS domains file ---
if exist "%BAT_DIR%build\domains.txt" (
    copy /y "%BAT_DIR%build\domains.txt" "%VERDIR%\x86\domains.txt" > nul
    copy /y "%BAT_DIR%build\domains.txt" "%VERDIR%\x64\domains.txt" > nul
    echo   [OK] domains.txt copied
) else (
    echo   [INFO] build\domains.txt not found, skipping
)

REM --- Resource check exclusion files: user list (skip-resources.txt) is NOT
REM     shipped - the app creates it on first start and protects it on updates;
REM     only the server-managed system list is shipped.
if exist "%BAT_DIR%skip-resources-system.txt" (
    copy /y "%BAT_DIR%skip-resources-system.txt" "%VERDIR%\x86\skip-resources-system.txt" > nul
    copy /y "%BAT_DIR%skip-resources-system.txt" "%VERDIR%\x64\skip-resources-system.txt" > nul
    echo   [OK] skip-resources-system.txt copied
) else (
    echo   [INFO] skip-resources-system.txt not found, skipping
)

REM --- Copy AdGuard blocklist file ---
if exist "%BAT_DIR%build\adguard_filter.txt" (
    copy /y "%BAT_DIR%build\adguard_filter.txt" "%VERDIR%\x86\adguard_filter.txt" > nul
    copy /y "%BAT_DIR%build\adguard_filter.txt" "%VERDIR%\x64\adguard_filter.txt" > nul
    echo   [OK] adguard_filter.txt copied
) else (
    echo   [INFO] build\adguard_filter.txt not found, skipping
)

echo.

REM === STEP 9: Zapret checksums (generated, both trees) ===
echo [9/10] Generating zapret checksums...
echo [9/10] Zapret checksums >> "%BUILDLOG%"
if exist "%VERDIR%\x86\zapret\bin\winws.exe" (
    for %%a in (x86 x64) do powershell -NoProfile -Command "$h=(Get-FileHash '%VERDIR%\%%a\zapret\bin\winws.exe' -Algorithm SHA256).Hash.ToLower(); [IO.File]::WriteAllText('%VERDIR%\%%a\zapret\checksum.sha256',$h,(New-Object Text.UTF8Encoding $false))"
    echo   [OK] zapret checksums generated
    echo   [OK] zapret checksums generated >> "%BUILDLOG%"
) else (
    echo   [SKIP] No zapret found, checksums skipped
)
if exist "%BAT_DIR%build\zapret\version.txt" (
    copy /y "%BAT_DIR%build\zapret\version.txt" "%VERDIR%\x86\zapret\version.txt" > nul 2>nul
    copy /y "%BAT_DIR%build\zapret\version.txt" "%VERDIR%\x64\zapret\version.txt" > nul 2>nul
)
echo.

REM === STEP 10: Installers (Wails/Go) ===
echo [10/10] Building installers (Wails)...
echo [10/10] Wails installers >> "%BUILDLOG%"

set "INSTPACK=%BAT_DIR%build\instpack"
if exist "%INSTPACK%" rmdir /s /q "%INSTPACK%"
mkdir "%INSTPACK%" 2>nul

REM --- uninstall.exe: 32-битный, запускается на любой Windows; в корень пакета
set GOOS=windows
set GOARCH=386
if not exist "%INSTPACK%" mkdir "%INSTPACK%"
go build -trimpath -tags desktop,production -ldflags "-s -w -H windowsgui -X main.version=%VERSION%" -o "%INSTPACK%\uninstall.exe" .\cmd\uninstall >> "%BUILDLOG%" 2>&1
if errorlevel 1 (
    echo [ERROR] uninstall.exe build failed >> "%BUILDLOG%"
    echo [ERROR] uninstall.exe build failed
) else (
    echo   [OK] uninstall.exe
    echo   [OK] uninstall.exe >> "%BUILDLOG%"
)

REM --- payload.zip: два пакета. Полный, с запретом — установщику; обновлятору
REM     --- без zapret: запрет ставится/обновляется только приложением,
REM     обновлятор его не содержит и не трогает.
mkdir "%INSTPACK%\x86" 2>nul
mkdir "%INSTPACK%\x64" 2>nul
xcopy /e /i /y "%VERDIR%\x86" "%INSTPACK%\x86\" > nul 2>nul
xcopy /e /i /y "%VERDIR%\x64" "%INSTPACK%\x64\" > nul 2>nul
copy /y "%BAT_DIR%README.md" "%INSTPACK%\" > nul 2>nul
copy /y "%BAT_DIR%LICENSE" "%BAT_DIR%cmd\setup\license.txt" > nul 2>nul
copy /y "%BAT_DIR%CHANGELOG.md" "%BAT_DIR%cmd\updater\changelog.txt" > nul 2>nul
copy /y "%BAT_DIR%CHANGELOG.md" "%BAT_DIR%cmd\setup\changelog.txt" > nul 2>nul
copy /y "%BAT_DIR%LICENSE" "%INSTPACK%\" > nul 2>nul
powershell -NoProfile -Command "Compress-Archive -Path '%INSTPACK%\*' -DestinationPath '%BAT_DIR%cmd\setup\payload.zip' -Force" >> "%BUILDLOG%" 2>&1
REM --- Setup несёт запрет (zapret1 Flowseal); для пакета обновлятора запрет
REM --- вырезается ниже перед упаковкой.
if exist "%INSTPACK%\x86\zapret" rmdir /s /q "%INSTPACK%\x86\zapret"
if exist "%INSTPACK%\x64\zapret" rmdir /s /q "%INSTPACK%\x64\zapret"
powershell -NoProfile -Command "Compress-Archive -Path '%INSTPACK%\*' -DestinationPath '%BAT_DIR%cmd\updater\payload.zip' -Force" >> "%BUILDLOG%" 2>&1

REM --- ГАРД: в пакете обновлятора запрета быть не должно никогда (запрет
REM --- ставится только установщиком). rmdir мог молча не сработать на
REM --- залоченном файле — тогда сборка обязана упасть здесь.
powershell -NoProfile -Command "Add-Type -AssemblyName System.IO.Compression.FileSystem; $z=[IO.Compression.ZipFile]::OpenRead('%BAT_DIR%cmd\updater\payload.zip'); $hit=@($z.Entries | Where-Object { $_.FullName -match 'zapret' }); $z.Dispose(); if($hit.Count -gt 0){ Write-Output ('[ERROR] updater payload contains zapret: ' + $hit[0].FullName); exit 1 }" > nul 2>nul
if errorlevel 1 (
    echo   [ERROR] updater payload.zip contains zapret - updater must NEVER ship zapret
    echo [ERROR] updater payload.zip contains zapret >> "%BUILDLOG%"
    if exist "%INSTPACK%" rmdir /s /q "%INSTPACK%"
    exit /b 1
)
echo   [OK] updater payload verified: no zapret inside
echo   [OK] updater payload verified: no zapret inside >> "%BUILDLOG%"

REM --- ГАРД (инверсия): в пакете установщика запрет ОБЯЗАН быть — приложение
REM --- стартует без скачивания только при наличии движка на диске.
REM --- ПРИМЕЧАНИЕ: Compress-Archive пишет пути с обратными слешами — регулярка
REM --- принимает оба разделителя, иначе гард ложно падает.
powershell -NoProfile -Command "Add-Type -AssemblyName System.IO.Compression.FileSystem; $z=[IO.Compression.ZipFile]::OpenRead('%BAT_DIR%cmd\setup\payload.zip'); $hit=@($z.Entries | Where-Object { $_.FullName -match 'zapret[\\/]bin[\\/]winws\.exe$' }); $z.Dispose(); if($hit.Count -eq 0){ Write-Output '[ERROR] setup payload has no zapret/bin/winws.exe'; exit 1 }" > nul 2>nul
if errorlevel 1 (
    echo   [ERROR] setup payload.zip contains no zapret - installer must ship zapret
    echo [ERROR] setup payload.zip contains no zapret >> "%BUILDLOG%"
    if exist "%INSTPACK%" rmdir /s /q "%INSTPACK%"
    exit /b 1
)
echo   [OK] setup payload verified: zapret inside
echo   [OK] setup payload verified: zapret inside >> "%BUILDLOG%"
if exist "%BAT_DIR%cmd\setup\payload.zip" (
    echo   [OK] payload.zip packed - setup with zapret, updater without
    echo   [OK] payload.zip packed - setup with zapret, updater without >> "%BUILDLOG%"
)

REM --- ГАРД: если компонент tgproxy собирался, он обязан быть в payload.
REM --- Установщик без моста Telegram = молчаливый баг (случалось на гонке сборок).
if "%TGPROXY_BUILT%"=="1" (
    powershell -NoProfile -Command "Add-Type -AssemblyName System.IO.Compression.FileSystem; $z=[IO.Compression.ZipFile]::OpenRead('%BAT_DIR%cmd\setup\payload.zip'); $hit=@($z.Entries | Where-Object { $_.FullName -like '*tgproxy/ZPUI-TgProxy.exe' -or $_.FullName -like '*tgproxy\ZPUI-TgProxy.exe' }); $z.Dispose(); if($hit.Count -eq 0){ exit 1 }" > nul 2>nul
    if errorlevel 1 (
        echo [ERROR] setup payload has no ZPUI-TgProxy.exe - installer would ship without Telegram bridge
        echo [ERROR] setup payload has no ZPUI-TgProxy.exe >> "%BUILDLOG%"
        rmdir /s /q "%BUILDLOCK%" 2>nul
        exit /b 1
    )
    echo   [OK] setup payload contains tgproxy component
    echo   [OK] setup payload contains tgproxy component >> "%BUILDLOG%"
) else (
    echo [ERROR] payload.zip packing failed >> "%BUILDLOG%"
    echo [ERROR] payload.zip packing failed
)

REM --- ресурсы (иконка + версия + манифест админа) для трёх инсталляторов
for /f "tokens=1-4 delims=." %%a in ("%VERSION%") do (
    set "IV_MAJ=%%a"
    set "IV_MIN=%%b"
    set "IV_PAT=%%c"
)
if not defined IV_MAJ set "IV_MAJ=0"
if not defined IV_MIN set "IV_MIN=0"
if not defined IV_PAT set "IV_PAT=0"
for %%m in (setup updater uninstall) do (
    pushd "cmd\%%m"
    (
echo {
echo   "FixedFileInfo": {
echo     "FileVersion": {"Major": %IV_MAJ%, "Minor": %IV_MIN%, "Patch": %IV_PAT%, "Build": 0},
echo     "ProductVersion": {"Major": %IV_MAJ%, "Minor": %IV_MIN%, "Patch": %IV_PAT%, "Build": 0}
echo   },
echo   "StringFileInfo": {
echo     "FileDescription": "ZPUI %%m",
echo     "OriginalFilename": "ZPUI-%%m.exe",
echo     "ProductName": "ZPUI",
echo     "CompanyName": "SuzucaRU",
echo     "LegalCopyright": "Copyright (c) 2026 SuzucaRU. All rights reserved."
echo   },
echo   "IconPath": "..\\..\\build\\windows\\icon.ico",
echo   "ManifestPath": "..\\..\\installer\\manifest.xml"
echo }
    ) > versioninfo.json
    "%GOVI%" -platform-specific versioninfo.json >> "%BUILDLOG%" 2>&1
    if exist "resource_windows_386.syso" (
        echo   [OK] %%m syso resources
        echo   [OK] %%m syso resources >> "%BUILDLOG%"
    ) else (
        echo [ERROR] %%m syso generation failed - exe will have no icon >> "%BUILDLOG%"
        echo [ERROR] %%m syso generation failed - exe will have no icon
    )
    popd
)

REM --- сборка (32-битные exe — запускаются на любой Windows)
go build -trimpath -tags desktop,production -ldflags "-s -w -H windowsgui -X main.version=%VERSION%" -o "%VERDIR%\ZPUI-Setup-%VERSION%.exe" .\cmd\setup >> "%BUILDLOG%" 2>&1
if errorlevel 1 (
    echo [ERROR] ZPUI-Setup build failed >> "%BUILDLOG%"
    echo [ERROR] ZPUI-Setup build failed
) else (
    echo   [OK] ZPUI-Setup-%VERSION%.exe
    echo   [OK] ZPUI-Setup-%VERSION%.exe >> "%BUILDLOG%"
    powershell -NoProfile -Command "$p='%VERDIR%\ZPUI-Setup-%VERSION%.exe'; $c='%VERDIR%\checksums.sha256'; $h=(Get-FileHash $p -Algorithm SHA256).Hash.ToLower(); $sz=(Get-Item $p).Length; Add-Content -Path $c -Value ($h+'  ZPUI-Setup-%VERSION%.exe ('+$sz+' bytes)')"
    echo   [OK] installer checksum appended to checksums.sha256
    echo   [OK] installer checksum appended >> "%BUILDLOG%"
)

go build -trimpath -tags desktop,production -ldflags "-s -w -H windowsgui -X main.version=%VERSION%" -o "%VERDIR%\ZPUI-Update-%VERSION%.exe" .\cmd\updater >> "%BUILDLOG%" 2>&1
if errorlevel 1 (
    echo [ERROR] ZPUI-Update build failed >> "%BUILDLOG%"
    echo [ERROR] ZPUI-Update build failed
) else (
    echo   [OK] ZPUI-Update-%VERSION%.exe
    echo   [OK] ZPUI-Update-%VERSION%.exe >> "%BUILDLOG%"
    powershell -NoProfile -Command "$p='%VERDIR%\ZPUI-Update-%VERSION%.exe'; $c='%VERDIR%\checksums.sha256'; $h=(Get-FileHash $p -Algorithm SHA256).Hash.ToLower(); $sz=(Get-Item $p).Length; Add-Content -Path $c -Value ($h+'  ZPUI-Update-%VERSION%.exe ('+$sz+' bytes)')"
    echo   [OK] updater checksum appended to checksums.sha256
    echo   [OK] updater checksum appended >> "%BUILDLOG%"
)

REM syso больше не нужны
del /f /q "cmd\setup\resource_windows_386.syso" "cmd\setup\resource_windows_amd64.syso" "cmd\setup\resource_windows_arm.syso" "cmd\setup\resource_windows_arm64.syso" "cmd\setup\versioninfo.json" 2>nul
del /f /q "cmd\updater\resource_windows_386.syso" "cmd\updater\resource_windows_amd64.syso" "cmd\updater\resource_windows_arm.syso" "cmd\updater\resource_windows_arm64.syso" "cmd\updater\versioninfo.json" 2>nul
del /f /q "cmd\uninstall\resource_windows_386.syso" "cmd\uninstall\resource_windows_amd64.syso" "cmd\uninstall\resource_windows_arm.syso" "cmd\uninstall\resource_windows_arm64.syso" "cmd\uninstall\versioninfo.json" 2>nul
set "GOARCH="

REM NSIS-инсталляторы выведены из эксплуатации: ZPUI-Setup/ZPUI-Update
REM собираются выше как Wails-приложения (cmd/setup, cmd/updater).
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
if exist "%VERDIR%\x86\components\service\ZPUI-Service.exe" if exist "%VERDIR%\x64\components\service\ZPUI-Service.exe" (
    echo   ZPUI-Service.exe in x86\ and x64\ trees + versions.json
)
echo   versions.json
echo   checksums.sha256
echo.
if exist "%VERDIR%\ZPUI-Setup-%VERSION%.exe" (
    echo   Installer:
    for %%s in ("%VERDIR%\ZPUI-Setup-%VERSION%.exe") do echo     ZPUI-Setup-%VERSION%.exe  %%~zs bytes  - universal x86+x64
    if exist "%VERDIR%\ZPUI-Update-%VERSION%.exe" for %%s in ("%VERDIR%\ZPUI-Update-%VERSION%.exe") do echo     ZPUI-Update-%VERSION%.exe  %%~zs bytes  - silent updater
) else (
    echo   Installer: - not built
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
echo   ZPUI v%VERSION% (x86 + x64) + modules
echo ========================================
echo.
echo ================================================ >> "%BUILDLOG%"
echo   Build completed: %DATE% %TIME%                  >> "%BUILDLOG%"
echo ================================================ >> "%BUILDLOG%"
echo   Build log: %BUILDLOG%
echo.
echo   Press any key to close...
timeout /t 5 /nobreak > nul
rmdir /s /q "%BUILDLOCK%" 2>nul
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
(
echo {
echo   "FixedFileInfo": {
echo     "FileVersion": {"Major": %MOD_MAJ%, "Minor": %MOD_MIN%, "Patch": %MOD_PAT%, "Build": 0},
echo     "ProductVersion": {"Major": %PROD_MAJ%, "Minor": %PROD_MIN%, "Patch": %PROD_PAT%, "Build": 0}
echo   },
echo   "StringFileInfo": {
echo     "FileDescription": "%MOD_DESC%",
echo     "OriginalFilename": "ZPUI-%MOD_NAME%.exe",
echo     "ProductName": "ZPUI",
echo     "CompanyName": "SuzucaRU",
echo     "LegalCopyright": "Copyright (c) 2026 SuzucaRU. All rights reserved."
echo   },
echo   "IconPath": "..\\..\\build\\windows\\icon.ico"
echo }
) > versioninfo.json
"%GOVI%" -platform-specific versioninfo.json 2>nul
popd
if exist "cmd\%MOD_NAME%\resource_windows_386.syso" (
    echo   [OK] %MOD_NAME% syso ^(v%MOD_VER%^)
) else (
    echo   [WARN] %MOD_NAME% syso generation failed
)
goto :eof
