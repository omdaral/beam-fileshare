@echo off
REM Beam - launcher for Windows (single Go binary, no install).
REM Double-click: starts the server if down (it opens the browser itself),
REM otherwise opens the web UI. Fixed port 2004 - no side files.
setlocal
cd /d "%~dp0"

set PORT=
set NOBROWSER=0
:parse
if "%~1"=="" goto done
if "%~1"=="--port" set PORT=%~2 & shift & shift & goto parse
if "%~1"=="--no-browser" set NOBROWSER=1 & shift & goto parse
shift
goto parse
:done

if "%PORT%"=="" set PORT=2004
set PORT=%PORT: =%
set PORT=%PORT:,=%

set BIN=
if exist "%~dp0Beam.exe" set BIN=%~dp0Beam.exe
if not defined BIN if exist "%~dp0FileShare.exe" set BIN=%~dp0FileShare.exe

curl -s -o nul --max-time 2 http://127.0.0.1:%PORT%/health >nul 2>&1
if errorlevel 1 (
  if not defined BIN (
    echo Beam.exe is missing.
    echo Fix: build once with build.bat (needs Go on the build machine only).
    pause
    exit /b 1
  )
  if "%NOBROWSER%"=="0" (
    start "" /min "%BIN%" --port %PORT%
  ) else (
    start "" /min "%BIN%" --port %PORT% --no-browser
  )
  for /l %%i in (1,1,20) do (
    timeout /t 1 /nobreak >nul 2>&1
    curl -s -o nul --max-time 2 http://127.0.0.1:%PORT%/health >nul 2>&1
    if not errorlevel 1 goto up
  )
  echo Server did not start on port %PORT%. The port may be busy - stop the old copy and retry.
  pause
  exit /b 1
) else (
  if "%NOBROWSER%"=="0" start "" http://127.0.0.1:%PORT%/
  goto end
)
:up
rem Server just started above and opens the browser itself - nothing to do.
:end
