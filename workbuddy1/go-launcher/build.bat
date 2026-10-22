@echo off
rem Truce Minecraft Launcher - build single app (pure Go, zero CGO, no WebView)
rem ASCII only; Chinese paths expanded at runtime via %USERPROFILE%.
setlocal

set GOPROXY=https://goproxy.cn,direct
set CGO_ENABLED=0
set GOFLAGS=-mod=mod

where go >nul 2>nul
if %errorlevel%==0 (
  set GO=go
) else (
  set GO=%USERPROFILE%\.workbuddy\binaries\go\go\bin\go.exe
)

if not exist "%GO%" (
  echo [ERROR] go not found. Install Go or add it to PATH.
  exit /b 1
)

if not exist resource.syso (
  echo [WARN] resource.syso missing; icon/version info will be absent.
)

echo [BUILD] building single app dist\TruceLauncher.exe ...
"%GO%" build -ldflags "-H windowsgui" -o dist\TruceLauncher.exe .
if %errorlevel%==0 (
  echo [OK] dist\TruceLauncher.exe ready
) else (
  echo [FAIL] build error
  exit /b 1
)
endlocal
