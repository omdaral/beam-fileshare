package beamcore

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// Session owner token: 256-bit random per server start, memory-only.
// Loopback peers are owners automatically; any other context proves
// ownership by presenting this token in the X-Beam-Owner header.
// No heuristics (no UA/IP sniffing): loopback math or token crypto.
var (
	ownerMu        sync.RWMutex
	ownerToken     [32]byte
	ownerTokenText string
)

// rotateOwnerToken generates a fresh session token. Called on every Run.
func rotateOwnerToken() {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Practically impossible; fall back to time-mixed entropy rather
		// than running with an empty token (empty must never validate).
		for i := range b {
			b[i] = byte(nowNano() >> uint(i%8))
		}
	}
	ownerMu.Lock()
	ownerToken = b
	ownerTokenText = hex.EncodeToString(b[:])
	ownerMu.Unlock()
}

// OwnerTokenHex returns the current session token (same-process callers
// only: the native bridge and the loopback-gated /api/status field).
func OwnerTokenHex() string {
	ownerMu.RLock()
	defer ownerMu.RUnlock()
	return ownerTokenText
}

// validOwnerToken constant-time compares a presented token. Empty never validates.
func validOwnerToken(s string) bool {
	if s == "" {
		return false
	}
	ownerMu.RLock()
	want := ownerTokenText
	ownerMu.RUnlock()
	if want == "" || len(s) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(s), []byte(want)) == 1
}

// ownerTokenFor discloses the token only to already-proven owners.
func ownerTokenFor(r *http.Request) string {
	if !hasAdmin(r) {
		return ""
	}
	return OwnerTokenHex()
}

func hasAdmin(r *http.Request) bool {
	if isLoopback(clientIP(r)) {
		return true
	}
	return validOwnerToken(r.Header.Get("X-Beam-Owner"))
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
	// The Android wrapper (capacitor://localhost) talks to the on-phone
	// server at 127.0.0.1: the hosts differ textually but the peer is still
	// loopback, and adminOnly() verifies hasAdmin() separately. Trust it.
	if trustedAppOrigin(origin) {
		return true
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
