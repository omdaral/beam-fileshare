#!/bin/bash
# Beam Flatpak entrypoint (/app/bin/beam): server in foreground under
# Flatpak supervision + browser opened once via xdg-open (OpenURI portal).
# No side files: fixed port 2004, settings + log in memory,
# shared files in ~/Downloads/Beam (see xdg-download in the manifest).
set -u
PORT=2004
while [ $# -gt 0 ]; do
  case "$1" in
    --port) PORT="$2"; shift 2 ;;
    *) shift ;;
  esac
done
/app/bin/Beam --port "$PORT" --no-browser &
SRV=$!
for _ in $(seq 1 20); do
  sleep 0.5
  curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$PORT/health" 2>/dev/null && break
done
xdg-open "http://127.0.0.1:$PORT" >/dev/null 2>&1 &
wait "$SRV"
