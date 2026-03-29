@echo off
title ghs installer
powershell -ExecutionPolicy Bypass -NoProfile -File "%~dp0install-ghs.ps1"
if %errorlevel% neq 0 (
    echo.
    echo Installation failed.
)
pause
