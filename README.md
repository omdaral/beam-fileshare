# Beam — Share your files between your devices in one click

One small program per OS — **no Python and nothing to install** on your machine.
Your files live in one place you know (`Downloads/Beam`), settings apply to the current session only, and there are no scattered files next to the program.

> **Current version:** `v1.6.0` — the version number appears in a badge on the home page so you can confirm you are running the latest build.

---

## Quick start

### Linux (no install)

1. Copy the whole program folder anywhere, then run this command once after each copy:
   ```bash
   ./install.sh
   ```
2. Open the folder in the file manager and double-click the **Beam** icon.
   - First time only: right-click it and choose **Allow Launching** if prompted.
   - Alternative: double-click the `Beam` file itself — it will open the browser automatically.
3. The program page will open in the browser: at the top a large **device login link** with a copy button — share it with any device.
4. Your files are saved in `~/Downloads/Beam` (Downloads folder → Beam), and its path is shown on the same page.
5. **Stopping:** round ⏻ button labeled **"Stop server"** at the top of the page (shown to the device owner only), or `Ctrl+C` in the terminal.
6. **Peace of mind:** if you leave the server running, it will stop by itself after **5 hours with no activity** (any upload, download, or browsing resets the counter).

> The login link is always fixed on port **2004**: `http://<ip>:2004` — save it in your browser once.

### Windows (no install)

1. Put these files next to each other: `Beam.exe` + `Beam.bat` + `VERSION`.
2. Double-click `Beam.bat` (or `Beam.exe` directly) — the server will start and automatically open the browser on port **2004**.
3. The first run on a hotspot network may ask for administrator permission — approve it once.
4. Your files are in `Downloads\Beam` inside the user folder.

### Mac (no install)

1. Put these files next to each other: `Beam` (Mac build from the `dist/` folder) + `Beam.command` + `VERSION`.
2. First time: right-click `Beam.command` and choose **Open** to bypass the security check once, then double-click it from then on.
3. Mac runs in **local-network mode only** — join the same Wi-Fi and open the login link.
4. Your files are in `~/Downloads/Beam`.

---

## Access from mobile or any device (browser only — no apps)

1. Join the same Wi-Fi network (or the `Beam` network in hotspot mode).
2. Open the browser at the address printed by the program, for example: `http://192.168.1.5:2004`
3. You land directly on the page: **everyone can upload, download, and delete** — no accounts or passwords.
4. **On mobile:** from the browser menu choose **Add to Home Screen** — the Beam icon will be installed and open fullscreen like an app (illustrated guide inside the Help section on the page).
5. The **network status** card shows the Wi-Fi QR + link QR + copy button + share-folder path + auto-shutdown counter.
6. Settings, network, and log are behind the ⚙️ button at the top — **from the host device only**, no remote control.

> **Native Android app (optional):** the `mobile/` folder runs the server on the phone itself. See `mobile/SETUP.md` for instructions, then build the app with `./mobile/build-apk.sh`.

---

## Folders and files

- The **Upload folder** button: always compressed in your browser first, then uploaded as one fast file, and extracted on the server after completion.
- The view is a **collapsible tree** with **instant search** that filters locally without waiting.
- Each folder has a **compressed download** button and a **delete** button (with double confirmation), and single files are deleted individually.
- A folder that exceeds the maximum limit is not shown — it appears as a direct-download row with the reason stated.

## Large files (up to 20 GB by default)

- Uploads **start immediately in small parallel chunks with a checksum per chunk** — no waiting.
- If the connection drops, re-select the same file and only the missing part resumes — and damaged chunks are retried alone, not the whole file.
- A **speed and time-remaining** counter shows during upload and download.
- The **resumable upload sessions** panel shows what stopped, with a resume button.
- The list **updates automatically** every few seconds — there is no refresh button.
- Two transfer modes: **reliable** (default — for unstable networks) and **turbo** (for maximum speed on stable networks).

## Network modes

| Mode | Description |
|---|---|
| **Wi-Fi (LAN)** | Default — works on the existing network |
| **Hotspot** | Private network with a name and password of your choice (8+ characters) |
| **Open network** | Linux only + clear warning (Windows requires a password from the OS itself) |

> The only file security is the Wi-Fi password — choose your network carefully.

---

## Appearance and language

- Clean light theme by default with **a toggle at the top for dark mode** — your choice is saved in your browser.
- **Arabic / English:** language button at the top for everyone, and the device owner sets the default visitor language from Settings.
- Embedded Arabic font works **without internet**, and QR codes are always black-on-white to guarantee scanning in any theme.

## Supported platforms

| System | Architecture | Launch |
|---|---|---|
| Linux | amd64 / arm64 | `Beam.sh` or the icon |
| Windows | amd64 / arm64 | `Beam.bat` |
| Mac | amd64 / arm64 | `Beam.command` (local network only) |

## Installing as system packages (optional)

- **Ready-made portable packages:** `dist/portables/` — copy the right package to any machine and double-click, no install.
- **Debian / Ubuntu:** `.deb` file from `dist/debian/` — adds an icon and the `beam` command.
- **Fedora / Arch / AppImage / Flatpak:** see `packaging/README.md` and `PACKAGING_NOTES.md`.
- Full build: `./build-all.sh` then `./packaging/build-packages.sh` — or `./publish.sh` for everything in one go.

---

## For developers (needs Go once)

```bash
go -C goserver run ./cmd/beam                                   # local-network mode
go -C goserver run ./cmd/beam --hotspot --password 12345678     # private network
go -C goserver run ./cmd/beam --no-browser --idle-timeout 30m   # no browser + shut down after 30 minutes idle
go -C goserver test ./...                                       # all tests
```

## Project contents

| Path | Description |
|---|---|
| `goserver/` | Go server code + web page in `goserver/web/index.html` |
| `mobile/` | Android app (server on the phone) |
| `packaging/` | Package build scripts for all systems |
| `docs/` | Project documentation / constitution |
| `Beam.sh` / `Beam.bat` / `Beam.command` | Double-click launchers |
| `install.sh` / `build-all.sh` / `publish.sh` | Install, build, and publish |

> Note: there is no `config.json` and no `logs/` folder — settings are for the session only (+ a copy in the owner's browser), and the log is in memory only (last 500 lines).

---

## Troubleshooting

- **Icon does not work:** run `./install.sh` inside the folder, then right-click the icon → **Allow Launching**, or click `Beam` directly.
- **Verify the file is intact:** run `./Beam --help` — if it prints the commands, the file is fine. If you see `Permission denied`, run `chmod +x Beam Beam.sh`.
- **One-click publish:** the **Publish Beam** desktop icon (created by `./install.sh`) checks, builds, and runs automatically, and rolls back to the previous version if the new one fails.
