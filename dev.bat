@echo off
setlocal
cd /d "%~dp0web"

where node >nul 2>&1 || (echo [ERROR] Node.js not found & timeout /t 5 >nul & exit /b 1)

echo ============================================
echo   ZPUI frontend — visual dev mode
echo   Mock backend (no Go server needed)
echo   http://localhost:3000  (F12 = devtools)
echo   Ctrl+C to stop
echo ============================================
echo.

REM Open browser shortly after the dev server starts.
start /b cmd /c "timeout /t 3 >nul & start http://localhost:3000"

call npm run dev
