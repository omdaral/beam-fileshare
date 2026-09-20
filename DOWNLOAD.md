# Beam — Download / التحميل

> One-click install (detects your OS/arch automatically):
> تثبيت بنقرة واحدة (يكتشف نظامك تلقائياً):
>
> ```bash
> curl -sSL https://raw.githubusercontent.com/AhmedFaseh/beam-fileshare/main/get-beam.sh | bash
> ```
>
> Or pick your file manually below. All files live on the [Releases page](https://github.com/AhmedFaseh/beam-fileshare/releases/latest) with `SHA256SUMS` + `MANIFEST.json` for verification.
> أو اختر ملفك يدوياً من الجداول بالأسفل. كل الملفات على [صفحة Releases](https://github.com/AhmedFaseh/beam-fileshare/releases/latest) مع بصمات التحقق.

Current version: `v1.7.0` — الإصدار الحالي.

---

## 1) Portable builds (recommended for most users) / النسخ المحمولة

No install, no admin rights. Just extract and double-click. بدون تثبيت وبدون صلاحيات مدير.

| Your system / نظامك | File / الملف | After download / بعد التحميل |
|---|---|---|
| Linux 64-bit (Intel/AMD) | [Beam-1.7.0-linux-amd64.tar.gz](https://github.com/AhmedFaseh/beam-fileshare/releases/latest/download/Beam-1.7.0-linux-amd64.tar.gz) | `tar xzf *.tar.gz && cd Beam-* && ./install.sh` then double-click **Beam** |
| Linux ARM (Raspberry Pi / ARM server) | [Beam-1.7.0-linux-arm64.tar.gz](https://github.com/AhmedFaseh/beam-fileshare/releases/latest/download/Beam-1.7.0-linux-arm64.tar.gz) | Same as above / نفس الخطوات |
| Windows 64-bit (Intel/AMD) | [Beam-1.7.0-windows-amd64.zip](https://github.com/AhmedFaseh/beam-fileshare/releases/latest/download/Beam-1.7.0-windows-amd64.zip) | Keep `Beam.exe` + `Beam.bat` together, double-click `Beam.bat` |
| Windows ARM (Snapdragon) | [Beam-1.7.0-windows-arm64.zip](https://github.com/AhmedFaseh/beam-fileshare/releases/latest/download/Beam-1.7.0-windows-arm64.zip) | Same as above / نفس الخطوات |
| Mac Intel | [Beam-1.7.0-macos-amd64.zip](https://github.com/AhmedFaseh/beam-fileshare/releases/latest/download/Beam-1.7.0-macos-amd64.zip) | Keep `Beam` + `Beam.command` together; first time: right-click → **Open** |
| Mac Apple Silicon (M1/M2/M3) | [Beam-1.7.0-macos-arm64.zip](https://github.com/AhmedFaseh/beam-fileshare/releases/latest/download/Beam-1.7.0-macos-arm64.zip) | Same as above / نفس الخطوات |

How to know amd64 vs arm64? / كيف تعرف معماريتك؟

```bash
uname -m   # x86_64 → amd64 | aarch64/arm64 → arm64
```

Windows: Settings → System → About → System type.
Mac: Apple menu → About This Mac → Chip (Intel = amd64, Apple M* = arm64).

## 2) Android & iPhone / أندرويد وآيفون

| Device | File | Install |
|---|---|---|
| Android | [Beam-1.7.0-android.apk](https://github.com/AhmedFaseh/beam-fileshare/releases/latest/download/Beam-1.7.0-android.apk) (~25MB) | Copy to phone → open → allow "unknown sources" once → open Beam → **Start server** |
| iPhone | No app needed | Join the same Wi-Fi → open the link in Safari → Share → **Add to Home Screen** |

Developers only: building the APK from source needs ~1.5GB of tools (Node 22 + JDK 21 + Android SDK + Go) — see `mobile/SETUP.md`. End users should just download the APK above.

## 3) System packages (Linux) / حزم النظام

| Distro | File | Install |
|---|---|---|
| Debian / Ubuntu 64-bit | [beam-fileshare_1.7.0-1_amd64.deb](https://github.com/AhmedFaseh/beam-fileshare/releases/latest/download/beam-fileshare_1.7.0-1_amd64.deb) | `sudo dpkg -i *.deb` (adds icon + `beam` command) |
| Debian / Ubuntu ARM | [beam-fileshare_1.7.0-1_arm64.deb](https://github.com/AhmedFaseh/beam-fileshare/releases/latest/download/beam-fileshare_1.7.0-1_arm64.deb) | Same / نفس الأمر |
| AppImage 64-bit | [Beam-1.7.0-x86_64.AppImage](https://github.com/AhmedFaseh/beam-fileshare/releases/latest/download/Beam-1.7.0-x86_64.AppImage) | `chmod +x *.AppImage && ./Beam-*.AppImage` (needs `libfuse2`: `sudo apt install libfuse2`) |
| AppImage ARM | [Beam-1.7.0-aarch64.AppImage](https://github.com/AhmedFaseh/beam-fileshare/releases/latest/download/Beam-1.7.0-aarch64.AppImage) | Same / نفس الأمر |
| Fedora (x86_64) | [beam-fileshare-1.7.0-1.x86_64.rpm](https://github.com/AhmedFaseh/beam-fileshare/releases/latest/download/beam-fileshare-1.7.0-1.x86_64.rpm) | `sudo dnf install *.rpm` |

Arch / Flatpak: see `packaging/README.md` (build from the tag — Flathub app-id cannot change after first acceptance).

## 4) Verify your download / التحقق من سلامة الملف

Each release attaches `SHA256SUMS` and `MANIFEST.json`. Compare:

```bash
sha256sum Beam-1.7.0-linux-amd64.tar.gz
# compare the output with the SHA256SUMS file from the Releases page
```

## 5) Releases page shows 404? / الصفحة تعطي 404؟

That means the repo is still **Private** or the `v1.7.0` tag was never pushed. The maintainer must:

1. Make the repo **Public** (Settings → General → Danger Zone → Change visibility).
2. Push the code and the tag: `git push origin main v1.7.0` (tag must equal `VERSION` — enforced by `.github/workflows/release.yml`).
3. Wait for the `Release` workflow to finish — it builds all 6 binaries + portables + deb/AppImage/rpm + APK, runs the smoke test, and publishes everything to Releases.

Local `dist/` paths (e.g. `dist/portables/...`) are **gitignored build outputs** — they exist only on the maintainer's machine and are never downloadable from GitHub. Always link to Releases, never to `dist/`.
