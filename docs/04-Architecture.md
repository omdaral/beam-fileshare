# 04 - Architecture

> The technical foundation that only changes by updating this file. Goal: one file, no libraries on the user's machine, Windows + Linux.

## 1. Final Technical Decision
- **Language:** Go (stdlib only — no external dependencies) + one static binary per system.
  - Why: no need for Python/pip/tkinter on the user's machine at all — produces `Beam`
    for Linux and `Beam.exe` for Windows (`CGO_ENABLED=0 go build` command) with server + web page inside.
  - Historical note: the app was Python (PyInstaller) then fully replaced with Go —
    any reference to Python/PyInstaller/tkinter in this constitution is void.
- **Server:** Internal HTTP Server (Go net/http) on fixed port `2004` — opens the browser itself on launch, and shuts itself down after 5 hours idle.
- **UI:** Raw HTML + CSS + JS embedded via go:embed (no CDN because we're offline) + local Cairo font.
- **Desktop:** No window — full control from the browser (Admin section visible only to the host machine).

## 2. Components (Modules)
```
Beam(.exe)
├── 1. Hotspot Manager
│   ├── Windows: netsh / PowerShell (Mobile Hotspot API)
│   └── Linux: nmcli device wifi hotspot (NetworkManager)
├── 2. File Server (net/http)
│   ├── GET /         -> files page (direct entry, no code)
│   ├── GET /files    -> JSON list
│   ├── POST /upload_init {uuid,name,size} -> start/resume session {id, offset}
│   ├── POST /upload_chunk?id=&offset= (raw bytes streamed to disk) -> {offset}
│   ├── GET /upload_status?id= -> resume position
│   ├── POST /upload_complete {id} -> atomic assembly {saved}
│   ├── GET /download?file= -> streaming download
│   ├── POST /delete  -> delete (all allowed — by user decision)
│   └── GET /api/status -> network status for UI (ssid/security/wifi_password)
├── 3. Browser Admin (Modal settings window — shown to host machine only)
│   ├── Network tab: LAN/Hotspot choice + SSID/password + start/stop button
│   ├── General tab: port + file limit + default visitor language (default_lang)
│   ├── Show SSID/Bass + QR + IP:Port (landing page for everyone)
│   └── Log + connected devices (owner only — any admin API rejects others with 403)
└── 4. Storage + Lifecycle
    ├── Single share folder: `~/Downloads/Beam` (known place — no bloatware)
    ├── Settings: session memory only (+ localStorage copy in owner browser) — no config.json
    ├── Log: memory ring (last 500 lines in web page) — no logs/
    └── Auto-close after 5 hours idle + prominent red ⏻ button in header for manual stop
```

## 3. Network & Hotspot
- **Primary Hotspot mode:**
  - Windows default IP: `192.168.137.1` — Linux usually `10.42.0.1` (auto-detected and shown in UI).
  - Default SSID: `Beam` — WPA2 encryption — 8+ character password.
  - Requires Admin/sudo first time (to create network and open port in firewall).
  - Wi-Fi card must support AP Mode — we check at launch and if it fails show a message + LAN Mode button.
- **Alternative LAN Mode:**
  - If Hotspot is impossible, the app runs on existing Wi-Fi and shows current device IP + QR.
- **Internet sharing:** If host machine is on Ethernet, enable Sharing (optional). If only on WiFi, file sharing still works without internet.

## 4. Files and Folders (actual project structure)
```
تطبيبق مشاركة ملفات/
├── docs/ (project constitution - this folder)
├── goserver/ (Go source: server + hotspot + admin + tests + embedded web/index.html)
├── Beam / Beam.exe (built binary per system — not uploaded)
├── Beam.sh / Beam.bat / Beam.command (launcher: starts server if stopped; server opens browser itself)
├── install.sh (generates icon paths) + Beam.desktop (icon — generated, not uploaded)
├── build.sh / build.bat (build: go vet + go test + os/arch matrix in dist/ + versions.json)
└── no config.json, no logs/, no Shared/ — (settings are session-only, log is memory, files in ~/Downloads/Beam)
```

## 5. Security (by user decision: Wi-Fi password is the only line of defense)
1. Strong WPA2 network password (12+ chars recommended, changed periodically) — any device on network enters directly.
2. Block `../` and hidden files in filenames (Path Traversal + temp hiding).
3. Single-file size limit: unlimited by default (`max_file_mb = 0`, changeable per session from Settings) — chunked upload never holds file in RAM.
4. Memory log (last 500 lines in web page): who uploaded/downloaded/deleted what and when (for review during session).
5. Atomic writes prevent downloading an incomplete file during upload; locking prevents losing a file on same-name concurrency.
6. Open network (Linux only): `wifi_open=true` — with explicit warning; Windows rejects it with a clear message.
- **Future (not now):** per-client speed limit + IP ban + internal HTTPS + optional entry code.

## 6. Build Phases (in order - no skipping)
1. **Phase 1:** Go server + web page (upload/download) and test on plain LAN without Hotspot.
2. **Phase 2:** Windows then Linux Hotspot module + QR display + automatic IP detection.
3. **Phase 3:** Browser admin (localhost manager only — no remote entry) + session settings + memory log + Arabic error messages.
4. **Phase 4:** Build with `./build.sh` (Linux) / `build.bat` (Windows)
   + `./install.sh` after every folder copy + 8-device test + README file.

## 7. Mandatory Testing Before Delivery
- [ ] Double-click on clean machine without Python/Go -> runs? (static binary needs no library)
- [ ] Android + iPhone join and upload/download?
- [ ] 100MB file round-trip unbroken?
- [ ] Wi-Fi disconnect during upload -> clear message?
- [ ] Stop and start network 5 times without restart?

## 8. Golden Rule
Any new library must be bundled in the final file and require no install from the user. If a library breaks the rule -> rejected even if easier in code.
