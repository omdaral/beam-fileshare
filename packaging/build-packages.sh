#!/bin/bash
# Beam — بناء كل حزم التوزيع من شجرة dist/ الجاهزة (./build-all.sh أولاً).
#   Debian/Ubuntu: dist/debian/*.deb            (dpkg-deb — يعمل هنا)
#   AppImage:      dist/appimage/*.AppImage     (يحتاج mksquashfs)
#   Fedora .rpm: dist/rpm/*.rpm               (needs rpmbuild)
#   Arch: PKGBUILD جاهز (البناء على Arch) + Flatpak: manifest جاهز
# الاستخدام: packaging/build-packages.sh [amd64|arm64|all] [--skip-deb] [--skip-appimage] [--skip-rpm]
# غير قاتل: أي حزمة تتعثر تُسجَّل ويُكمل الباقي (الخروج 1 في النهاية للتنبيه فقط).
# كل حزمة تطبع تقدمها حياً على الشاشة (tee للسجل) حتى لا تبدو الخطوة معلقة
# أثناء mksquashfs/rpmbuild الطويلة.
set -u
APP_DIR="$(cd "$(dirname "$0")/.." && pwd)" || exit 1
cd "$APP_DIR" || exit 1
ARCHSEL="all"
SKIP_DEB=0; SKIP_APPIMAGE=0; SKIP_RPM=0
for a in "$@"; do
  case "$a" in
    amd64|arm64|all) ARCHSEL="$a" ;;
    --skip-deb) SKIP_DEB=1 ;;
    --skip-appimage) SKIP_APPIMAGE=1 ;;
    --skip-rpm) SKIP_RPM=1 ;;
    *) echo "arch: amd64|arm64|all [--skip-deb] [--skip-appimage] [--skip-rpm]"; exit 1 ;;
  esac
done
case "$ARCHSEL" in amd64|arm64) GOARCHS="$ARCHSEL" ;; all) GOARCHS="amd64 arm64" ;; esac
VER="$(tr -d ' \t\r\n' < VERSION 2>/dev/null)"
[ -z "$VER" ] && { echo "VERSION missing"; exit 1; }
FAIL=0

BLOG="/tmp/beam-build.log"
# run <label> <cmd...>: stream output live to screen AND log with timestamps,
# so long steps (mksquashfs, rpmbuild vet+test+build) never look hung.
run() {
  label="$1"; shift
  echo "  [$label] بدأ $(date '+%H:%M:%S') — التفاصيل حية أدناه:"
  start=$(date +%s)
  "$@" 2>&1 | tee -a "$BLOG"
  rc=${PIPESTATUS[0]:-$?}
  echo "  [$label] انتهى في $(( $(date +%s) - start )) ثانية (خروج $rc)"
  return "$rc"
}

for GOARCH in $GOARCHS; do
  case "$GOARCH" in
    amd64) DEBARCH=amd64; AIMGARCH=x86_64 ;;
    arm64) DEBARCH=arm64; AIMGARCH=aarch64 ;;
  esac
  echo "—— $GOARCH ——"
  if [ ! -x "dist/linux/$GOARCH/$VER/Beam" ]; then
    echo "⚠️ binary missing: dist/linux/$GOARCH/$VER/Beam (run ./build-all.sh)"; FAIL=1; continue
  fi
  if [ "$SKIP_DEB" = "1" ]; then
    echo "  ⏭️ deb $DEBARCH — تُخطي (--skip-deb)"
  else
    echo "[deb]"
    run "deb-$DEBARCH" ./packaging/debian/build-deb.sh "$DEBARCH" \
      && echo "  ✅ deb $DEBARCH" || { echo "  ❌ deb $DEBARCH ($BLOG)"; FAIL=1; }
  fi
  echo "[appimage]"
  if [ "$SKIP_APPIMAGE" = "1" ]; then
    echo "  ⏭️ appimage $AIMGARCH — تُخطي (--skip-appimage)"
  elif command -v mksquashfs >/dev/null 2>&1; then
    run "appimage-$AIMGARCH" ./packaging/appimage/build-appimage.sh "$AIMGARCH" \
      && echo "  ✅ appimage $AIMGARCH" || { echo "  ❌ appimage $AIMGARCH ($BLOG)"; FAIL=1; }
  else
    echo "  ⏭️ mksquashfs missing (Debian: sudo apt install squashfs-tools) — skipped"; FAIL=1
  fi
done

echo "[rpm]"
if [ "$SKIP_RPM" = "1" ]; then
  echo "  ⏭️ rpm — تُخطي (--skip-rpm)"
elif command -v rpmbuild >/dev/null 2>&1; then
  echo "  (rpmbuild يعيد vet+test+build كاملاً من السورس — يستغرق دقيقة أو أكثر، والتقدم ظاهر أدناه)"
  run "rpm" ./packaging/fedora/build-rpm.sh \
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
