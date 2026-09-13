# Embedding the UI in the Binary (Go)

One page for everyone: `goserver/web/index.html` (owner zone `#ownerZone` shows for localhost only — and the APIs reject anyone else), embedded in the binary via:

```go
//go:embed web/index.html
var indexHTML []byte
```

- `/` and `/index.html` and `/admin` and `/guest` serve the same page (no broken links).
- **i18n:** `STR` dictionary (default Arabic + English), `data-i18n` attributes for static text and `T()` function for dynamic text, language button in the header + settings row, `document.dir/lang` switch, choice persisted (`beam-lang`). Bilingual server messages via the `X-Lang` header (and the `goserver/i18n.go` table).
- If an `index.html` file sits next to the binary it is served instead of the embedded one (development mode only).

- `/` and `/index.html` and `/admin` and `/guest` serve the same single page (compatibility with old links).
- Owner zone is hidden from non-localhost, and every admin API rejects others (403).
- If an `index.html` file sits next to the binary it is served instead of the embedded one (development mode only).

- The server serves it on `GET /` with `Cache-Control: no-store` (always the latest).
- If an `index.html` file sits next to the binary it is served instead of the embedded one (development mode only).
- Status contract for management: `GET /api/status` (public) + `GET /api/net/status` (manager details).

## Testing with `curl`

```bash
go -C goserver run ./cmd/beam --port 2004 --no-browser &
curl -s http://127.0.0.1:2004/api/status
curl -s http://127.0.0.1:2004/files   # open directly with no login (security = Wi-Fi password)
kill %1
```
