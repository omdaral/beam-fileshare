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
| `version` / `is_admin` | string/boolean | Version, and whether the requester is the device owner (localhost) |
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
- No code, no cookie, no remote login — any non-localhost request to any admin endpoint → `403`.

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
- `GET /download_zip?dir=Docs` streams the folder as zip (pre-check: empty is `404`, over the compression limit is `413`).
- `POST /delete` accepts `{file:path}` or `{dir:path}` (safe recursive delete inside the share folder + pruning empty folders).
- The chunked-upload protocol accepts `name` with a relative path — each file is an independent session (resume/checksums as-is).

## Transfer modes + always-compressed folders (v1.5.0)
- Folders are always compressed in the browser first (Deflate if available, otherwise STORE), then uploaded as one resumable `.zip` file; a single `.zip` file is stored as-is and not extracted.
- `upload_init` accepts `noverify:true` (turbo session: chunks without checksums) and `extract:true` (extract intent).
- `upload_complete` accepts `full_hash` (required for turbo sessions, `422` without it) and `extract`; the extract intent sticks to the session so it works with resume.
- After completion: background extraction into the tree (pre-check + caps + zip-slip protection + merge without overwriting), then the zip is deleted; any failure keeps the zip with only a log line.
- **Reliable** mode (2MB x3 + checksum/chunk, default) and **turbo** mode (8MB x6 + single final check) — a switch per upload/download operation, and turbo download skips IndexedDB persistence.

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
