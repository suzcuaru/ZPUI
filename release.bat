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

REM === Check dist folder exists ===
set "VERDIR=%BAT_DIR%dist\%VERSION%"
if not exist "%VERDIR%" (
    echo [ERROR] dist\%VERSION%\ not found. Run build.bat first.
    timeout /t 10 > nul
    exit /b 1
)

REM === Copy dist/{VERSION}/* to root ===
echo [1/5] Copying dist\%VERSION%\ to repository root...
xcopy /e /i /y /q "%VERDIR%\*" "%BAT_DIR%" > nul
if errorlevel 1 (
    echo [ERROR] Failed to copy files from dist\%VERSION%\
    timeout /t 10 > nul
    exit /b 1
)
echo   Done.
echo.

REM === Stage all changes ===
echo [2/5] Staging changes...
git add -A
echo   Done.
echo.

REM === Check if there are changes to commit ===
git diff --cached --quiet
if errorlevel 1 (
    REM === Commit ===
    echo [3/5] Committing v%VERSION%...
    git commit -m "v%VERSION%"
    if errorlevel 1 (
        echo [ERROR] Commit failed
        timeout /t 10 > nul
        exit /b 1
    )
    echo   Done.
    echo.
) else (
    echo [3/5] No changes to commit.
    echo.
)

REM === Create tag ===
echo [4/5] Creating tag v%VERSION%...
git tag -f "v%VERSION%"
if errorlevel 1 (
    echo [ERROR] Tag creation failed
    timeout /t 10 > nul
    exit /b 1
)
echo   Done.
echo.

REM === Push ===
echo [5/5] Pushing to origin...
git push origin main --force --tags
if errorlevel 1 (
    echo [ERROR] Push failed
    timeout /t 10 > nul
    exit /b 1
)
echo   Done.
echo.

echo ========================================
echo   Release v%VERSION% pushed!
echo   GitHub Actions will create the release
echo   with setup + portable from CHANGELOG.md
echo ========================================
echo.
echo   Press any key to close...
pause > nul
exit /b 0
