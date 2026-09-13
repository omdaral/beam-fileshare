#!/bin/bash
# Beam .deb builder (Debian/Ubuntu): needs dist binary first (./build-all.sh).
# Usage: packaging/debian/build-deb.sh [amd64|arm64]   (output: dist/debian/*.deb)
# No root needed to BUILD; root needed only to INSTALL the result.
set -u
APP_DIR="$(cd "$(dirname "$0")/../.." && pwd)" || exit 1
cd "$APP_DIR" || exit 1
ARCH="${1:-amd64}"
case "$ARCH" in amd64|arm64) ;; *) echo "arch: amd64|arm64"; exit 1 ;; esac
command -v dpkg-deb >/dev/null 2>&1 || { echo "dpkg-deb missing"; exit 1; }
VER="$(tr -d ' \t\r\n' < VERSION 2>/dev/null)"
[ -z "$VER" ] && { echo "VERSION missing"; exit 1; }
BIN="dist/linux/$ARCH/$VER/Beam"
[ -x "$BIN" ] || { echo "binary missing: $BIN (run ./build-all.sh first)"; exit 1; }
[ -d packaging/icons ] || ./packaging/icons.sh

PKG="$(mktemp -d)/beam"
mkdir -p "$PKG/DEBIAN" "$PKG/usr/libexec/beam" "$PKG/usr/bin" \
         "$PKG/usr/share/applications" "$PKG/usr/share/doc/beam-fileshare"
cp "$BIN" "$PKG/usr/libexec/beam/Beam"
cp packaging/common/beam-wrapper.sh "$PKG/usr/bin/beam"
chmod 755 "$PKG/usr/libexec/beam/Beam" "$PKG/usr/bin/beam"
# VERSION fallback for LoadVersion (binary also carries ldflags version)
cp VERSION "$PKG/usr/share/doc/beam-fileshare/VERSION"
mkdir -p "$PKG/usr/share/man/man1"
gzip -9 -n -c packaging/common/beam.1 > "$PKG/usr/share/man/man1/beam.1.gz"
chmod 644 "$PKG/usr/share/man/man1/beam.1.gz"
for s in 16 22 24 32 48 64 128 256; do
  d="$PKG/usr/share/icons/hicolor/${s}x${s}/apps"
  mkdir -p "$d"
  cp "packaging/icons/beam_${s}.png" "$d/beam.png"
done
cat > "$PKG/usr/share/applications/beam-fileshare.desktop" <<EOF
[Desktop Entry]
Version=1.0
Type=Application
Name=Beam
Name[en]=Beam
Comment=Share files between your devices over a private network - offline
Comment[ar]=مشاركة الملفات بين أجهزتك عبر شبكة خاصة - بدون إنترنت
Exec=/usr/bin/beam
Icon=beam
Terminal=false
StartupNotify=true
Categories=Network;FileTransfer;
Keywords=share;files;hotspot;beam;
StartupWMClass=beam
EOF
cat > "$PKG/DEBIAN/control" <<EOF
Package: beam-fileshare
Version: $VER-1
Architecture: $ARCH
Maintainer: Beam Project <beam-fileshare@localhost>
Priority: optional
Section: net
Description: Direct file sharing between your devices over a private network
 Beam shares files between phones and computers on the same network,
 no internet and no accounts needed. Single static binary with an
 offline Arabic/English web UI.
 .
 مشاركة مباشرة للملفات بين الأجهزة على نفس الشبكة بدون إنترنت.
Depends: curl, xdg-utils
Recommends: libnotify-bin
EOF
BDATE="$(date -R)"
if [ -n "${SOURCE_DATE_EPOCH:-}" ]; then
  BDATE="$(date -u -d "@$SOURCE_DATE_EPOCH" -R 2>/dev/null || date -R)"
fi
cat > "$PKG/usr/share/doc/beam-fileshare/changelog.Debian" <<EOF2
beam-fileshare ($VER) stable; urgency=medium

  * Upstream $VER (see README.md).

 -- Beam Project <beam-fileshare@localhost>  $BDATE
EOF2
gzip -9 -n "$PKG/usr/share/doc/beam-fileshare/changelog.Debian"
mkdir -p "$PKG/usr/share/lintian/overrides"
cat > "$PKG/usr/share/lintian/overrides/beam-fileshare" <<EOF3
beam-fileshare binary: statically-linked-binary [usr/libexec/beam/Beam]
EOF3
cat > "$PKG/usr/share/doc/beam-fileshare/copyright" <<EOF
Copyright (c) 2026 Beam Project. All rights reserved.
Beam file sharing — packaged from local source.
TODO(packaging): add upstream LICENSE file and reference it here.
EOF
mkdir -p dist/debian
rm -f "dist/debian/beam-fileshare_${VER}_"*.deb
OUT="dist/debian/beam-fileshare_${VER}-1_${ARCH}.deb"
dpkg-deb --root-owner-group --build "$PKG" "$OUT" >/dev/null || { echo "dpkg-deb failed"; exit 1; }
echo "Built: $OUT ($(du -h "$OUT" | cut -f1))"
echo "--- info ---"; dpkg-deb --info "$OUT" | head -12
echo "--- contents ---"; dpkg-deb -c "$OUT"
if command -v lintian >/dev/null 2>&1; then
  echo "--- lintian ---"; lintian "$OUT" || true
else
  echo "(lintian not installed — skipped)"
fi
if command -v desktop-file-validate >/dev/null 2>&1; then
  desktop-file-validate "$PKG/usr/share/applications/beam-fileshare.desktop" && echo "desktop: valid ✅"
fi
