@echo off
setlocal
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\setup.ps1"
set "result=%ERRORLEVEL%"
echo.
pause
exit /b %result%
