@echo off

echo Starting ZPUI Web Interface development server...
echo.

REM Check if Node.js is installed
where node >nul 2>nul
if %ERRORLEVEL% neq 0 (
    echo Node.js is not installed. Please install Node.js and try again.
    exit /b 1
)

REM Check if npm is installed
where npm >nul 2>nul
if %ERRORLEVEL% neq 0 (
    echo npm is not installed. Please install npm and try again.
    exit /b 1
)

REM Install dependencies
echo Installing dependencies...
npm install
if %ERRORLEVEL% neq 0 (
    echo Failed to install dependencies.
    exit /b 1
)

REM Start development server
echo Starting development server on http://localhost:3000...
npm run dev
