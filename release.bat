@echo off
setlocal enabledelayedexpansion

set "BAT_DIR=%~dp0"
cd /d "%BAT_DIR%"

echo ========================================
echo   ZPUI Release Script
echo   Build locally ^> commit dist/ ^> tag ^> push
echo ========================================
echo.

REM --- Step 1: Build ---
echo [1/4] Building...
call "%BAT_DIR%build.bat"
if errorlevel 1 (echo [ERROR] Build failed & exit /b 1)
echo.

REM --- Step 2: Read version ---
set /p VERSION=<"%BAT_DIR%version.txt"
set "TAG=v%VERSION%"
echo [2/4] Version: %VERSION% (tag: %TAG%)
echo.

REM --- Step 3: Commit dist/ ---
echo [3/4] Committing dist/ to repo...
git add dist/ version.txt wails.json
git commit -m "release: v%VERSION%"
if errorlevel 1 (echo [WARN] Nothing to commit or commit failed)
echo.

REM --- Step 4: Tag and push ---
echo [4/4] Creating tag %TAG% and pushing...
git tag -d "%TAG%" 2>nul
git tag "%TAG%"
git push origin main
git push origin "%TAG%" --force
if errorlevel 1 (echo [ERROR] Push failed & exit /b 1)

echo.
echo ========================================
echo   Release %TAG% pushed!
echo   GitHub Actions will create the release.
echo ========================================
echo.
timeout /t 5 /nobreak > nul
