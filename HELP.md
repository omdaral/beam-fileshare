# Beam — Help / مساعدة

> **English?** Full guide in [README.md](README.md) — **عربي؟** الدليل الكامل في [README_AR.md](README_AR.md) — **كل روابط التحميل:** [DOWNLOAD.md](DOWNLOAD.md).
> Current version: `v1.7.1` (check the badge on the home page).

This is the central help file (Arabic + English). For the full install guide see the READMEs above.

هذا هو ملف المساعدة المركزي (عربي + إنجليزي). لدليل التثبيت الكامل انظر ملفي README أعلاه.

---

## ٠) Downloading the right file / تحميل الملف الصحيح

- Easiest way (auto-detects your OS/arch):
  أسهل طريقة (تكتشف نظامك تلقائياً):
  ```bash
  curl -sSL https://raw.githubusercontent.com/omdaral/beam-fileshare/main/get-beam.sh | bash -s -- --install
  ```
- Manual: open [DOWNLOAD.md](DOWNLOAD.md) or the [Releases page](https://github.com/omdaral/beam-fileshare/releases/latest) and pick your row (Windows / Linux / Mac × amd64/arm64, Android APK, or system packages).
  يدوياً: افتح [DOWNLOAD.md](DOWNLOAD.md) أو [صفحة Releases](https://github.com/omdaral/beam-fileshare/releases/latest) واختر صف نظامك.
- Releases page shows `404`? The repo is still **Private** or the `v1.7.1` tag was never pushed — make the repo Public and run `git push origin main v1.7.1`, then the `Release` workflow publishes all assets.
  صفحة Releases تعطي `404`؟ المستودع ما زال **خاصاً** أو تاج `v1.7.1` لم يُدفع — اجعله Public ونفّذ الدفع، وسيبني الـ workflow كل الأصول تلقائياً.
- Verify integrity: compare SHA256 with `dist/SHA256SUMS` / `MANIFEST.json` attached to the release:
  للتحقق من سلامة الملف: قارن بصمة SHA256 مع المرفقة في الـ Release:
  ```bash
  sha256sum Beam-1.7.1-linux-amd64.tar.gz
  ```

---

## ١) Quick fixes / حلول سريعة

- Linux icon broken → run `./install.sh`, then right-click → **Allow Launching**.
  الأيقونة لا تعمل في Linux ← نفّذ `./install.sh` ثم كليك يمين ← **Allow Launching**.
- `Permission denied` → `chmod +x Beam Beam.sh`.
- Browser didn't open → copy the `http://<ip>:2004` address printed in the terminal and paste it manually.
  المتصفح لم يفتح ← انسخ الرابط المطبوع في الطرفية والصقه يدوياً.
- Host files: copy them to `~/Downloads/Beam` — they appear for everyone instantly.
  ملفات المضيف: انسخها إلى `~/Downloads/Beam` — تظهر للجميع فوراً.

---

## ٢) Troubleshooting / استكشاف الأعطال

| # | Problem / المشكلة | Cause / السبب | Fix / الحل |
|---|---|---|---|
| 1 | Port 2004 is busy / المنفذ 2004 مشغول | Another Beam copy (or another app) already uses port 2004 / نسخة Beam أخرى أو تطبيق آخر يستخدم المنفذ | Stop the old copy (`Ctrl+C` or the ⏻ **Stop server** button). Guests: ask the host for the correct address. / أوقف النسخة القديمة. الضيوف: اطلبوا العنوان الصحيح من المضيف |
| 2 | Firewall blocks access / الجدار الناري يحجب الدخول | OS firewall blocks incoming connections on port 2004 / جدار النظام يحجب الاتصالات الواردة على 2004 | Linux: `sudo ufw allow 2004/tcp`. Windows: allow `Beam.exe` on **Private** networks when prompted. Make sure guest and host are on the same network. / اسمح بالمنفذ 2004، وتأكد أن الجميع على نفس الشبكة |
| 3 | Icon needs "Allow Launching" / الأيقونة تطلب Allow Launching | GNOME/Nautilus marks copied `.desktop` files untrusted / مدير الملفات يعتبر ملف `.desktop` المنسوخ غير موثوق | Right-click the icon → **Allow Launching**. If it still fails, run `./install.sh` first (required **after every folder copy/move**), or double-click the `Beam` binary directly. / كليك يمين ← Allow Launching. ولو نُقل المجلد أعد تشغيل `./install.sh` |
| 4 | Browser does not open / المتصفح لا يفتح تلقائياً | No default browser, Wayland/minimal distro, or `--no-browser` flag / لا يوجد متصفح افتراضي أو خيار `--no-browser` | Copy the printed `http://<ip>:2004` link into any browser manually. Linux: check `xdg-open`. / انسخ الرابط المطبوع والصقه في أي متصفح يدوياً |
| 5 | AppImage does not start (missing libfuse2) / الـ AppImage لا يعمل (libfuse2 ناقص) | Modern distros (Ubuntu 22.04+) ship FUSE3, AppImage needs `libfuse2` / التوزيعات الحديثة فيها FUSE3 والـ AppImage يحتاج `libfuse2` | Install it once: `sudo apt install libfuse2` (Debian/Ubuntu) or `sudo dnf install fuse-libs` (Fedora). Alternative: use the portable `.tar.gz` from [DOWNLOAD.md](DOWNLOAD.md) — no FUSE needed. / ثبّت `libfuse2` مرة واحدة، أو استخدم النسخة المحمولة من [DOWNLOAD.md](DOWNLOAD.md) |
| 6 | Hotspot asks for password / الهوتسبوت يطلب صلاحية (pkexec/Admin) | Creating a hotspot changes system Wi-Fi — Linux asks via `pkexec`, Windows hotspot needs Administrator / إنشاء هوتسبوت يغيّر شبكة النظام فيطلب صلاحية | Linux: approve the `pkexec` dialog once (or run from a terminal to see the prompt). Windows: right-click `Beam.bat` → **Run as administrator** once on hotspot networks. Password must be 8+ chars. / وافق على طلب الصلاحية مرة واحدة، وكلمة السر 8+ أحرف |
| 7 | Wi-Fi drops during upload / انقطاع الواي فاي أثناء الرفع | Signal lost, hotspot stopped, or phone Doze killed the connection / فقدان الإشارة أو إيقاف الهوتسبوت أو قتل النظام للاتصال | Rejoin the same network, reopen the same link, then Resume from the sessions panel and **re-pick the same file** (browser security requires this) — only missing chunks resume. Host files copied to `~/Downloads/Beam` need no resume. / أعد الانضمام للشبكة ثم استأنف من لوحة الجلسات وأعد اختيار نفس الملف — يُستأنف الناقص فقط |
| 8 | Gatekeeper (Mac) blocks Beam / SmartScreen (Windows) blocks Beam | Unsigned binaries trigger macOS Gatekeeper / Windows SmartScreen warnings / الملفات غير موقّعة فتعترضها حماية النظام | **Mac:** right-click `Beam.command` → **Open** → **Open** (once), then double-click normally. **Windows:** click **More info** → **Run anyway** on the SmartScreen prompt (file is built locally from this repo). Mac runs **LAN-only**. / ماك: كليك يمين ← Open مرة واحدة. ويندوز: More info ← Run anyway |

---

## ٣) Folders: `~/Downloads/Beam` vs `Beam-Temp` / المجلدات

| Folder / المجلد | Use / الاستخدام |
|---|---|
| `~/Downloads/Beam` | Permanent share folder. **Host:** copy files here — visible to everyone instantly, no browser upload. / مجلد المشاركة الدائم. **المضيف:** انسخ ملفاتك هنا — تظهر للجميع فوراً |
| `~/Downloads/Beam-Temp` | Internal unified registry root for shared files — managed automatically, do not copy here manually. / جذر سجل المشاركة الداخلي — يُدار آلياً، لا تنسخ إليه يدوياً |

Host copies to `~/Downloads/Beam`. / المضيف ينسخ إلى `~/Downloads/Beam`.

---

## ٤) Notes / ملاحظات

- `Beam` = the program binary (server). `Beam.desktop` = just the desktop icon/shortcut (absolute paths — rerun `./install.sh` after every folder copy/move).
  `Beam` هو البرنامج نفسه، و`Beam.desktop` مجرد أيقونة بمسارات مطلقة — أعد تشغيل `./install.sh` بعد كل نسخ/نقل.
- `Beam.prev` = previous binary kept for **rollback only** — do not distribute it.
  `Beam.prev` نسخة سابقة **للرجوع فقط** — لا توزّعها.
- Login link is always on port **2004**: `http://<ip>:2004`. Encrypted LAN: run with `--tls` and open `https://<ip>:2004`.
  رابط الدخول دائماً على المنفذ **2004**.
- iPhone: no native app — Safari → **Add to Home Screen**. Android native APK: download from [Releases](https://github.com/omdaral/beam-fileshare/releases/latest/download/Beam-1.7.1-android.apk) (allow "unknown sources" once); build guide in `mobile/SETUP.md` is for developers only.
  آيفون: بدون تطبيق — من Safari اختر Add to Home Screen. أندرويد: حمّل الـ APK من Releases.
- Still stuck? Run `./Beam --help` — if it prints commands the binary is fine (icon/path issue). Copy the terminal output when asking for help.
  ما زالت المشكلة؟ نفّذ `./Beam --help` — لو طبع الأوامر فالبرنامج سليم والمشكلة في الأيقونة/المسار.
