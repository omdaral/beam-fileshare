#!/bin/bash
# Beam — نشر بضغطة واحدة (10 خطوات بتقرير جدولي):
# فحص + قدرات + أيقونات + باينريات + حزم محمولة/نظام + فهرس + لانشرات + APK + تركيب حي + تحقق.
# عند أي فشل جوهري يتوقف فوراً برسالة عربية، ويرجع تلقائياً للنسخة السابقة لو التركيب تم.
# الحزم المتعثرة لنقص أدوات تُتخطى بتقرير (لا تفشل النشر).
# الاستخدام: ./publish.sh [--port 2004] [--install-menu]
# ملاحظة: تثبيت لانشر القائمة opt-in فقط (--install-menu) — لا يظهر شيء في
# قائمة ستارت ما لم تطلبه صراحة.
# لا ملفات جانبية: البورت ثابت 2004، والسجل في /tmp.
set -u
export PATH="$HOME/go/bin:$HOME/gopath/bin:/usr/local/go/bin:/opt/go/bin:$PATH"
# بيئة الأندرويد الافتراضية (نفس اكتشاف mobile/build-apk.sh) —
# النشر من أيقونة سطح المكتب لا يحمّل .bashrc فتغيب المتغيرات.
if [ -z "${ANDROID_HOME:-}" ] && [ -d "$HOME/Android/Sdk" ]; then
  export ANDROID_HOME="$HOME/Android/Sdk"
fi

APP_DIR="$(cd "$(dirname "$0")" && pwd)" || { echo "تعذر فتح مجلد البرنامج"; exit 1; }
cd "$APP_DIR" || exit 1

PORT=2004
INSTALL_MENU=0
for a in "$@"; do
  case "$a" in
    --port|--install-menu|--no-desktop) ;; # معروف — يُعالج أدناه
    *) echo "استخدام: ./publish.sh [--port 2004] [--install-menu]"; echo "علم غير معروف: $a"; exit 2 ;;
  esac
done
while [ $# -gt 0 ]; do
  case "$1" in
    --port) PORT="${2:-}"; shift 2 ;;
    --install-menu) INSTALL_MENU=1; shift ;;
    --no-desktop) shift ;; # اسم قديم: التجاهل هو الافتراضي الآن
    *) shift ;;
  esac
done
BIN="$APP_DIR/Beam"
NEWBIN="$APP_DIR/.Beam.new"
PREV="$APP_DIR/Beam.prev"
BUILDLOG="/tmp/beam-build.log"
: > "$BUILDLOG"
VER="$(tr -d ' \t\r\n' < VERSION 2>/dev/null)"
[ -z "$VER" ] && { echo "❌ ملف VERSION مفقود."; exit 1; }

# Per-system delivery status for the final table (1 = ready).
ST_LOCAL=0; ST_PORTABLE=0; ST_DEB=0; ST_APPIMAGE=0; ST_RPM=0; ST_APK=0
SKIPPED=""
STEP_T=""

# Lock: prevent two simultaneous publishes (double-click accidents).
LOCKDIR="/tmp/beam-publish.lock"
if [ -d "$LOCKDIR" ] && [ -f "$LOCKDIR/pid" ]; then
  OLDPID="$(cat "$LOCKDIR/pid" 2>/dev/null)"
  if [ -n "$OLDPID" ] && kill -0 "$OLDPID" 2>/dev/null; then
    echo "❌ نشر آخر يعمل بالفعل (PID $OLDPID) — انتظر انتهاءه ثم أعد المحاولة."
    exit 2
  fi
  rm -rf "$LOCKDIR" 2>/dev/null # stale lock from a killed run
fi
mkdir "$LOCKDIR" 2>/dev/null || { echo "❌ تعذر بدء النشر (قفل عالق)."; exit 2; }
echo $$ > "$LOCKDIR/pid"
trap 'rm -rf "$LOCKDIR" 2>/dev/null' EXIT

step()  {
  now=$(date +%s)
  if [ -n "$STEP_T" ]; then
    echo "  (الخطوة السابقة استغرقت $((now-STEP_T)) ثانية)"
  fi
  STEP_T=$now
  echo "—— $1"
}
ok()    { echo "✅ $1"; }
skip()  { echo "⏭️ $1"; SKIPPED="${SKIPPED}  • $1
"; }
note()  { echo "$1"; SKIPPED="${SKIPPED}  • $1
"; }
# pause: never close the window before the user reads (TTY only — CI-safe).
pause() {
  if [ -t 0 ]; then
    echo "—— اضغط Enter للإغلاق ——"
    read -r _ </dev/tty 2>/dev/null || read -r _
  fi
}
fail()  {
  echo "❌ $1"; echo "الحل: $2"
  if [ -s "$BUILDLOG" ]; then
    echo "--- آخر سطر من سجل البناء ($BUILDLOG) ---"
    tail -15 "$BUILDLOG"
    echo "--- نهاية المقتطف ---"
  fi
  notify "نشر Beam: فشل" "$1" 2>/dev/null
  pause
  exit 1
}
notify(){
  command -v notify-send >/dev/null 2>&1 && notify-send "$1" "$2" >/dev/null 2>&1 &
  return 0
}
health() { curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$PORT/health" 2>/dev/null; }

wait_down() {
  for _ in $(seq 1 30); do
    health || return 0
    sleep 0.5
  done
  return 1
}
wait_up() {
  for _ in $(seq 1 30); do
    health && return 0
    sleep 0.5
  done
  return 1
}
start_server() {
  nohup "$BIN" --port "$PORT" --no-browser >>/tmp/beam-server.log 2>&1 &
}

echo "=========================================="
echo "  نشر Beam v$VER — $(date '+%Y-%m-%d %H:%M:%S')"
echo "=========================================="

step "1/10 تقرير القدرات (ماذا سيُبنى هنا)"
have() { command -v "$1" >/dev/null 2>&1 && echo "موجود" || echo "ناقص"; }
echo "  Go: $(have go) | python3: $(have python3) | dpkg-deb: $(have dpkg-deb)"
echo "  mksquashfs (AppImage): $(have mksquashfs) | rpmbuild (Fedora rpm): $(have rpmbuild)"
echo "  desktop-file-validate: $(have desktop-file-validate)"
_eff_java="$(command -v java 2>/dev/null)"
if [ -d "$HOME/jdks" ]; then
  for _j in "$HOME"/jdks/jdk-21* "$HOME"/jdks/*21*; do
    if [ -x "$_j/bin/java" ]; then _eff_java="$_j/bin/java"; break; fi
  done
fi
_jv="؟"; [ -n "$_eff_java" ] && _jv="$("$_eff_java" -version 2>&1 | head -1 | grep -oE '[0-9]+' | head -1)"
[ -n "$_jv" ] || _jv="؟"
_ndk="ناقص"; [ -n "$(ls -d "${ANDROID_HOME:-/nonexistent}"/ndk/* 2>/dev/null)" ] && _ndk="موجود"
echo "  Android (APK): java $_jv ($_eff_java) + node ($(have node)) + gomobile ($(have gomobile)) + ANDROID_HOME=${ANDROID_HOME:-unset} + NDK ($_ndk)"
if [ "$_jv" != "؟" ] && [ "$_jv" -lt 21 ] 2>/dev/null; then
  skip "JDK $_jv أقدم من 21 — الـ APK سيُتخطى (ثبّت JDK 21: راجع mobile/SETUP-ar.md)."
fi
echo "  → الباينريات الست + ZIPs المحمولة + deb + rpm تُبنى هنا."
if [ "$(have mksquashfs)" = "ناقص" ]; then
  skip "AppImage سيُتخطى (للبناء: sudo apt install squashfs-tools ثم أعد النشر)."
fi
if [ "$(have rpmbuild)" = "ناقص" ]; then
  skip "rpm سيُتخطى (للبناء: sudo apt install rpm ثم أعد النشر)."
fi
echo "  → Arch/Flatpak: ملفات + تعليمات فقط (تحتاج Arch/flatpak-builder)."

step "2/10 فحص الكود والاختبارات"
command -v go >/dev/null 2>&1 || fail "Go غير مثبت على هذا الجهاز." "ثبّته مرة واحدة من https://go.dev/dl ثم أعد النشر."
[ -d "$APP_DIR/goserver" ] || fail "مجلد السورس goserver غير موجود." "نفّذ النشر من مجلد المشروع الكامل."
export GOPROXY=off
go -C goserver vet ./... >>"$BUILDLOG" 2>&1 \
  || fail "فحص الكود (vet) رسب." "راجع $BUILDLOG وأصلحها ثم أعد النشر."
go -C goserver test -count=1 ./... >>"$BUILDLOG" 2>&1 \
  || fail "الاختبارات رسبت." "شغّل go -C goserver test ./... وراجع الفشل ثم أعد النشر."
ok "الفحص والاختبارات خضراء"

step "3/10 تجديد الأيقونات من الهوية (icon.svg ← كل المقاسات)"
if python3 packaging/render-icons.py >>"$BUILDLOG" 2>&1; then
  ok "الأيقونات متجددة (root + ico/icns + حزم)"
else
  fail "فشل توليد الأيقونات." "راجع $BUILDLOG (هل Pillow مثبت؟)."
fi

step "4/10 بناء الباينريات الست (linux/windows/darwin × amd64/arm64)"
if "$APP_DIR/build-all.sh" >>"$BUILDLOG" 2>&1; then
  ok "مصفوفة المنصات مصنفة في dist/ (os/arch/version + versions.json)"
else
  fail "فشل بناء المنصات." "راجع $BUILDLOG ثم نفّذ ./build-all.sh منفرداً."
fi

step "5/10 الحزم المحمولة الجاهزة للمشاركة (ZIPs)"
if ./packaging/build-portables.sh >>"$BUILDLOG" 2>&1; then
  ST_PORTABLE=1
  ok "الحزم المحمولة في dist/portables/$VER/ (6 حزم: ويندوز/ماك/لينكس × معماريتين)"
else
  note "⚠️ تعثرت الحزم المحمولة — راجع $BUILDLOG (النشر المحلي مستمر)."
fi

step "6/10 حزم النظام (deb/AppImage/rpm — تُبنى هنا، الباقي ملفات)"
if ./packaging/build-packages.sh all >>"$BUILDLOG" 2>&1; then
  ST_DEB=1; ST_APPIMAGE=1; ST_RPM=1
  ok "الحزم جاهزة في dist/debian + dist/appimage + dist/rpm"
else
  [ -n "$(ls dist/debian/*.deb 2>/dev/null)" ] && ST_DEB=1
  [ -n "$(ls dist/appimage/*.AppImage 2>/dev/null)" ] && ST_APPIMAGE=1
  [ -n "$(ls dist/rpm/*.rpm 2>/dev/null)" ] && ST_RPM=1
  note "⚠️ بعض الحزم تعثرت/تُخطيت — التفاصيل في $BUILDLOG (النشر المحلي مستمر)."
fi

step "7/10 الفهرس الموحد (MANIFEST + SHA256SUMS لكل المخرجات)"
if python3 packaging/manifest.py "$VER" >>"$BUILDLOG" 2>&1; then
  ok "dist/MANIFEST.json + dist/SHA256SUMS يغطيان كل الملفات"
else
  note "⚠️ تعثر الفهرس — راجع $BUILDLOG."
fi

step "8/10 تحديث اللانشرات بالأيقونة الجديدة"
if [ "$INSTALL_MENU" = "1" ]; then
  ./install.sh --install-menu >>"$BUILDLOG" 2>&1 \
    && ok "اللانشرات مولّدة ومثبتة (سطح المكتب + القائمة)" \
    || note "⚠️ تعثر تثبيت اللانشرات — راجع $BUILDLOG."
else
  ./install.sh >>"$BUILDLOG" 2>&1 \
    && ok "اللانشرات مولّدة داخل مجلد البرنامج فقط (بلا تثبيت في القائمة)" \
    || note "⚠️ تعثر توليد اللانشرات — راجع $BUILDLOG."
fi

step "9/10 تطبيق الأندرويد (APK — يُبنى لو البيئة جاهزة، وإلا تخطي مع السبب)"
if [ -d "$APP_DIR/mobile" ]; then
  if "$APP_DIR/mobile/build-apk.sh" >>"$BUILDLOG" 2>&1; then
    ST_APK=1
    ok "APK جاهز في dist/mobile/"
    # الفهرس وُلّد في خطوة 7 قبل الـ APK — جدّده ليدخل الجديد فيه.
    if python3 packaging/manifest.py "$VER" >>"$BUILDLOG" 2>&1; then
      ok "الفهرس جُدد بعد APK (MANIFEST + SHA256SUMS يشملانه)"
    else
      note "⚠️ تعثر تجديد الفهرس بعد APK — راجع $BUILDLOG."
    fi
  else
    echo "--- لماذا تعثر الـ APK؟ (تشخيص البيئة) ---"
    echo "  java: $(command -v java 2>/dev/null || echo 'غائب') ($(java -version 2>&1 | head -1 || echo 'لا نسخة'))"
    echo "  ANDROID_HOME=${ANDROID_HOME:-غير معرّف} | NDK: $(ls -d "${ANDROID_HOME:-/nonexistent}"/ndk/* 2>/dev/null | tail -1 || echo 'غائب')"
    echo "  node: $(command -v node 2>/dev/null || echo 'غائب') | gomobile: $(command -v gomobile 2>/dev/null || echo 'غائب')"
    if [ -s "$BUILDLOG" ]; then
      echo "--- آخر سطر من سجل البناء ($BUILDLOG) ---"
      tail -15 "$BUILDLOG"
      echo "--- نهاية المقتطف ---"
    fi
    note "⚠️ تعثر بناء APK (النشر مستمر — يحتاج: JDK 21 + Android SDK/NDK + Node — راجع mobile/SETUP-ar.md)."
    echo "   لإعادة بناء الـ APK وحده لاحقاً: ./mobile/build-apk.sh"
  fi
else
  skip "مجلد mobile غير موجود."
fi

step "10/10 التركيب المحلي + التحقق الحي"
if health; then
  curl -s --max-time 5 -X POST "http://127.0.0.1:$PORT/api/server/stop" \
    -H 'Content-Type: application/json' -d '{}' >/dev/null 2>&1
  wait_down || fail "السيرفر لم يتوقف." "أوقفه يدوياً (زر ⏻ الدائري في أعلى الصفحة) ثم أعد النشر."
  ok "توقف السيرفر القديم"
else
  ok "لا سيرفر يعمل على البورت $PORT — نكمل"
fi
rm -f "$NEWBIN"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go -C goserver build -trimpath -ldflags="-s -w -X fileshare.AppVersion=$VER" -o ../.Beam.new ./cmd/beam \
  || fail "فشل بناء الباينري المحلي." "راجع رسالة go build أعلاه."
[ -x "$BIN" ] && cp -f "$BIN" "$PREV" && echo "  (احتفاظ بالنسخة السابقة للرجوع)"
mv -f "$NEWBIN" "$BIN" && chmod +x "$BIN"
start_server
if ! wait_up; then
  echo "الجديد لم يعمل — رجوع تلقائي للنسخة السابقة..."
  if [ -x "$PREV" ]; then
    cp -f "$PREV" "$BIN" && start_server && wait_up \
      && fail "الجديد فشل في التشغيل ورجعنا للقديم." "راجع /tmp/beam-server.log ثم أعد النشر بعد الإصلاح."
  fi
  fail "الجديد فشل ولا توجد نسخة سابقة." "راجع /tmp/beam-server.log."
fi
ST_LOCAL=1
B="http://127.0.0.1:$PORT"
curl -s --max-time 8 "$B/admin" -o /tmp/beam_pub.html || fail "صفحة المالك لا ترد." "راجع /tmp/beam-server.log."
grep -q "Beam" /tmp/beam_pub.html || fail "الصفحة لا تحمل الهوية." "تأكد أن البناء تم من السورس الحالي."
grep -q "srvStopTop" /tmp/beam_pub.html || fail "زر الإيقاف الجديد غائب من الصفحة." "أعد البناء من سورس نظيف."
grep -q "unlockCard\|QRmini" /tmp/beam_pub.html \
  && fail "بقايا النظام القديم ما زالت في الصفحة." "أعد البناء من سورس نظيف."
curl -s --max-time 8 "$B/vendor/qrcode.js" -o /dev/null || fail "مكتبة QR لا تُقدَّم." "راجع /tmp/beam-server.log."
curl -s --max-time 8 "$B/app/app.js" -o /dev/null || fail "ملف الواجهة /app/app.js لا يُقدَّم." "راجع /tmp/beam-server.log."
curl -s --max-time 8 "$B/app/style.css" -o /dev/null || fail "ملف الواجهة /app/style.css لا يُقدَّم." "راجع /tmp/beam-server.log."
LIVEVER="$(curl -s --max-time 5 "$B/api/status" 2>/dev/null | grep -o '"version":"[^"]*"' | head -1 | cut -d'"' -f4)"
[ "$LIVEVER" = "$VER" ] || fail "السيرفر الحي نسخة $LIVEVER لا $VER." "راجع /tmp/beam-server.log."
rm -f /tmp/beam_pub.html
ok "السيرفر الجديد حي بالنسخة $VER (هوية + زر إيقاف + QR)"
URL="$(curl -s --max-time 5 "$B/api/status" 2>/dev/null | grep -o '"url":"[^"]*"' | head -1 | cut -d'"' -f4)"
[ -z "$URL" ] && URL="$B"

echo "=========================================="
echo "  تم النشر بنجاح ✅ — رابط الدخول: $URL"
echo "------------------------------------------"
if [ -n "$SKIPPED" ]; then
  echo "  ملاحظات التخطي/التعثر (غير قاتلة):"
  printf "%s" "$SKIPPED"
  echo "------------------------------------------"
fi
printf "  %-28s %s\n" "السيرفر المحلي v$VER"      "$([ $ST_LOCAL = 1 ] && echo ✅ || echo ❌)"
printf "  %-28s %s\n" "ZIPs محمولة (6 أنظمة)"      "$([ $ST_PORTABLE = 1 ] && echo ✅ || echo ⚠️)"
printf "  %-28s %s\n" "deb (amd64/arm64)"          "$([ $ST_DEB = 1 ] && echo ✅ || echo ⏭️)"
printf "  %-28s %s\n" "AppImage"                   "$([ $ST_APPIMAGE = 1 ] && echo ✅ || echo ⏭️ ناقص mksquashfs)"
printf "  %-28s %s\n" "rpm (fedora)"               "$([ $ST_RPM = 1 ] && echo ✅ || echo ⏭️ ناقص rpmbuild)"
printf "  %-28s %s\n" "APK (android)"              "$([ $ST_APK = 1 ] && echo ✅ || echo ⏭️ بيئة أندرويد ناقصة)"
printf "  %-28s %s\n" "arch/flatpak"               "⏭️ ملفات + تعليمات"
echo "  الملفات: dist/portables/$VER/ + dist/debian/ + dist/rpm/ + dist/mobile/ + dist/MANIFEST.json"
echo "=========================================="
notify "نشر Beam: تم ✅" "السيرفر الجديد يعمل — $URL"
pause
