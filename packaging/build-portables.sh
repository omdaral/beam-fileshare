#!/bin/bash
# Beam — تجميع الحزم المحمولة الجاهزة للمشاركة من شجرة dist/ (./build-all.sh أولاً).
#   dist/portables/<ver>/Beam-<ver>-{windows,macos,linux}-<arch>.{zip,tar.gz}
# كل حزمة: الباينري + المشغّل + الأيقونة (.ico/.icns/.png) + VERSION + تعليمات عربي.
# الصلاحيات التنفيذية محفوظة ومتحقق منها داخل كل أرشيف. stdlib فقط (python3).
# الاستخدام: packaging/build-portables.sh
set -u
APP_DIR="$(cd "$(dirname "$0")/.." && pwd)" || exit 1
cd "$APP_DIR" || exit 1
VER="$(tr -d ' \t\r\n' < VERSION 2>/dev/null)"
[ -z "$VER" ] && { echo "VERSION missing"; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "python3 missing (needed for zip assembly)"; exit 1; }

OUT="dist/portables/$VER"
mkdir -p "$OUT"
if python3 packaging/mkportable.py "$VER" "$OUT"; then
  echo "=========================================="
  echo "  portable bundles $VER:"
  ls -la "$OUT"
  echo "=========================================="
else
  echo "❌ portable assembly failed (run ./build-all.sh first)"
  exit 1
fi
