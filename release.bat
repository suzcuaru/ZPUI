@echo off
setlocal enabledelayedexpansion

set "BAT_DIR=%~dp0"
cd /d "%BAT_DIR%"

REM === Read version from versions.txt ===
set "VERSION="

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
)

if not defined VERSION (
    echo [ERROR] zpui version not found in versions.txt
    timeout /t 10 > nul
    exit /b 1
)

echo ========================================
echo   ZPUI Release v%VERSION%
echo ========================================
echo.

REM === [1/7] Preflight: dist artifacts must exist ===
echo [1/7] Preflight checks...
set "VERDIR=%BAT_DIR%dist\%VERSION%"
if not exist "%VERDIR%" (
    echo [ERROR] dist\%VERSION%\ not found. Run build.bat first.
    timeout /t 10 > nul
    exit /b 1
)
if not exist "%VERDIR%\ZPUI.exe" (
    echo [ERROR] dist\%VERSION%\ZPUI.exe not found. Build is incomplete.
    timeout /t 10 > nul
    exit /b 1
)
if not exist "%VERDIR%\versions.json" (
    echo [ERROR] dist\%VERSION%\versions.json not found. Build is incomplete.
    timeout /t 10 > nul
    exit /b 1
)
if not exist "%VERDIR%\ZPUI-Setup-%VERSION%-win32.exe" (
    echo [ERROR] dist\%VERSION%\ZPUI-Setup-%VERSION%-win32.exe not found. Build is incomplete.
    timeout /t 10 > nul
    exit /b 1
)
if exist "%BAT_DIR%CHANGELOG.md" (
    findstr /b /c:"## v%VERSION%" "%BAT_DIR%CHANGELOG.md" > nul 2>&1
    if errorlevel 1 echo [WARN] CHANGELOG.md has no "## v%VERSION%" section - GitHub release notes will fall back to a plain title.
) else (
    echo [WARN] CHANGELOG.md not found - GitHub release notes will fall back to a plain title.
)
echo   Done.
echo.

REM === [2/7] Tag check: local and remote ===
echo [2/7] Checking tag v%VERSION%...
set "TAG_EXISTS=0"
git rev-parse -q --verify "refs/tags/v%VERSION%" > nul 2>&1
if not errorlevel 1 set "TAG_EXISTS=1"

set "REMOTE_TAG="
for /f "delims=" %%i in ('git ls-remote --tags origin "refs/tags/v%VERSION%" 2^>nul') do set "REMOTE_TAG=%%i"
if defined REMOTE_TAG set "TAG_EXISTS=1"

if "%TAG_EXISTS%"=="0" (
    echo   Tag v%VERSION% is new.
    goto after_tag
)
echo   Tag v%VERSION% already exists on this machine and/or on origin.
choice /c YN /m "  Re-upload? The existing GitHub release will be replaced"
if errorlevel 2 (
    echo Cancelled. Nothing was pushed.
    echo.
    echo   Press any key to close...
    pause > nul
    exit /b 0
)
if defined REMOTE_TAG (
    where gh > nul 2>&1
    if not errorlevel 1 (
        echo   Deleting existing GitHub release and tag via gh...
        gh release delete "v%VERSION%" --yes --cleanup-tag > nul 2>&1
        if errorlevel 1 (
            echo   [INFO] gh delete failed - CI will update the existing release in place.
        ) else (
            echo   [INFO] Old release and tag removed on GitHub - CI will recreate the release.
        )
    ) else (
        echo   [INFO] gh CLI not found - CI will update the existing release in place.
    )
)
:after_tag
echo.

REM === [3/7] Refresh repository root from dist ===
echo [3/7] Refreshing repository root from dist\%VERSION%\...
del /f /q "%BAT_DIR%ZPUI-Setup-*-win32.exe" > nul 2>&1
del /f /q "%BAT_DIR%report.exe" > nul 2>&1
xcopy /e /i /y /q "%VERDIR%\*" "%BAT_DIR%" > nul
if errorlevel 1 (
    echo [ERROR] Failed to copy files from dist\%VERSION%\
    timeout /t 10 > nul
    exit /b 1
)
REM === Remove copies of artifacts that CI does not use (they stay in dist) ===
del /f /q "%BAT_DIR%checksums.sha256" > nul 2>&1
del /f /q "%BAT_DIR%telegram_links.txt" > nul 2>&1
rd /s /q "%BAT_DIR%zapret" > nul 2>&1
rd /s /q "%BAT_DIR%components" > nul 2>&1
echo   Done.
echo.

REM === [4/7] Sanity: root must satisfy release.yml ===
echo [4/7] Sanity checks...
if not exist "%BAT_DIR%ZPUI.exe" (
    echo [ERROR] ZPUI.exe missing in repository root - CI would silently skip the release.
    timeout /t 10 > nul
    exit /b 1
)
if not exist "%BAT_DIR%versions.json" (
    echo [ERROR] versions.json missing in repository root - CI would silently skip the release.
    timeout /t 10 > nul
    exit /b 1
)
if not exist "%BAT_DIR%ZPUI-Setup-%VERSION%-win32.exe" (
    echo [ERROR] ZPUI-Setup-%VERSION%-win32.exe missing in repository root - CI would silently skip the release.
    timeout /t 10 > nul
    exit /b 1
)
echo   Done.
echo.

REM === [5/7] Stage and commit ===
echo [5/7] Staging changes...
git add -A
git diff --cached --quiet
if errorlevel 1 (
    echo   Committing v%VERSION%...
    git commit -m "v%VERSION%"
    if errorlevel 1 (
        echo [ERROR] Commit failed
        timeout /t 10 > nul
        exit /b 1
    )
) else (
    echo   No changes to commit.
)
echo.

REM === [6/7] Tag ===
echo [6/7] Creating tag v%VERSION%...
git tag -f "v%VERSION%"
if errorlevel 1 (
    echo [ERROR] Tag creation failed
    timeout /t 10 > nul
    exit /b 1
)
echo   Done.
echo.

REM === [7/7] Push: branch without force, tag with force ===
echo [7/7] Pushing to origin...
git push origin main
if errorlevel 1 (
    echo [ERROR] Push of main failed. Tag was NOT pushed, CI was not triggered.
    timeout /t 10 > nul
    exit /b 1
)
git push origin "refs/tags/v%VERSION%" --force
if errorlevel 1 (
    echo [ERROR] Tag push failed
    timeout /t 10 > nul
    exit /b 1
)
echo   Done.
echo.

echo ========================================
echo   Release v%VERSION% pushed!
echo   GitHub Actions will create the release
echo   with setup + portable from CHANGELOG.md
echo   Remember to upload the setup and
echo   checksums.sha256 to Yandex.Disk manually.
echo ========================================
echo.
echo   Press any key to close...
pause > nul
exit /b 0
