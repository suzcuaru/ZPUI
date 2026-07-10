@echo off
setlocal enabledelayedexpansion

set "BAT_DIR=%~dp0"
cd /d "%BAT_DIR%"

REM --- Read version from version.txt ---
if not exist "%BAT_DIR%version.txt" (
    echo [ERROR] version.txt not found. Run build.bat first.
    timeout /t 10 > nul
    exit /b 1
)
set /p VERSION=<"%BAT_DIR%version.txt"
set "TAG=v%VERSION%"

echo ========================================
echo   ZPUI Release %TAG%
echo   GitHub release publisher
echo ========================================
echo.

REM === STEP 1: Verify build artifacts ===
echo [1/7] Verifying build artifacts...
if not exist "%BAT_DIR%dist\zpui.exe" (
    echo [ERROR] dist\zpui.exe not found. Run build.bat first.
    timeout /t 10 > nul
    exit /b 1
)
echo   [OK] Build artifacts found in dist\
echo.

REM === STEP 2: Check tag does not exist ===
echo [2/7] Checking tag %TAG%...
git rev-parse "%TAG%" >nul 2>&1
if not errorlevel 1 (
    echo [ERROR] Tag %TAG% already exists locally.
    timeout /t 10 > nul
    exit /b 1
)
echo   [OK] Tag %TAG% is available
echo.

REM === STEP 3: Find and verify gh CLI ===
echo [3/7] Checking GitHub CLI...
set "GH="
for /f "delims=" %%A in ('where gh 2^>nul') do (set "GH=%%A")
if not defined GH if exist "C:\Program Files\GitHub CLI\gh.exe" set "GH=C:\Program Files\GitHub CLI\gh.exe"
if not defined GH (
    echo [ERROR] GitHub CLI ^(gh^) not found.
    echo         Install from https://cli.github.com/
    timeout /t 10 > nul
    exit /b 1
)
"%GH%" auth status >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Not authenticated with GitHub.
    echo         Run: gh auth login
    timeout /t 10 > nul
    exit /b 1
)
echo   [OK] gh CLI found and authenticated
echo.

REM === STEP 4: Create portable zip ===
echo [4/7] Creating portable zip...
set "ZIP_NAME=ZPUI-%VERSION%-win32-portable.zip"
set "ZIP_PATH=%BAT_DIR%build\%ZIP_NAME%"
if exist "%ZIP_PATH%" del /f /q "%ZIP_PATH%"
powershell -NoProfile -Command "Compress-Archive -Path '%BAT_DIR%dist\*' -DestinationPath '%ZIP_PATH%' -Force"
if not exist "%ZIP_PATH%" (
    echo [ERROR] Failed to create portable zip.
    timeout /t 10 > nul
    exit /b 1
)
echo   [OK] %ZIP_NAME%
echo.

REM === STEP 5: Git commit + tag ===
echo [5/7] Committing and tagging...
git add version.txt wails.json dist/
git commit -m "release: %TAG%" >nul 2>&1
if errorlevel 1 (
    echo   [WARN] Nothing new to commit ^(already committed^)
) else (
    echo   [OK] Committed: release: %TAG%
)
git tag "%TAG%"
if errorlevel 1 (
    echo [ERROR] Failed to create tag %TAG%.
    timeout /t 10 > nul
    exit /b 1
)
echo   [OK] Tag %TAG% created
echo.

REM === STEP 6: Push to GitHub ===
echo [6/7] Pushing to origin...
git push origin HEAD --tags
if errorlevel 1 (
    echo [ERROR] Failed to push to origin.
    timeout /t 10 > nul
    exit /b 1
)
echo   [OK] Pushed
echo.

REM === STEP 7: Create GitHub Release ===
echo [7/7] Creating GitHub release...
set "INSTALLER=%BAT_DIR%dist\ZPUI-Setup-%VERSION%-win32.exe"

if exist "%INSTALLER%" (
    "%GH%" release create "%TAG%" --title "%TAG%" --generate-notes "%INSTALLER%" "%ZIP_PATH%"
) else (
    "%GH%" release create "%TAG%" --title "%TAG%" --generate-notes "%ZIP_PATH%"
)
if errorlevel 1 (
    echo [ERROR] Failed to create GitHub release.
    echo         You can create it manually:
    echo         gh release create %TAG% --title %TAG% --generate-notes
    timeout /t 10 > nul
    exit /b 1
)
echo   [OK] Release %TAG% published
echo.

REM --- Summary ---
echo ========================================
echo   Release %TAG% published!
echo.
echo   Assets uploaded:
if exist "%INSTALLER%" (
    echo     - ZPUI-Setup-%VERSION%-win32.exe ^(installer^)
) else (
    echo     - ^(installer not built - NSIS missing^)
)
echo     - %ZIP_NAME% ^(portable^)
echo.
echo   https://github.com/suzcuaru/ZPUI/releases/tag/%TAG%
echo ========================================
echo.
echo   Press any key to close...
timeout /t 5 /nobreak > nul
