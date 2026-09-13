# Building the Beam App for Android (one-time setup)

> Code is complete and ready in `mobile/` — these steps only install the build tools (~1.5GB one-time download), then one command always builds the APK.

## 1) Requirements (one-time install)

1. **Node.js 18+**: from https://nodejs.org (check: `node -v`)
2. **JDK 21** (Capacitor 8 requires 21 — 17 is not enough):
    - Without root (tested here): download Temurin 21 from https://adoptium.net → extract to `~/jdks` — the build script detects it automatically.
    - Or: `sudo apt install openjdk-21-jdk`.
    - (Check: `java -version` → 21+)
3. **Go 1.22+** (for the server engine inside the app) + **NDK**:
    ```bash
    sdkmanager "ndk;26.3.11579264"
    ```
    First `./mobile/build-apk.sh` build installs `gomobile` and wires the engine automatically (needs internet the first time only).
3. **Android SDK**:
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
3. Open Beam ← type the server link shown in large type on the computer page (or 🔍 auto-search) ← enter.
4. When an upload starts: persistent "Beam يرفع…" ("Beam is uploading…") notification = foreground service is running — lock the screen and it will continue.

## 4) Checklist on a real device

- [ ] Upload 200MB with the screen locked → completes.
- [ ] Minimize the app mid-upload → completes without restart.
- [ ] Kill the app and reopen it → auto-resume finishes the rest.
- [ ] After all uploads finish: notification disappears (service stopped itself).
- [ ] Same page and features (QR/search/turbo) — no difference from the browser.

## 5) If upload stops in the background (known fallback)

Some devices (Xiaomi/Huawei/Oppo) kill apps aggressively: enable "Autostart" and "No battery restrictions" for the Beam app in the phone settings. If stopping persists, the fallback is documented in `goserver/web/index.html` (`BG-FALLBACK` comment: small local plugin that calls `resumeTimers`).

## 6) Notes

- iPhone: this package is Android only — for iPhone use "Add to Home Screen" from Safari (PWA-lite from the server), and iOS builds need a Mac.
- No developer accounts or stores needed — direct APK with debug signature is enough for manual distribution.
