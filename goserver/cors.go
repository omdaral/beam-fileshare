package beamcore

import (
	"net/http"
	"net/url"
	"strings"
)

// CORS for the mobile wrapper (Capacitor Android + iOS).
//
// The launcher page runs inside the app WebView (Capacitor 8 default
// scheme serves it as "https://localhost" on Android; iOS uses
// "capacitor://localhost") and talks to the
// on-phone Go server at http://127.0.0.1:2004. The WebView enforces CORS,
// so without ACAO headers every fetch probe (/health, /api/status) fails
// and the start button looks dead even though the server is up.
// iPhone Safari PWA (Add to Home Screen) is same-origin and needs no CORS,
// but the same allowlist covers it when opened via loopback/beams names.
//
// Threat model stays intact: the page is LAN-shared (Wi-Fi password is the
// only secret) and every admin POST still requires BOTH a trusted origin
// (checkOrigin) AND a loopback peer (hasAdmin). Reflecting the app origin
// does not open admin APIs to random websites — their Origin won't match
// and their peer IP won't be loopback anyway.

// trustedAppOrigin reports whether an Origin header comes from the Beam
// Android wrapper (or a local dev page) and is therefore allowed CORS.
func trustedAppOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	switch scheme {
	case "capacitor", "ionic":
		// Capacitor/ionic wrapper origins (capacitor://localhost).
		return true
	case "http", "https":
		// Local pages opened directly (desktop preview, dev tools).
		// Only exact trusted names — no wildcard *.local (mDNS spoofable).
		if host == "localhost" || host == "127.0.0.1" || host == "::1" ||
			host == "beam" || host == "beam.local" {
			return true
		}
		return false
	default:
		return false
	}
}

// setCORS reflects a trusted Origin so the Android WebView can read
// API responses. Called at the top of Route before any WriteHeader.
func setCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" || !trustedAppOrigin(origin) {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Lang, X-Beam-Owner")
	w.Header().Set("Access-Control-Max-Age", "86400")
}
