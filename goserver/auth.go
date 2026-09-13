package beamcore

import (
	"net/http"
	"net/url"
	"strings"
)

func hasAdmin(r *http.Request) bool {
	return isLoopback(clientIP(r))
}

// checkOrigin mirrors fileshare._check_origin (basic CSRF guard).
func checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		// No Origin (curl/Capacitor/health): require JSON content-type on
		// POST so classic <form> CSRF (urlencoded/text) is rejected.
		// Frontend always sends application/json for admin POSTs.
		if r.Method == "POST" {
			ct := strings.ToLower(r.Header.Get("Content-Type"))
			if !strings.Contains(ct, "application/json") {
				return false
			}
		}
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.ToLower(u.Host) == strings.ToLower(r.Host)
}
func adminOnly(w http.ResponseWriter, r *http.Request) bool {
	if !checkOrigin(r) {
		sendJSON(w, r, 403, map[string]interface{}{"error": tr(reqLang(r), "csrf")})
		return false
	}
	if !hasAdmin(r) {
		sendJSON(w, r, 403, map[string]interface{}{"error": tr(reqLang(r), "owner_only")})
		return false
	}
	return true
}
