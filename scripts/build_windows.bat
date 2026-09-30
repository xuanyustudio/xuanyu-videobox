@echo off
echo ============================================================
echo   Narrato-GUI Build (AB Tab included via PowerShell runner)
echo ============================================================
echo.

powershell -NoProfile -ExecutionPolicy Bypass -File "%~dpn0.ps1"

echo.
pause
