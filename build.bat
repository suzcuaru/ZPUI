@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion

set APP_NAME=zpui
set PLATFORM=windows/amd64
set BUILD_DIR=build\bin

if /i "%1"=="devtools" (
    set DEVTOOLS=-devtools
    echo [BUILD] Сборка с DevTools (режим тестирования)
) else if /i "%1"=="release" (
    set DEVTOOLS=-s -trimpath
    echo [BUILD] Релизная сборка
) else if /i "%1"=="clean" (
    if exist "%BUILD_DIR%" rmdir /s /q "%BUILD_DIR%"
    if exist "web\dist" rmdir /s /q "web\dist"
    echo [BUILD] Очищено
    exit /b 0
) else (
    set DEVTOOLS=-devtools
    echo [BUILD] Сборка с DevTools (по умолчанию)
)

if not exist "%BUILD_DIR%" mkdir "%BUILD_DIR%"

echo [BUILD] Фронтенд...
pushd web
call npm run build
if %errorlevel% neq 0 (
    echo [BUILD] Ошибка фронтенда
    popd
    exit /b 1
)
popd

echo [BUILD] Бэкенд (%PLATFORM%)...
set GOARCH=amd64
set GOOS=windows
wails build -platform %PLATFORM% %DEVTOOLS% -o "%BUILD_DIR%\%APP_NAME%.exe"
if %errorlevel% neq 0 (
    echo [BUILD] Ошибка сборки
    exit /b 1
)

echo [BUILD] Готово: %BUILD_DIR%\%APP_NAME%.exe
exit /b 0
