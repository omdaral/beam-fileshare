#!/bin/bash
# Beam — توحيد رقم الإصدار في كل الملفات (VERSION هو المصدر الوحيد).
# Usage: packaging/bump-version.sh 1.7.0
set -euo pipefail
APP_DIR="$(cd "$(dirname "$0")/.." && pwd)" || exit 1
cd "$APP_DIR" || exit 1
# shellcheck source=common/lib.sh
source "$APP_DIR/packaging/common/lib.sh"

if [ $# -ne 1 ]; then
  echo "Usage: packaging/bump-version.sh X.Y.Z"
  exit 2
fi
VER="$1"
if ! [[ "$VER" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "❌ صيغة الإصدار غير صالحة: $VER (مثال: 1.7.0)"
  exit 2
fi
echo "$VER" > VERSION
echo "VERSION=$VER"

# Go fallback default (ldflags is the real source; this keeps `go run` correct)
sed -i 's/AppVersion = "[0-9.]*"/AppVersion = "'"$VER"'"/' goserver/app.go
# Arch + flatpak comments
sed -i 's/^pkgver=.*/pkgver='"$VER"'/' packaging/arch/PKGBUILD
sed -i 's/tag: v[0-9.]*/tag: v'"$VER"'/' packaging/flatpak/com.beam.beam.yaml
sed -i 's/-ldflags="-s -w -X fileshare.AppVersion=[^"]*"/-ldflags="-s -w -X fileshare.AppVersion='"$VER"'"/' packaging/flatpak/com.beam.beam.yaml || true
# mobile npm versions (keep in sync; lock file version updated via npm version below)
if command -v python3 >/dev/null 2>&1; then
  python3 - "$VER" <<'PY'
import json, sys
ver = sys.argv[1]
for p in ["mobile/package.json"]:
    try:
        with open(p, encoding="utf-8") as f: d = json.load(f)
        d["version"] = ver
        with open(p, "w", encoding="utf-8") as f: json.dump(d, f, indent=2, ensure_ascii=False); f.write("\n")
        print(f"{p}={ver}")
    except FileNotFoundError:
        pass
PY
fi
echo "✅ تم التوحيد على $VER — راجع git diff ثم ابنِ ./build-all.sh"
echo "   (mobile/package-lock.json يُحدَّث تلقائياً مع npm ci/install القادم)"
