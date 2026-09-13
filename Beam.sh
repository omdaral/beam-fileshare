#!/bin/bash
# Beam — دبل كليك: يشغّل السيرفر لو واقف، ثم يفتح واجهة الويب في المتصفح.
# السيرفر نفسه يفتح المتصفح تلقائياً عند بدئه؛ السكربت يفتحه فقط لو كان
# السيرفر شغالاً أصلاً (ضغطة ثانية). بورت ثابت 2004 — بدون أي ملفات جانبية.
# الاستخدام: ./Beam.sh [--port 2004] [--no-browser]
set -u
APP_DIR="$(cd "$(dirname "$0")" && pwd)" || { echo "تعذر فتح مجلد البرنامج"; exit 1; }
cd "$APP_DIR" || exit 1

PORT=""
NO_BROWSER=0
while [ $# -gt 0 ]; do
  case "$1" in
    --port) PORT="${2:-}"; shift 2 ;;
    --no-browser) NO_BROWSER=1; shift ;;
    *) shift ;;
  esac
done
[ -z "$PORT" ] && PORT=2004

BIN=""
for c in "$APP_DIR/Beam" "$APP_DIR/FileShare" "$APP_DIR/Beam-linux-amd64" "$APP_DIR/FileShare-linux-amd64"; do
  if [ -x "$c" ]; then BIN="$c"; break; fi
done

health() { curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$PORT/health" 2>/dev/null; }

open_ui() {
  local URL="http://127.0.0.1:$PORT"
  if [ "$NO_BROWSER" = "0" ]; then
    if command -v xdg-open >/dev/null 2>&1; then
      xdg-open "$URL" >/dev/null 2>&1 &
    elif command -v gio >/dev/null 2>&1; then
      gio open "$URL" >/dev/null 2>&1 &
    elif command -v sensible-browser >/dev/null 2>&1; then
      sensible-browser "$URL" >/dev/null 2>&1 &
    else
      echo "السيرفر شغال. افتح المتصفح على: $URL"
    fi
  fi
}

WAS_DOWN=0
if ! health; then
  WAS_DOWN=1
fi

if [ "$WAS_DOWN" = "1" ]; then
  if [ -z "$BIN" ]; then
    echo "ملف البرنامج Beam غير موجود."
    echo "الحل: ابنِ الباينري مرة واحدة: ./build.sh (يحتاج Go على جهاز البناء فقط)"
    exit 1
  fi
  # السيرفر يفتح المتصفح بنفسه (إلا مع --no-browser)؛ السجل في /tmp لا بجانب البرنامج.
  if [ "$NO_BROWSER" = "0" ]; then
    nohup "$BIN" --port "$PORT" >>/tmp/beam-server.log 2>&1 &
  else
    nohup "$BIN" --port "$PORT" --no-browser >>/tmp/beam-server.log 2>&1 &
  fi
  for _ in $(seq 1 20); do
    sleep 0.5
    health && break
  done
fi

if ! health; then
  echo "تعذر تشغيل السيرفر على البورت $PORT."
  echo "الحل: راجع /tmp/beam-server.log ثم أعد المحاولة."
  exit 1
fi

# رابط دخول الأجهزة (LAN/Hotspot) من السيرفر نفسه — للمالك لمشاركته.
SHARE_URL="$(curl -s --max-time 5 "http://127.0.0.1:$PORT/api/status" 2>/dev/null | grep -o '"url":"[^"]*"' | head -1 | cut -d'"' -f4)"
[ -z "$SHARE_URL" ] && SHARE_URL="http://127.0.0.1:$PORT"
echo "رابط دخول الأجهزة: $SHARE_URL"
if command -v notify-send >/dev/null 2>&1; then
  notify-send "Beam" "السيرفر شغال — رابط دخول الأجهزة:\n$SHARE_URL" >/dev/null 2>&1 &
fi

# السيرفر الجديد فتح المتصفح بنفسه؛ الفتح هنا للضغطة الثانية فقط.
if [ "$WAS_DOWN" = "0" ]; then
  open_ui
fi
