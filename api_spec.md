# Status and Admin API Spec

## `GET /api/status` (public — feeds the network card and QR on both pages)

| Field | Type | Description |
|---|---|---|
| `running` | boolean | Whether the server is running (`true` practically always) |
| `ssid` | string | Wi-Fi network name, e.g. `Beam` |
| `ip` / `port` / `url` | string/number/string | Full login address, e.g. `http://192.168.1.5:2004` (port is fixed at 2004) |
| `clients` | any | `—` or list of IPs in hotspot mode |
| `lan_mode` | boolean | `true` for Wi-Fi, `false` for direct Hotspot |
| `security` | string | `wpa` or `open` |
| `wifi_password` | string | **Intentionally visible to everyone** — the page is a Wi-Fi QR sharing channel |
| `wifi_ssid` / `wifi_source` | string | Effective join SSID + source (`hotspot`/`auto`/`manual`/`none`) |
| `version` / `is_admin` | string/boolean | Version, and whether the requester is the device owner (loopback peer or valid `X-Beam-Owner` token) |
| `owner_token` | string | **Owner contexts only** (loopback/token/bridge): 64-hex session owner token. Absent from guest responses entirely |
| `default_lang` | string | Default language for new visitors (`ar` or `en`) — session-only, browser keeps a copy in localStorage |
| `shared_dir` | string | Path of the single share folder (`~/Downloads/Beam`) — shown on the page |
| `mdns_url` | string | `http://beam.local:2004` — friendly name that works on all systems (mDNS) |
| `plain_url` | string | `http://beam:2004` — works on Windows (LLMNR) |
| `idle_seconds` | number | Seconds remaining before idle auto-shutdown (`-1` if disabled) |

## Pages (single landing page for everyone)
- `/` and `/index.html` and `/admin` and `/guest` serve the same single page (compatibility with old links).
- Full-bleed sections: login hero + steps + network/QR + files + help — no narrow cards.
- The Settings ⚙️ button in the header and the `#ownerZone` window (modal with tabs: Network/General/Devices & Log/Device) appear for the device owner only — every admin API rejects anyone else (`403`).
- Language button in the header for everyone (local choice); the language menu inside Settings sets `default_lang` for new visitors after saving.
- No code, no cookie, no remote login — any request that is neither loopback nor presenting the session owner token → `403` on admin endpoints.
- Owner proof (deterministic, no heuristics): **(1)** loopback peer (`127/8`, `::1`) is automatically owner on any OS/device; **(2)** any other context presents the 256-bit session token in the `X-Beam-Owner` header (constant-time compare). The token is generated fresh on every server start (memory-only, dies with the process), disclosed only inside owner contexts (`owner_token` field, `BeamServer.ownerToken` bridge), sent in headers only (never URLs/logs), and grants full owner powers. A paired browser stores it in localStorage and is recognized from any URL afterwards.

## Admin endpoints (device owner only)
| Path | Description |
|---|---|
| `GET /api/net/status` | Full status (IP list for the owner, count for guests) |
| `GET /api/net/clients` | Lightweight live-device path (polled every 3 seconds): `{clients, clients_count}` |
| `POST /api/net/start` / `/api/net/stop` | Start/stop the network (LAN/hotspot) |
| `GET /api/logs` / `POST /api/logs/clear` | View the log (in-memory ring: last 500 lines) / **clear it completely** |
| `GET`+`POST /api/config` | Settings **for the session only — no config.json file** (no code field of any kind — removed permanently). Accepts `port` and `max_file_mb` and `ssid` and `default_lang` (`ar`/`en` only — anything else is `400`). The UI keeps a copy in localStorage and re-fills it automatically |
| `POST /api/server/stop` | Stop the server (exits the process) — has a round ⏻ icon-only button in the page header (owner only, one click then closes the tab or shows a stopped screen) |

## Folders (v1.4.0)
- Paths are relative in the style `Docs/2026/a.pdf` (depth ≤ 10, length ≤ 512, no dot segments/symlinks) — violations return `400`.
- `GET /files` → `{files:[{name,path,size,mtime}], dirs:[{path,files,over}], truncated}`.
  `over` is empty (openable) or `files`/`depth`/`list`: an over-limit folder is **not shown as a tree** — the UI offers it as a direct zip row with the reason.
- `GET /download?file=a/b.txt` and `GET /file_hash?file=a/b.txt` work with paths (with Range).
- `GET /download_zip?dir=Docs[&method=store]` streams the folder as ZIP-STORE only (no compression, ZIP-only policy for widest device compat; `method=deflate` or any other codec is `400 zip_store_only`). Default `method=store` sends an exact `Content-Length` whenever the archive fits plain zip32 limits, otherwise it falls back to chunked streaming. Empty subdirectories are included as explicit entries. `Content-Disposition` carries both `filename="X.zip"` (ASCII fallback) and `filename*=` (UTF-8).
  `GET /download_zip?dir=Docs&preflight=1` runs the same pre-checks and returns `{ok:true, files, bytes, name, method}` as JSON without streaming — the web UI calls it first so server errors surface as readable text instead of a corrupt download.
- `POST /delete` accepts `{file:path}` or `{dir:path}` (safe recursive delete inside the share folder + pruning empty folders).
- The chunked-upload protocol accepts `name` with a relative path — each file is an independent session (resume/checksums as-is).

## Transfer modes + always-compressed folders (v1.5.0, unified in v1.6.x)
- Folders upload directly file-by-file with no browser compression (each file its own resumable session keeping the same structure); a single `.zip` uploaded with `extract:true` is unpacked by the server after completion.
- `upload_init` accepts `noverify:true` (legacy turbo flag: chunks without per-chunk checksums) and `extract:true` (extract intent). The UI in v1.6.x uses a single fast+reliable mode (verified pieces); `noverify` remains accepted for backward compatibility only.
- `upload_complete` accepts `full_hash` (required for legacy `noverify` sessions, `422` without it) and `extract`; the extract intent sticks to the session so it works with resume.
- After completion: background extraction into the tree (staged under a hidden dot dir, then moved without overwriting; pre-check + live byte/file caps + zip-slip protection + entry modtimes kept), then the zip is deleted; any failure keeps the zip with only a log line.
- **Current behavior (v1.6.x):** single fast+reliable mode (parallel verified pieces, no UI switch); large files fall back to direct browser download automatically.

## HTTPS (optional self-signed LAN certificate)

- Desktop serves **plain HTTP by default** (`http://<ip>:2004`): no warnings,
  simplest for LAN use.
- Opt in with `--tls` / `BEAM_TLS=1`: the cert is generated once and kept in
  `~/Downloads/Beam-Temp/.beam-tls/` (never listed or downloadable). The
  SHA-256 fingerprint prints on boot and shows in the UI — accept the browser
  warning once, then compare fingerprints.
- Every URL the server emits (`/api/status` `url`/`mdns_url`/`plain_url`,
  hotspot info, LAN fallback) uses the live scheme.
- `--no-tls` is kept as a deprecated no-op (HTTP is the default). The Android
  wrapper stays plain HTTP (the system WebView cannot silently trust a
  self-signed cert — enabling it needs a native `onReceivedSslError`
  handler, planned with the SAF picker bridge).

## Wi-Fi sharing (auto-detect + manual fallback)

- `/api/status` and `/api/net/status` expose `wifi_ssid` + `wifi_password` +
  `wifi_source` (`hotspot`/`auto`/`manual`/`none`): in hotspot mode the
  credentials we created; in LAN mode the OS-detected Wi-Fi
  (Linux `nmcli`, Windows `netsh`, macOS `networksetup`) with the owner's
  manual `wifi_ssid`/`wifi_password` as fallback when the PSK needs privileges.
- `GET /api/wifi/detect` (owner only) forces a fresh OS probe: `{ok, ssid,
  password, security, source}`.
- `POST /api/config` accepts `wifi_ssid`/`wifi_password`/`wifi_security`
  (owner only, session-only) — the Settings → Network → Wi-Fi box edits them
  with autosave, plus an Auto-detect button.

## QR (bundled qrcode-generator library — no CDN)
`/vendor/qrcode.js` is served from the binary. The Wi-Fi QR tabs (with embedded password) and link QR work on both pages. Acceptance gate: real scan with a mobile phone.

## Split UI (no bundler — v1.6.x)
- `GET /` is a light Shell skeleton + `/app/*.js` (same `go:embed`, no build step):
  `config` (limits) → `i18n` → `api` (transfer/SHA-256) → `ui` → `files` →
  `upload` + `upload_queue` + `upload_zip` → `download` → `net` → `app` (boot).
- `GET /api/status` includes `limits` (chunks/compression/search/polling) — the UI
  applies them via `applyLimits()` and resets timers (`resetPolling`) instead of a hard-coded copy.
- Security: `Content-Security-Policy` + `X-Frame-Options: SAMEORIGIN` + `X-Content-Type-Options: nosniff` on HTML and files.
- Updates: unified tick + full pause when `document.hidden` + `fetchClients` for the owner only with the modal open.

## Centralized settings (v1.6.x)
- All limits, sizes, and timeouts live in `goserver/limits.go` (single source) and accept
  `BEAM_*` overrides (e.g.: `BEAM_PORT`, `BEAM_PIECE_MAX`, `BEAM_IDLE_TIMEOUT=90s`,
  `BEAM_CHUNK_MAX_V2`, `BEAM_MULTIPART_MAX`) — empty/bad values are safely ignored.

## Mobile: PWA-lite + native wrapper (v1.6.0)
- `GET /manifest.json` (bundled static) + `/vendor/icon-192.png` + `/vendor/icon-512.png` + `/vendor/apple-touch-icon.png` + `theme-color`/Apple tags in the page — the Beam icon on the mobile home screen opens the UI fullscreen (manual install from the browser).
- No Service Worker by design: a LAN origin (`http://IP:2004`) is not a secure context so registering one is impossible — no dead code.
- `mobile/` folder (Capacitor): light setup page (discovery/network scan + save link) directs the WebView to the server — all APIs stay same-origin with no changes. The background wire in the page (`bgKick`/`bgIdleCheck`) is a no-op on the web and runs a foreground service inside the native app only.

## No side files + auto-shutdown (v1.6.0)
- No `config.json`, no `logs/` folder, no `Shared/` next to the program — permanently.
- Port is fixed at **2004** (`http://<ip>:2004`), changed only with the `--port` flag.
- Single share folder: `~/Downloads/Beam` (auto-created + one-time migration from the old `Shared/`).
- The server opens the browser itself on start (except with `--no-browser`).
- Any HTTP request (except `/health`) resets the idle counter; after **5 hours** of silence (`--idle-timeout`, and `0` disables it) the server shuts itself down.
