package beamcore

import (
	"embed"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

//go:embed web/index.html
var indexHTML []byte

//go:embed web/app/style.css web/app/config.js web/app/i18n.js web/app/api.js web/app/ui.js web/app/files.js web/app/upload.js web/app/upload_queue.js web/app/upload_zip.js web/app/download.js web/app/shares.js web/app/net.js web/app/app.js
var appFS embed.FS

//go:embed web/qrcode-vendor.js web/cairo-400.woff2 web/cairo-700.woff2 web/cairo-900.woff2 web/icon-192.png web/icon-512.png web/apple-touch-icon.png
var vendorFS embed.FS
var (
	diskMu     sync.Mutex
	diskPages  = map[string][]byte{}
	diskPageMT = map[string]int64{}
)

// appContentTypes whitelists split UI files (no bundler, same go:embed).
var appContentTypes = map[string]string{
	"style.css":       "text/css; charset=utf-8",
	"config.js":       "application/javascript; charset=utf-8",
	"i18n.js":         "application/javascript; charset=utf-8",
	"api.js":          "application/javascript; charset=utf-8",
	"ui.js":           "application/javascript; charset=utf-8",
	"files.js":        "application/javascript; charset=utf-8",
	"upload.js":       "application/javascript; charset=utf-8",
	"upload_queue.js": "application/javascript; charset=utf-8",
	"upload_zip.js":   "application/javascript; charset=utf-8",
	"download.js":     "application/javascript; charset=utf-8",
	"shares.js":       "application/javascript; charset=utf-8",
	"net.js":          "application/javascript; charset=utf-8",
	"app.js":          "application/javascript; charset=utf-8",
}

// vendorContentTypes whitelists servable vendor files (offline, embedded).
var vendorContentTypes = map[string]string{
	"qrcode.js":            "application/javascript; charset=utf-8",
	"cairo-400.woff2":      "font/woff2",
	"cairo-700.woff2":      "font/woff2",
	"cairo-900.woff2":      "font/woff2",
	"icon-192.png":         "image/png",
	"icon-512.png":         "image/png",
	"apple-touch-icon.png": "image/png",
}

// serveIndexBody serves the single embedded page, with dev override:
// index.html next to the binary wins when present (unless disabled via
// BEAM_ALLOW_DISK_UI=0 — prevents local malware from hijacking the LAN UI).
func allowDiskUI() bool {
	if v := os.Getenv("BEAM_ALLOW_DISK_UI"); v == "0" || v == "false" || v == "no" {
		return false
	}
	return true
}
func serveIndexBody() []byte {
	if allowDiskUI() {
		p := filepath.Join(BaseDir, "index.html")
		if st, err := os.Stat(p); err == nil {
			mt := st.ModTime().UnixNano()
			diskMu.Lock()
			defer diskMu.Unlock()
			if diskPages["index"] == nil || mt != diskPageMT["index"] {
				if body, err := os.ReadFile(p); err == nil {
					diskPages["index"] = body
					diskPageMT["index"] = mt
				}
			}
			if diskPages["index"] != nil {
				return diskPages["index"]
			}
		}
	}
	return indexHTML
}

// setSecurityHeaders adds hardening headers (XSS/clickjacking/MIME).
// CSP allows self + inline (single-file UI without bundler) + blob:/data:
// for chunk downloads; object/base restricted.
// frame-ancestors also allows the on-phone Capacitor wrapper
// (capacitor://localhost on Android AND iOS, ionic://localhost) so the
// mobile launcher can embed the full UI in an iframe while keeping its
// native bridge alive. HTTPS loopback variants are included because the
// desktop defaults to self-signed HTTPS while the phone engine is HTTP.
// X-Frame-Options is intentionally omitted: SAMEORIGIN would block that
// same embedding in browsers that still honor it, and CSP frame-ancestors
// already governs.
func setSecurityHeaders(w http.ResponseWriter, isHTML bool) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	if isHTML {
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; "+
				"style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; "+
				"font-src 'self' data:; connect-src 'self'; media-src 'self' blob:; "+
				"object-src 'none'; base-uri 'self'; frame-ancestors 'self' capacitor://localhost capacitor://* ionic://localhost ionic://* http://localhost http://localhost:* https://localhost https://localhost:* http://127.0.0.1:* https://127.0.0.1:* http://beam.local:* https://beam.local:*; form-action 'self'")
	}
}
func writeHTML(w http.ResponseWriter, r *http.Request, body []byte) {
	setSecurityHeaders(w, true)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(body)), 10))
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.WriteHeader(200)
	if r.Method == "HEAD" {
		return
	}
	_, _ = w.Write(body)
}

// handleVendor serves embedded vendor files (offline-safe, no CDN).
// Only whitelisted names under /vendor/ are served.
func handleVendor(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/vendor/")
	ctype, ok := vendorContentTypes[name]
	if !ok || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		sendEmpty(w, r, 404)
		return
	}
	body, err := vendorFS.ReadFile("web/" + vendorFile(name))
	if err != nil {
		sendEmpty(w, r, 404)
		return
	}
	setSecurityHeaders(w, false)
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(body)), 10))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(200)
	if r.Method == "HEAD" {
		return
	}
	_, _ = w.Write(body)
}

// handleApp serves split UI files (/app/style.css, /app/app.js).
// No bundler: same go:embed, ordered <script> tags in index.html.
// Dev override: <BaseDir>/app/<name> wins when present (like index.html),
// unless BEAM_ALLOW_DISK_UI=0.
func handleApp(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/app/")
	ctype, ok := appContentTypes[name]
	if !ok || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		sendEmpty(w, r, 404)
		return
	}
	if allowDiskUI() {
		if p := filepath.Join(BaseDir, "app", name); true {
			if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
				if body, err := os.ReadFile(p); err == nil {
					setSecurityHeaders(w, false)
					w.Header().Set("Content-Type", ctype)
					w.Header().Set("Content-Length", strconv.FormatInt(int64(len(body)), 10))
					w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
					w.WriteHeader(200)
					if r.Method == "HEAD" {
						return
					}
					_, _ = w.Write(body)
					return
				}
			}
		}
	}
	body, err := appFS.ReadFile("web/app/" + name)
	if err != nil {
		sendEmpty(w, r, 404)
		return
	}
	setSecurityHeaders(w, false)
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(body)), 10))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(200)
	if r.Method == "HEAD" {
		return
	}
	_, _ = w.Write(body)
}

// vendorFile maps the public /vendor/ name to the embedded web/ filename.
func vendorFile(name string) string {
	if name == "qrcode.js" {
		return "qrcode-vendor.js"
	}
	return name // cairo-*.woff2 keep their names
}

// handleIndex serves the single page to everyone; owner-only sections
// are revealed client-side, and every admin API still enforces localhost.
func handleIndex(w http.ResponseWriter, r *http.Request) {
	writeHTML(w, r, serveIndexBody())
}
func handleHealth(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, r, 200, map[string]interface{}{"ok": true})
}

// installManifest is the PWA-lite manifest (installable home-screen icon).
// No service worker: the LAN origin (http://IP:2004) is not a secure
// context, so SW registration is impossible there by design — the manifest
// + icons still give a full-screen home-screen app on Android/iOS.
const installManifest = `{"name":"Beam","short_name":"Beam",` +
	`"description":"Beam — مشاركة الملفات بين أجهزتك عبر شبكة خاصة",` +
	`"start_url":"/","scope":"/","display":"standalone","dir":"auto","lang":"ar",` +
	`"theme_color":"#F7F5F0","background_color":"#F7F5F0",` +
	`"icons":[{"src":"/vendor/icon-192.png","sizes":"192x192","type":"image/png"},` +
	`{"src":"/vendor/icon-512.png","sizes":"512x512","type":"image/png",` +
	`"purpose":"any maskable"}]}`

func handleManifest(w http.ResponseWriter, r *http.Request) {
	body := []byte(installManifest)
	w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(body)), 10))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(200)
	if r.Method == "HEAD" {
		return
	}
	_, _ = w.Write(body)
}
