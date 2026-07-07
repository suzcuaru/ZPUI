@echo off
setlocal

set "BAT_DIR=%~dp0"
cd /d "%BAT_DIR%"

echo ========================================
echo   ZPUI Release Build (local only)
echo ========================================
echo.

call "%BAT_DIR%build.bat"
if errorlevel 1 (echo [ERROR] Build failed & exit /b 1)

echo.
echo ========================================
echo   Build complete.
echo   Output in: dist\
echo.
echo   Push to git manually when ready:
echo     git add dist/ version.txt wails.json
echo     git commit -m "release: vX.X.X"
echo     git tag vX.X.X
echo     git push origin BRANCH --tags
echo ========================================
echo.
timeout /t 5 /nobreak > nul
