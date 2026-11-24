@echo off
rem Compile AzimovLauncher.exe (Go 1.22+ requis : https://go.dev/dl/)
cd /d "%~dp0"
go build -trimpath -ldflags "-H windowsgui -s -w" -o AzimovLauncher.exe .
if errorlevel 1 (
  echo.
  echo La compilation a echoue.
  pause
  exit /b 1
)
echo.
echo OK : AzimovLauncher.exe
pause
