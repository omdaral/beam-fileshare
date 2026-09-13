#!/bin/bash
# Beam — بناء كل حزم التوزيع من شجرة dist/ الجاهزة (./build-all.sh أولاً).
#   Debian/Ubuntu: dist/debian/*.deb            (dpkg-deb — يعمل هنا)
#   AppImage:      dist/appimage/*.AppImage     (يحتاج mksquashfs)
#   Fedora .rpm: dist/rpm/*.rpm               (needs rpmbuild)
#   Arch: PKGBUILD جاهز (البناء على Arch) + Flatpak: manifest جاهز
# الاستخدام: packaging/build-packages.sh [amd64|arm64|all]
# غير قاتل: أي حزمة تتعثر تُسجَّل ويُكمل الباقي (الخروج 1 في النهاية للتنبيه فقط).
set -u
APP_DIR="$(cd "$(dirname "$0")/.." && pwd)" || exit 1
cd "$APP_DIR" || exit 1
ARCHSEL="${1:-all}"
case "$ARCHSEL" in amd64|arm64) GOARCHS="$ARCHSEL" ;; all) GOARCHS="amd64 arm64" ;; *) echo "arch: amd64|arm64|all"; exit 1 ;; esac
VER="$(tr -d ' \t\r\n' < VERSION 2>/dev/null)"
[ -z "$VER" ] && { echo "VERSION missing"; exit 1; }
FAIL=0

for GOARCH in $GOARCHS; do
  case "$GOARCH" in
    amd64) DEBARCH=amd64; AIMGARCH=x86_64 ;;
    arm64) DEBARCH=arm64; AIMGARCH=aarch64 ;;
  esac
  echo "—— $GOARCH ——"
  if [ ! -x "dist/linux/$GOARCH/$VER/Beam" ]; then
    echo "⚠️ binary missing: dist/linux/$GOARCH/$VER/Beam (run ./build-all.sh)"; FAIL=1; continue
  fi
  BLOG="/tmp/beam-build.log"
  echo "[deb]"; ./packaging/debian/build-deb.sh "$DEBARCH" >>"$BLOG" 2>&1 \
    && echo "  ✅ deb $DEBARCH" || { echo "  ❌ deb $DEBARCH ($BLOG)"; FAIL=1; }
  echo "[appimage]"
  if command -v mksquashfs >/dev/null 2>&1; then
    ./packaging/appimage/build-appimage.sh "$AIMGARCH" >>"$BLOG" 2>&1 \
      && echo "  ✅ appimage $AIMGARCH" || { echo "  ❌ appimage $AIMGARCH ($BLOG)"; FAIL=1; }
  else
    echo "  ⏭️ mksquashfs missing (Debian: sudo apt install squashfs-tools) — skipped"; FAIL=1
  fi
done

echo "[rpm]"
if command -v rpmbuild >/dev/null 2>&1; then
  ./packaging/fedora/build-rpm.sh >>"$BLOG" 2>&1 \
    && echo "  ✅ rpm" || { echo "  ❌ rpm ($BLOG)"; FAIL=1; }
else
  echo "  ⏭️ rpmbuild missing (Fedora: sudo dnf install rpm-build golang / Debian: sudo apt install rpm) — skipped"; FAIL=1
fi

echo "[static] arch PKGBUILD + flatpak manifest: files ready (build on target distro)"
echo "=========================================="
echo "  حزم $VER:"
find dist/debian dist/appimage dist/rpm -type f 2>/dev/null | sort
echo "=========================================="
exit "$FAIL"
