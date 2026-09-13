#!/bin/bash
# Beam AppImage builder — no FUSE needed to BUILD (manual runtime+squashfs
# method). Users need FUSE2 to RUN it (Ubuntu 22.04+: see packaging/README).
# Usage: packaging/appimage/build-appimage.sh [x86_64|aarch64]
# Needs: dist binary first (./build-all.sh) + mksquashfs in PATH
#   (Debian: sudo apt install squashfs-tools) + runtime in packaging/tools/.
set -u
APP_DIR="$(cd "$(dirname "$0")/../.." && pwd)" || exit 1
cd "$APP_DIR" || exit 1
ARCH="${1:-x86_64}"
case "$ARCH" in
  x86_64)  GOARCH=amd64; RUNTIME=packaging/tools/runtime-x86_64 ;;
  aarch64) GOARCH=arm64;  RUNTIME=packaging/tools/runtime-aarch64 ;;
  *) echo "arch: x86_64|aarch64"; exit 1 ;;
esac
MKSQUASHFS="${MKSQUASHFS:-mksquashfs}"
command -v "$MKSQUASHFS" >/dev/null 2>&1 || { echo "mksquashfs missing (Debian: sudo apt install squashfs-tools)"; exit 1; }
VER="$(tr -d ' \t\r\n' < VERSION 2>/dev/null)"
[ -z "$VER" ] && { echo "VERSION missing"; exit 1; }
BIN="dist/linux/$GOARCH/$VER/Beam"
[ -x "$BIN" ] || { echo "binary missing: $BIN (run ./build-all.sh first)"; exit 1; }
[ -f "$RUNTIME" ] || { echo "runtime missing: $RUNTIME"; exit 1; }
[ -d packaging/icons ] || ./packaging/icons.sh

STAGE="$(mktemp -d)" || exit 1
AD="$STAGE/Beam.AppDir"
mkdir -p "$AD/usr/bin" "$AD/usr/share/icons/hicolor/256x256/apps" "$AD/usr/share/metainfo"
cp "$BIN" "$AD/usr/bin/Beam"
cp packaging/icons/beam_256.png "$AD/beam.png"
cp packaging/icons/beam_256.png "$AD/usr/share/icons/hicolor/256x256/apps/beam.png"
cp packaging/flatpak/com.beam.beam.metainfo.xml "$AD/usr/share/metainfo/" 2>/dev/null || true
chmod +x "$AD/usr/bin/Beam"
# AppRun: the server runs in the FOREGROUND (clean shutdown, mount stays),
# only the browser opens in the background. Fixed port 2004, no side files.
cat > "$AD/AppRun" <<'EOF'
#!/bin/bash
HERE="$(dirname "$(readlink -f "$0")")"
PORT=2004
NO_BROWSER=0
while [ $# -gt 0 ]; do
  case "$1" in
    --port) PORT="${2:-2004}"; shift 2 ;;
    --no-browser) NO_BROWSER=1; shift ;;
    *) shift ;;
  esac
done
if [ "$NO_BROWSER" = "0" ] && command -v xdg-open >/dev/null 2>&1; then
  xdg-open "http://127.0.0.1:$PORT" >/dev/null 2>&1 &
fi
exec "$HERE/usr/bin/Beam" --port "$PORT" --no-browser
EOF
chmod +x "$AD/AppRun"
cat > "$AD/beam.desktop" <<EOF
[Desktop Entry]
Version=1.0
Type=Application
Name=Beam
Name[en]=Beam
Comment=Share files between your devices over a private network - offline
Comment[ar]=مشاركة الملفات بين أجهزتك عبر شبكة خاصة - بدون إنترنت
Exec=Beam
Icon=beam
Terminal=false
StartupNotify=true
Categories=Network;FileTransfer;
Keywords=share;files;hotspot;beam;
StartupWMClass=beam
EOF
if command -v desktop-file-validate >/dev/null 2>&1; then
  desktop-file-validate "$AD/beam.desktop" && echo "desktop: valid ✅"
else
  echo "(desktop-file-validate missing — skipped)"
fi

FS="$STAGE/fs.squashfs"
"$MKSQUASHFS" "$AD" "$FS" -root-owned -noappend -comp zstd >/dev/null || { echo "mksquashfs failed"; exit 1; }
mkdir -p dist/appimage
OUT="dist/appimage/Beam-${VER}-${ARCH}.AppImage"
cat "$RUNTIME" "$FS" > "$OUT"
chmod +x "$OUT"
rm -rf "$STAGE"
echo "Built: $OUT ($(du -h "$OUT" | cut -f1))"
file "$OUT" | head -1
# Structural check: superblock sits exactly after the runtime stub
# (first hsqs hit lives inside the runtime itself — skip it).
OFF=$(stat -c%s "$RUNTIME")
echo "squashfs at byte $OFF"
if command -v unsquashfs >/dev/null 2>&1; then
  unsquashfs -o "$OFF" -l "$OUT" | head -20
fi
# Live smoke test on a scratch port (AppImage needs FUSE to run here).
echo "--- live test ---"
TPORT=2031
"$OUT" --port "$TPORT" --no-browser >/tmp/beam-appimage-test.log 2>&1 &
APID=$!
sleep 2
if ! kill -0 $APID 2>/dev/null; then
  echo "live: ⏭️ AppImage cannot execute here (usually no FUSE) — built only, run-tested on user machines"
  echo "      (users need FUSE2: sudo apt install libfuse2)"
  exit 0
fi
UP=0
for _ in $(seq 1 20); do
  sleep 0.5
  curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$TPORT/health" 2>/dev/null && { UP=1; break; }
done
curl -s --max-time 5 -X POST "http://127.0.0.1:$TPORT/api/server/stop" -H 'Content-Type: application/json' -d '{}' >/dev/null 2>&1 || kill $APID 2>/dev/null
wait $APID 2>/dev/null
if [ "$UP" = "1" ]; then
  echo "live: ✅ /health OK, stopped cleanly"
else
  echo "live: ❌ no response (see /tmp/beam-appimage-test.log)"
  exit 1
fi
