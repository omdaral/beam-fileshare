#!/usr/bin/env bash
# Company Share — بناء كل المنصات (ينفذ build-all.sh: مصفوفة os/arch + تصنيف الإصدارات في dist/).
# يحتاج Go 1.21+ مرة واحدة على جهاز البناء فقط (stdlib، بدون تحميل مكتبات).
set -u
cd "$(dirname "$0")" || exit 1
exec ./build-all.sh "$@"
