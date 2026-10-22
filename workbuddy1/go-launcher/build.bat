@echo off
rem Truce Minecraft Launcher - build the single desktop app (WPF UI + Go core)
rem ASCII only: cmd.exe parses this file as GBK, non-ASCII would corrupt it.
rem Chinese paths are expanded at runtime via %USERPROFILE% / %ProgramW6432%.
setlocal

set "ROOT=%~dp0"
set "ROOTDIR=%ROOT:~0,-1%"
set "GOCORE=%ROOT%wpf\TruceLauncher.App\Resources\core.exe"
set "PROJ=%ROOT%wpf\TruceLauncher.App\TruceLauncher.App.csproj"
set "OUT=%ROOT%dist\publish"

rem ---- locate go.exe (managed runtime first, then PATH) ----
set "GOEXE="
if exist "%USERPROFILE%\.workbuddy\binaries\go\go\bin\go.exe" set "GOEXE=%USERPROFILE%\.workbuddy\binaries\go\go\bin\go.exe"
if not defined GOEXE (
  for %%g in (go.exe) do if not "%%~$PATH:g"=="" set "GOEXE=%%~$PATH:g"
)

rem ---- locate dotnet.exe ----
set "DOTNET="
if exist "%ProgramW6432%\dotnet\dotnet.exe" set "DOTNET=%ProgramW6432%\dotnet\dotnet.exe"
if not defined DOTNET if exist "%ProgramFiles%\dotnet\dotnet.exe" set "DOTNET=%ProgramFiles%\dotnet\dotnet.exe"
if not defined DOTNET (
  for %%d in (dotnet.exe) do if not "%%~$PATH:d"=="" set "DOTNET=%%~$PATH:d"
)

if not defined GOEXE (
  echo [ERROR] go.exe not found. Install Go or add it to PATH.
  exit /b 1
)
if not defined DOTNET (
  echo [ERROR] dotnet.exe not found. Install .NET 8 SDK or later.
  exit /b 1
)

echo [1/3] Building Go core ...
set "GOPROXY=https://goproxy.cn,direct"
set "GOFLAGS=-mod=mod"
set "CGO_ENABLED=0"
"%GOEXE%" -C "%ROOTDIR%" build -ldflags "-H windowsgui" -o "%GOCORE%" .
if errorlevel 1 goto :fail

echo [2/3] Publishing WPF UI as single exe ...
"%DOTNET%" publish "%PROJ%" -c Release -r win-x64 --self-contained true -p:PublishSingleFile=true -p:IncludeNativeLibrariesForSelfExtract=true -p:EnableCompressionInSingleFile=true -o "%OUT%"
if errorlevel 1 goto :fail

echo [3/3] Done.
echo Output: %OUT%\TruceLauncher.exe
for %%f in ("%OUT%\TruceLauncher.exe") do echo Size:   %%~zf bytes
endlocal
exit /b 0

:fail
echo [FAIL] build failed.
endlocal
exit /b 1
