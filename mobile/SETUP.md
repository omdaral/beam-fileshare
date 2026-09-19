# Building the Beam App for Android (one-time setup)

> Code is complete and ready in `mobile/` — these steps only install the build tools (~1.5GB one-time download), then one command always builds the APK.

## 1) Requirements (one-time install)

1. **Node.js 22+** (Capacitor 8 requires 22 — check: `node -v`; the build script auto-picks an nvm Node 22 if the default is older): from https://nodejs.org
2. **JDK 21** (Capacitor 8 requires 21 — 17 is not enough):
    - Without root (tested here): download Temurin 21 from https://adoptium.net → extract to `~/jdks` — the build script detects it automatically.
    - Or: `sudo apt install openjdk-21-jdk`.
    - (Check: `java -version` → 21+)
3. **Go 1.22+** (for the server engine inside the app) + **NDK**:
    ```bash
    sdkmanager "ndk;26.3.11579264"
    ```
    First `./mobile/build-apk.sh` build installs `gomobile` and wires the engine automatically (needs internet the first time only).
4. **Android SDK**:
    ```bash
    mkdir -p ~/Android/Sdk/cmdline-tools
    # Download commandlinetools-linux from https://developer.android.com/studio#command-line-tools-only
    unzip commandlinetools-*.zip -d ~/Android/Sdk/cmdline-tools
    mv ~/Android/Sdk/cmdline-tools/cmdline-tools ~/Android/Sdk/cmdline-tools/latest
    export ANDROID_HOME=~/Android/Sdk
    export PATH=$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools:$PATH
    # Put both lines in ~/.bashrc so they persist
    sdkmanager --licenses  # accept all with y
    sdkmanager "platform-tools" "platforms;android-34" "build-tools;34.0.0"
    ```

## 2) Build (every time)

```bash
cd mobile
./build-apk.sh
# Output: dist/mobile/Beam-<ver>-android.apk
```

First build downloads Gradle and Android components from the internet (slow the first time only).

## 3) Install on the phone

1. Copy the APK to the phone (USB/Bluetooth/Beam itself!) and open it.
2. Approve "Install from unknown sources" for this time.
3. Open Beam → tap **"▶ تشغيل السيرفر"** → status becomes **"يعمل الآن"** with the LAN address (e.g. `192.168.1.5:2004`), QR, share folder, and version badge.
4. Tap **"فتح المشاركة الكاملة على هذا الهاتف"** to open the full file UI (upload/search/turbo/settings) **inside the app** — do not open an external browser, or the native bridge (server/hotspot) stops working.
5. Optional hotspot: in the **"هوتسبوت من هذا الهاتف"** card tap start — Android chooses the SSID/password itself, they appear in the card and the QR above updates after devices join.
6. Optional client mode: in the **"الاتصال بجهاز آخر"** card type a Beam address running elsewhere (e.g. `192.168.1.5:2004`) to browse it from this phone.
7. While the server runs, a persistent "Beam يعمل" notice stays visible (this is what
keeps Android from killing the server in the background — the التشخيص card verifies it
live as "خدمة الخلفية: تعمل ✅"); it disappears when you stop the server. During uploads
the notice reads "Beam يرفع…" — lock the screen and it will continue.
8. On first start the app asks for background permission ("allow" it) so Doze/OEM battery
killers can't stop the server — verified live as "إعفاء البطارية: مُعفى ✅". If the phone
is ever force-stopped, reopening Beam auto-resumes the server with no tap.

## 4) Checklist on a real device

- [ ] Tap "تشغيل السيرفر" → allow the background-permission dialog → status "يعمل الآن" +
LAN address + QR + version badge (no "تعذر تشغيل السيرفر"), التشخيص shows
"خدمة الخلفية: تعمل ✅" + "إعفاء البطارية: مُعفى ✅", and the "Beam يعمل" notice is visible.
- [ ] If anything fails: the **التشخيص** card shows the live event log — tap "نسخ التشخيص" and paste it to debug (bridge/platform/URL/errors included).
- [ ] First launch on Android 13+ asks for notification permission (needed for the "Beam يرفع…" background notice).
- [ ] "فتح المشاركة الكاملة" opens the file UI inside the app (same QR/search/turbo as desktop).
- [ ] Hotspot card: start → SSID/password appear; stop → back to Wi-Fi mode.
- [ ] Upload 200MB with the screen locked → completes.
- [ ] Minimize the app mid-upload → completes without restart.
- [ ] Kill the app and reopen it → auto-resume finishes the rest.
- [ ] After all uploads finish: notification disappears (service stopped itself).

## 5) If the server stops in the background (should not happen anymore)

The app now defends itself in layers: verified foreground-service notice (auto-restarted
if lost) + battery-optimization exemption prompt + WiFi/CPU locks + auto-resume after a
process kill. If the server still dies on some device, copy التشخيص (it includes the
live "خدمة الخلفية / إعفاء البطارية" states) and check:
1. Is the "Beam يعمل" notice visible? If not, enable app notifications in Android settings.
2. Does التشخيص say "إعفاء البطارية: مقيّد ⛔"? Tap "طلب إعفاء من تحسين البطارية" and allow.
3. Last resort on aggressive ROMs (Xiaomi/Huawei/Oppo): enable "Autostart" and
"No battery restrictions" for Beam in the phone settings.

## 6) Notes

- iPhone: this package is Android only — for iPhone use "Add to Home Screen" from Safari (PWA-lite from the server), and iOS builds need a Mac.
- No developer accounts or stores needed — direct APK with debug signature is enough for manual distribution.
