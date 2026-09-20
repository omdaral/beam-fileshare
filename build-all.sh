#!/bin/bash
# Beam — بناء كل المنصات وتصنيفها حسب الإصدار:
#   dist/<os>/<arch>/<version>/FileShare[.exe]  +  dist/versions.json  +  dist/SHA256SUMS
# يعمل standalone أو من publish.sh (الخطوة 7). stdlib فقط، بدون تحميل مكتبات.
# المنصات: linux/windows/darwin × amd64/arm64 (الهوتسبوت لينكس/ويندوز؛ ماك/ARM وضع LAN).
set -euo pipefail
APP_DIR="$(cd "$(dirname "$0")" && pwd)" || { echo "تعذر فتح مجلد البرنامج"; exit 1; }
cd "$APP_DIR" || exit 1
# shellcheck source=packaging/common/lib.sh
source "$APP_DIR/packaging/common/lib.sh"
export PATH="$HOME/go/bin:/usr/local/go/bin:/opt/go/bin:$PATH"
export GOPROXY=off
export GOTOOLCHAIN=auto
mkdir -p "$APP_DIR/dist"
BUILDLOG="${BEAM_BUILDLOG:-/tmp/beam-build.log}"
: >>"$BUILDLOG" 2>/dev/null || BUILDLOG="$APP_DIR/build.log"

beam_require_cmd go "ثبّته مرة واحدة من https://go.dev/dl" || exit 1
VER="$(beam_load_ver VERSION)" || exit 1
beam_require_cmd sha256sum || exit 1

echo "فحص + اختبارات..."
go -C goserver vet ./... >>"$BUILDLOG" 2>&1 || { echo "❌ vet رسب — راجع $BUILDLOG"; exit 1; }
go -C goserver test -count=1 ./... >>"$BUILDLOG" 2>&1 || { echo "❌ الاختبارات رسبت — راجع $BUILDLOG"; exit 1; }
echo "✅ الفحص والاختبارات خضراء"

# تنظيف التخطيطات القديمة (ما قبل v1.6 — أسماء FileShare وملفات جانبية).
rm -rf dist/linux/FileShare dist/linux/config.json dist/linux/logs dist/linux/Shared \
       dist/windows/FileShare.exe
rm -f Beam.prev FileShare.prev .FileShare.new
rmdir dist/linux dist/windows 2>/dev/null || true

TARGETS="linux/amd64/Beam linux/arm64/Beam windows/amd64/Beam.exe windows/arm64/Beam.exe darwin/amd64/Beam darwin/arm64/Beam"
FAIL=0
for t in $TARGETS; do
  os="${t%%/*}"; rest="${t#*/}"; arch="${rest%%/*}"; bin="${rest#*/}"
  out="dist/$os/$arch/$VER/$bin"
  mkdir -p "$(dirname "$out")"
  printf '  - %s/%s ... ' "$os" "$arch"
  if CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go -C goserver build -trimpath -buildvcs=false -ldflags="-s -w -X fileshare.AppVersion=$VER" -o "../$out" ./cmd/beam >>"$BUILDLOG" 2>&1 && [ -s "$out" ]; then
    chmod +x "$out" 2>/dev/null || true
    if command -v file >/dev/null 2>&1; then
      ftype="$(file -b "$out" 2>/dev/null)"
      case "$ftype" in
        *ELF*|*PE32*|*Mach-O*) echo "OK ($(du -h "$out" | cut -f1))" ;;
        *) echo "⚠️ نوع غير متوقع: $ftype"; FAIL=1 ;;
      esac
    else
      echo "OK ($(du -h "$out" | cut -f1))"
    fi
  else
    echo "❌ فشل البناء — راجع logs/build.log"
    FAIL=1
  fi
done

# versions.json + SHA256SUMS من مسح الشجرة (التاريخ يتراكم عبر النشرات).
# SOURCE_DATE_EPOCH يجعل الطابع حتمياً عند توفره.
UPDATED="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
if [ -n "${SOURCE_DATE_EPOCH:-}" ]; then
  UPDATED="$(date -u -d "@$SOURCE_DATE_EPOCH" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || echo "$UPDATED")"
fi
{
  echo "{"
  echo '  "app": "Beam",'
  echo "  \"updated\": \"$UPDATED\","
  echo '  "artifacts": ['
  first=1
  find dist -type f \( -name Beam -o -name 'Beam.exe' \) | sort | while read -r f; do
    rel="${f#dist/}"
    [ "$(awk -F/ '{print NF-1}' <<<"$rel")" = 3 ] || continue
    case "$rel" in *[!a-zA-Z0-9._/-]*) continue ;; esac
    os="${rel%%/*}"; rest="${rel#*/}"; arch="${rest%%/*}"; rest2="${rest#*/}"; ver="${rest2%%/*}"
    bytes="$(stat -c%s "$f" 2>/dev/null)" || continue
    sha="$(sha256sum "$f" 2>/dev/null | cut -d' ' -f1)" || continue
    [ -z "$bytes" ] || [ -z "$sha" ] && continue
    [ "$first" = 1 ] && first=0 || echo ","
    printf '    {"os": "%s", "arch": "%s", "version": "%s", "file": "%s", "bytes": %s, "sha256": "%s"}' \
      "$os" "$arch" "$ver" "$rel" "$bytes" "$sha"
  done
  echo ""
  echo "  ]"
  echo "}"
} > dist/versions.json
(cd dist && find . -type f \( -name Beam -o -name 'Beam.exe' \) | sort | xargs sha256sum > SHA256SUMS)

echo "=========================================="
echo "  نسخ $VER مصنفة في dist/ (os/arch/version)"
cat dist/versions.json | grep -o '"os": "[a-z]*", "arch": "[a-z0-9]*", "version": "[^"]*"' || true
echo "=========================================="
[ "$FAIL" -ne 0 ] && { echo "❌ تعثرت منصة واحدة على الأقل."; exit 1; }
echo "✅ كل المنصات جاهزة."
