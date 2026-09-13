#!/bin/bash
# Beam launcher for system packages (deb/rpm): starts the server if down
# (it opens the browser itself), otherwise opens the web UI.
# No side files: fixed port 2004, settings in memory (+browser localStorage),
# log in memory, shared files in ~/Downloads/Beam.
set -u
PORT=2004
while [ $# -gt 0 ]; do
  case "$1" in
    --port) PORT="$2"; shift 2 ;;
    --no-browser) NO_BROWSER=1; shift ;;
    *) shift ;;
  esac
done
NO_BROWSER="${NO_BROWSER:-0}"

BIN=""
for c in /usr/libexec/beam/Beam /opt/beam/Beam /usr/local/bin/Beam; do
  if [ -x "$c" ]; then BIN="$c"; break; fi
done
[ -z "$BIN" ] && { echo "Beam binary not found. Reinstall the package."; exit 1; }

health() { curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$PORT/health" 2>/dev/null; }

WAS_DOWN=0
if ! health; then
  WAS_DOWN=1
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
  echo "Could not start Beam on port $PORT. See /tmp/beam-server.log"
  exit 1
fi

SHARE_URL="$(curl -s --max-time 5 "http://127.0.0.1:$PORT/api/status" 2>/dev/null | grep -o '"url":"[^"]*"' | head -1 | cut -d'"' -f4)"
[ -z "$SHARE_URL" ] && SHARE_URL="http://127.0.0.1:$PORT"
echo "Join URL: $SHARE_URL"
if command -v notify-send >/dev/null 2>&1; then
  notify-send "Beam" "Server running — join URL:\n$SHARE_URL" >/dev/null 2>&1 &
fi

if [ "$WAS_DOWN" = "0" ] && [ "$NO_BROWSER" = "0" ]; then
  if command -v xdg-open >/dev/null 2>&1; then
    xdg-open "http://127.0.0.1:$PORT" >/dev/null 2>&1 &
  else
    echo "Server running. Open your browser at: http://127.0.0.1:$PORT"
  fi
fi
