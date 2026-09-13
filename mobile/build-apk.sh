#!/bin/bash
# Beam — بناء APK الأندرويد (يعمل على جهاز فيه JDK 21+ و Android SDK + NDK).
# السيرفر يعمل داخل التطبيق (Go عبر gomobile) + هوتسبوت محلي + خدمة أمامية.
# الخطوات: Go engine (.aar) ← الواجهة ← npm ← مشروع أندرويد ← باتش أصلي ← APK.
# الاستخدام (من mobile/): ./build-apk.sh
set -u
cd "$(dirname "$0")" || exit 1
export PATH="$HOME/go/bin:$HOME/gopath/bin:/usr/local/go/bin:$PATH"
export GOPATH="${GOPATH:-$HOME/gopath}"
export GOTOOLCHAIN=local
export GOPROXY=off

command -v node >/dev/null 2>&1 || { echo "❌ node غير مثبت (https://nodejs.org)"; exit 1; }
# Capacitor 8 يشترط Node 22+: لو الافتراضي أقدم، جرّب نسخ nvm تلقائياً
# (النشر من أيقونة سطح المكتب لا يحمّل .bashrc فلا يرى nvm).
NVER="$(node -v 2>/dev/null | grep -oE '[0-9]+' | head -1)"
if [ "${NVER:-0}" -lt 22 ] 2>/dev/null; then
  for nd in "$HOME"/.nvm/versions/node/v22* "$HOME"/.nvm/versions/node/*; do
    [ -x "$nd/bin/node" ] || continue
    _nv="$("$nd/bin/node" -v 2>/dev/null | grep -oE '[0-9]+' | head -1)"
    if [ "${_nv:-0}" -ge 22 ] 2>/dev/null; then export PATH="$nd/bin:$PATH"; NVER="$_nv"; break; fi
  done
fi
[ "${NVER:-0}" -ge 22 ] 2>/dev/null || { echo "❌ Node $NVER أقدم من المطلوب (Capacitor 8 يشترط 22+ — المستخدم الآن: $(command -v node)، ثبّت Node 22 من https://nodejs.org أو عبر nvm)"; exit 1; }
echo "  Node $(node -v) ($(command -v node)) ✅"
if [ -d "$HOME/jdks" ]; then
  for j in "$HOME"/jdks/jdk-21* "$HOME"/jdks/*21*; do
    if [ -x "$j/bin/java" ]; then export JAVA_HOME="$j"; break; fi
  done
fi
if [ -n "${JAVA_HOME:-}" ]; then export PATH="$JAVA_HOME/bin:$PATH"; fi
command -v java >/dev/null 2>&1 || { echo "❌ JDK غير مثبت (يحتاج JDK 21+ — راجع mobile/SETUP-ar.md: نزّل Temurin 21 في ~/jdks أو sudo apt install openjdk-21-jdk)"; exit 1; }
JVER="$(java -version 2>&1 | head -1 | grep -oE '[0-9]+' | head -1)"
[ "${JVER:-0}" -ge 21 ] || { echo "❌ JDK $JVER أقدم من المطلوب (Capacitor 8 يشترط 21+ — المستخدم الآن: $(command -v java)، راجع mobile/SETUP-ar.md)"; exit 1; }
echo "  JDK $JVER ($(command -v java)) ✅"
if [ -z "${ANDROID_HOME:-}" ]; then
  [ -d "$HOME/Android/Sdk" ] && export ANDROID_HOME="$HOME/Android/Sdk"
fi
[ -n "${ANDROID_HOME:-}" ] || { echo "❌ عرّف ANDROID_HOME على مجلد Android SDK أولاً (المتوقع ~/Android/Sdk — راجع mobile/SETUP-ar.md)"; exit 1; }
[ -d "${ANDROID_HOME:-/nonexistent}" ] || { echo "❌ ANDROID_HOME=$ANDROID_HOME غير موجود — راجع mobile/SETUP-ar.md"; exit 1; }
echo "  ANDROID_HOME=$ANDROID_HOME ✅"

VER="$(tr -d ' \t\r\n' < ../VERSION 2>/dev/null)"
[ -z "$VER" ] && { echo "❌ ملف VERSION مفقود."; exit 1; }

export TMPDIR="${TMPDIR:-/tmp}"
# تثبيت gomobile تلقائياً لو غائب (يحتاج إنترنت أول مرة فقط).
# ملاحظة: GOPROXY=off العام يُرفع مؤقتاً هنا لأن التثبيت يحمّل من الوكيل.
if ! command -v gomobile >/dev/null 2>&1; then
  echo "تثبيت gomobile لمرة واحدة…"
  command -v go >/dev/null 2>&1 || { echo "❌ ثبّت Go 1.22+ أولاً (https://go.dev/dl)"; exit 1; }
  # Pinned for reproducible builds (update with VERSION bumps).
  (cd ../goserver && GOPROXY="https://proxy.golang.org,direct" GOTOOLCHAIN=auto \
    go install golang.org/x/mobile/cmd/gomobile@v0.0.0-20231127183840-76ac6878050a) || { echo "❌ فشل تثبيت gomobile (تحقق من الإنترنت ثم أعد المحاولة)"; exit 1; }
  GOBIN_CANDIDATES="$HOME/gopath/bin:$HOME/go/bin"
  for d in ${GOBIN_CANDIDATES//:/ }; do
    [ -x "$d/gomobile" ] && export PATH="$d:$PATH" && break
  done
fi
command -v gomobile >/dev/null 2>&1 || { echo "❌ تعذر تثبيت gomobile (ثبّته يدوياً: راجع mobile/SETUP-ar.md)"; exit 1; }
NDK_DIR="$(ls -d "$ANDROID_HOME"/ndk/* 2>/dev/null | sort -V | tail -1)"
[ -n "$NDK_DIR" ] || { echo "❌ ثبّت NDK أولاً: sdkmanager \"ndk;26.3.11579264\" (راجع mobile/SETUP-ar.md)"; exit 1; }
gomobile init -ndk "$NDK_DIR" >/dev/null 2>&1 || true
echo "—— 1/5 محرك Go داخل التطبيق (beam.aar)"
export TMPDIR="${TMPDIR:-/tmp}"
(cd ../goserver/beamapp && gomobile bind -androidapi 21 \
  -target android/arm64,android/arm -o ../../mobile/beam.aar .) \
  || { echo "❌ فشل gomobile bind (يحتاج NDK + شبكة أول مرة)"; exit 1; }
[ -s beam.aar ] || { echo "❌ لم يُنتج beam.aar"; exit 1; }
echo "  beam.aar جاهز ($(du -h beam.aar | cut -f1))"

echo "—— 2/5 تجميع الواجهة"
./sync-web.sh || exit 1

echo "—— 3/5 تثبيت اعتماديات Capacitor"
if [ -f package-lock.json ]; then
  npm ci || { echo "❌ فشل npm ci (تحقق من الإنترنت)"; exit 1; }
else
  npm install || { echo "❌ فشل npm install (تحقق من الإنترنت)"; exit 1; }
fi

echo "—— 4/5 مشروع أندرويد + الطبقة الأصلية (سيرفر/هوتسبوت/أيقونات/أذونات)"
[ -d android ] || npx cap add android || { echo "❌ فشل cap add android"; exit 1; }
npx cap sync android || exit 1
python3 scripts/apply-android-src.py || exit 1

echo "—— 5/5 بناء APK"
(cd android && ./gradlew assembleDebug --offline 2>/dev/null || ./gradlew assembleDebug) || {
  echo "❌ فشل البناء — راجع رسالة gradle أعلاه (أول بناء يحمّل مكونات من الإنترنت)."
  exit 1
}
APK="$(find android/app/build/outputs/apk/debug -name '*.apk' | head -1)"
[ -n "$APK" ] || { echo "❌ لم يُنتج APK"; exit 1; }
mkdir -p ../dist/mobile
OUT="../dist/mobile/Beam-$VER-android.apk"
cp "$APK" "$OUT"
echo "=========================================="
echo "  ✅ $OUT ($(du -h "$OUT" | cut -f1))"
echo "  التثبيت على الموبايل: انسخ الملف وافتحه (فعّل مصادر غير معروفة)"
echo "=========================================="
