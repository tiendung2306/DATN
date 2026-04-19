@echo off
REM Fourth peer. Instance đầu: cd app && wails dev  |  Node 2/3: scripts tương ứng
cd /d "%~dp0.."
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0dev-fourth-instance.ps1" %*
