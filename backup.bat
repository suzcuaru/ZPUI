@echo off
REM ZPUI project backup -> central vault ZPUI-Backups\zpui (rotation: 5 raw snapshots + 5 zip archives)
REM Vault control: ..\backup-all.bat (all projects) / ..\backup.ps1 (engine)
setlocal
set "PS1=%~dp0..\backup.ps1"
powershell -NoProfile -ExecutionPolicy Bypass -File "%PS1%" -Project zpui
endlocal & exit /b %errorlevel%
