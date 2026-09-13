#!/bin/bash
# Assemble the Capacitor webDir: a tiny launcher page (NOT a copy of the app).
# The real Beam page always loads FROM the Beam server (same LAN), so every
# API/asset path stays same-origin and the app can never drift from the web UI.
# Usage: ./sync-web.sh  (from mobile/)
set -u
cd "$(dirname "$0")" || exit 1
rm -rf www
mkdir -p www
cp src/launcher.html www/index.html
cp ../goserver/web/qrcode-vendor.js www/qrcode.js
cp ../icon.png www/icon.png
echo "www/ assembled (launcher only — app loads live from Beam server) ✅"
ls www/
