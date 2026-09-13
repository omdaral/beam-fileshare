#!/bin/bash
# Beam — macOS launcher (double-click Beam.command): starts the server
# if down (it opens the browser by itself), otherwise opens the page.
# Fixed port 2004 — no side files. macOS runs LAN mode (hotspot is Linux/Windows).
# Usage: ./Beam.command [--port 2004] [--no-browser]
set -u
APP_DIR="$(cd "$(dirname "$0")" && pwd)" || { echo "Cannot open app folder"; exit 1; }
cd "$APP_DIR" || exit 1

PORT=""
NO_BROWSER=0
while [ $# -gt 0 ]; do
  case "$1" in
    --port) PORT="${2:-}"; shift 2 ;;
    --no-browser) NO_BROWSER=1; shift ;;
    *) shift ;;
  esac
done
[ -z "$PORT" ] && PORT=2004

BIN=""
for c in "$APP_DIR/Beam" "$APP_DIR/FileShare" "$APP_DIR/Beam-darwin-arm64" "$APP_DIR/Beam-darwin-amd64"; do
  if [ -x "$c" ]; then BIN="$c"; break; fi
done

health() { curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$PORT/health" 2>/dev/null; }

WAS_DOWN=0
if ! health; then
  WAS_DOWN=1
fi

if [ "$WAS_DOWN" = "1" ]; then
  [ -z "$BIN" ] && { echo "Beam binary not found next to Beam.command."; exit 1; }
  if [ "$NO_BROWSER" = "0" ]; then
    nohup "$BIN" --port "$PORT" >>/tmp/beam-server.log 2>&1 &
  else
    nohup "$BIN" --port "$PORT" --no-browser >>/tmp/beam-server.log 2>&1 &
  fi
  for _ in $(seq 1 20); do
    sleep 0.5
    health && break
  done
fi

if ! health; then
  echo "Could not start the server on port $PORT. See /tmp/beam-server.log"
  exit 1
fi

SHARE_URL="$(curl -s --max-time 5 "http://127.0.0.1:$PORT/api/status" 2>/dev/null | grep -o '"url":"[^"]*"' | head -1 | cut -d'"' -f4)"
[ -z "$SHARE_URL" ] && SHARE_URL="http://127.0.0.1:$PORT"
echo "Join URL: $SHARE_URL"

if [ "$WAS_DOWN" = "0" ] && [ "$NO_BROWSER" = "0" ]; then
  open "http://127.0.0.1:$PORT" >/dev/null 2>&1 &
fi
