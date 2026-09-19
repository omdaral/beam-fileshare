#!/bin/bash
# Beam — تثبيت اللانشرات بالأيقونة بعد نسخ المجلد لأي مكان.
# يولّد المسارات المطلقة الصحيحة داخل Beam.desktop حسب مكانه الحالي
# (مواصفة .desktop تشترط مسارات مطلقة — لهذا النقل اليدوي كان يكسر الأيقونة)،
# ثم يثبتها على سطح المكتب وفي قائمة التطبيقات فقط عند طلب صريح
# (opt-in — حتى لا تظهر أيقونة في قائمة ستارت بالغلط أبداً).
# الاستخدام (مرة واحدة بعد كل نسخ):  ./install.sh [--install-menu]
set -u
APP_DIR="$(cd "$(dirname "$0")" && pwd)" || { echo "تعذر فتح مجلد البرنامج"; exit 1; }
cd "$APP_DIR" || exit 1

INSTALL_MENU=0
for a in "$@"; do
  case "$a" in
    --install-menu) INSTALL_MENU=1 ;;
    --no-desktop) ;; # اسم قديم: التجاهل هو الافتراضي الآن
  esac
done

chmod +x Beam.sh Beam Beam.command build.sh build-all.sh publish.sh install.sh packaging/build-portables.sh packaging/build-packages.sh packaging/debian/build-deb.sh packaging/appimage/build-appimage.sh packaging/fedora/build-rpm.sh packaging/fedora/make-tarball.sh packaging/icons.sh mobile/build-apk.sh mobile/sync-web.sh 2>/dev/null || true

DESK="$APP_DIR/Beam.desktop"
cat > "$DESK" <<EOF
[Desktop Entry]
Version=1.0
Type=Application
Name=Beam
Name[en]=Beam
Comment=Beam — مشاركة الملفات بين أجهزتك عبر شبكة خاصة — بدون إنترنت وبدون تثبيت
Comment[en]=Beam — share files between your devices over a private network — offline, no install
Exec="$APP_DIR/Beam.sh"
Path="$APP_DIR"
Icon=$APP_DIR/icon.png
Terminal=false
StartupNotify=true
Categories=Network;FileTransfer;
Keywords=share;files;hotspot;beam;مشاركة;ملفات;Beam;
StartupWMClass=Beam
EOF
chmod +x "$DESK" 2>/dev/null
# Remove the stale pre-v1.6 desktop file so two Beam icons never appear.
rm -f "$APP_DIR/FileShare.desktop" "$HOME/Desktop/FileShare.desktop" 2>/dev/null

PUB="$APP_DIR/Publish.desktop"
cat > "$PUB" <<EOF
[Desktop Entry]
Version=1.0
Type=Application
Name=نشر Beam
Name[en]=Publish Beam
Comment=نشر تعديلات Beam حية بضغطة واحدة: بناء + اختبار + تركيب + تشغيل (تبقى النافذة مفتوحة للمتابعة)
Comment[en]=Publish Beam live in one click: build, test, install, run (window stays open)
Exec="$APP_DIR/publish.sh"
Path="$APP_DIR"
Icon=$APP_DIR/icon.png
Terminal=true
StartupNotify=true
Categories=Network;FileTransfer;
Keywords=publish;deploy;نشر;Beam;
StartupWMClass=Publish
EOF
chmod +x "$PUB" 2>/dev/null

echo "تم توليد اللانشرين في: $APP_DIR (Beam + نشر Beam)"

if command -v desktop-file-validate >/dev/null 2>&1; then
  if desktop-file-validate "$DESK" && desktop-file-validate "$PUB"; then
    echo "فحص الصياغة: سليم ✅"
  else
    echo "تحذير: فحص الصياغة أبلغ عن ملاحظات (راجع الأعلى)."
    exit 1
  fi
else
  echo "(أداة desktop-file-validate غير مثبتة — تم التخطي)"
fi

if [ "$INSTALL_MENU" = "0" ]; then
  echo "(تثبيت سطح المكتب/القائمة متخطى — للتثبيت: ./install.sh --install-menu)"
  exit 0
fi

# Install on the desktop + app menu so the icon is one click away.
installed=""
if [ -d "$HOME/Desktop" ]; then
  cp -f "$DESK" "$HOME/Desktop/Beam.desktop" 2>/dev/null && installed="$installed desktop"
  rm -f "$HOME/Desktop/FileShare.desktop" 2>/dev/null
fi
if [ -d "$HOME/.local/share/applications" ]; then
  cp -f "$DESK" "$HOME/.local/share/applications/beam.desktop" 2>/dev/null && installed="$installed menu"
fi
# Auto-trust on GNOME/Cinnamon so no "Allow Launching" step is needed.
for d in "$HOME/Desktop/Beam.desktop" "$HOME/.local/share/applications/beam.desktop" "$DESK"; do
  if [ -f "$d" ] && command -v gio >/dev/null 2>&1; then
    gio set "$d" metadata::trusted true 2>/dev/null || true
  fi
done
if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database "$HOME/.local/share/applications" >/dev/null 2>&1 || true
fi
if [ -n "$installed" ]; then
  echo "تم تثبيت الأيقونة في:$installed ✅"
else
  echo "تعذر التثبيت التلقائي — الخطوة اليدوية: كليك يمين على الأيقونة ← Allow Launching"
fi
