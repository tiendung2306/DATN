@echo off
REM Third peer. Instance đầu: cd app && wails dev  |  Node 2: dev-second-instance
cd /d "%~dp0.."
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0dev-third-instance.ps1" %*
