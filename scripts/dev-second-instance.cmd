@echo off
REM Second peer only. Run first peer with: cd app && wails dev
cd /d "%~dp0.."
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0dev-second-instance.ps1" %*
