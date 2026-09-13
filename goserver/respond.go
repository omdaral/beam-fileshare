package beamcore

import (
	"net/http"
	"strings"
)

// Short response helpers: one line per error (replaces 3-line sendJSON
// boilerplate repeated 70+ times). Behavior identical to the expanded form.

// fail writes {"error": tr(lang,key,args...)} with the given status.
func fail(w http.ResponseWriter, r *http.Request, code int, key string, args ...interface{}) {
	sendJSON(w, r, code, map[string]interface{}{"error": tr(reqLang(r), key, args...)})
}

// ok writes a 200 JSON object.
func ok(w http.ResponseWriter, r *http.Request, obj interface{}) {
	sendJSON(w, r, 200, obj)
}

// mustRel validates a share-relative path (safeRelPath + no dotfiles).
// Returns (rel, true) when usable, ("", false) otherwise.
func mustRel(raw string) (string, bool) {
	rel := safeRelPath(raw)
	if rel == "" || strings.HasPrefix(rel, ".") {
		return "", false
	}
	return rel, true
}

// mustSession loads a session meta or writes 404 and returns nil.
func mustSession(w http.ResponseWriter, r *http.Request, uid string) *sessionMeta {
	uid = strings.ToLower(uid)
	if m := loadMeta(uid); m != nil {
		return m
	}
	fail(w, r, 404, "up_session_gone")
	return nil
}
