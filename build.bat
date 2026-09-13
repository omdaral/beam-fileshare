@echo off
REM Company Share - build Windows binary (run once on the build machine).
REM Needs Go 1.21+ (https://go.dev/dl), no downloads required (stdlib only).
REM Output: dist\windows\amd64\<version>\Beam.exe (full matrix via ./build-all.sh on Linux).
cd /d "%~dp0"
where go >nul 2>&1
if errorlevel 1 (
  echo Go is not installed. Install Go 1.21+ once, then run build.bat again.
  pause
  exit /b 1
)
set /p VER=<VERSION
set VER=%VER: =%
if "%VER%"=="" (
  echo VERSION file is missing.
  pause
  exit /b 1
)
go -C goserver vet ./...
if errorlevel 1 exit /b 1
go -C goserver test ./...
if errorlevel 1 exit /b 1
if not exist dist\windows\amd64\%VER% mkdir dist\windows\amd64\%VER%
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
go -C goserver build -trimpath -ldflags="-s -w -X fileshare.AppVersion=%VER%" -o ..\dist\windows\amd64\%VER%\Beam.exe ./cmd/beam
if errorlevel 1 exit /b 1
echo Built: dist\windows\amd64\%VER%\Beam.exe
pause
