package beamcore

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func clampLogTail(raw string) int {
	tail := 200
	if t, err := strconv.Atoi(raw); err == nil {
		tail = t
	}
	if tail < 1 {
		tail = 1
	}
	if tail > 2000 {
		tail = 2000
	}
	return tail
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	if !hasAdmin(r) {
		sendJSON(w, r, 403, map[string]interface{}{"error": tr(reqLang(r), "owner_page")})
		return
	}
	lines := readLog(clampLogTail(r.URL.Query().Get("tail")))
	sendJSON(w, r, 200, map[string]interface{}{"lines": lines})
}
func handleConfigGet(w http.ResponseWriter, r *http.Request) {
	if !hasAdmin(r) {
		sendJSON(w, r, 403, map[string]interface{}{"error": tr(reqLang(r), "owner_page")})
		return
	}
	CfgMu.RLock()
	defer CfgMu.RUnlock()
	sendJSON(w, r, 200, map[string]interface{}{"config": Cfg})
}
func handleLogsClear(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	// Full wipe of the in-memory ring, then an audit line so a stolen
	// token can't silently erase forensics.
	clearLog()
	writeLog(clientIP(r), "logs_cleared", "by owner")
	sendJSON(w, r, 200, map[string]interface{}{"ok": true, "msg": tr(reqLang(r), "logs_cleared")})
}

// asInt accepts every JSON number shape our clients send.
func asInt(raw interface{}) (int, bool) {
	switch n := raw.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case int32:
		return int(n), true
	case json.Number:
		if v, err := n.Int64(); err == nil {
			return int(v), true
		}
	}
	return 0, false
}

func handleConfigPost(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	data := readJSONBody(r, jsonSmallMax)
	CfgMu.RLock()
	c := Cfg
	CfgMu.RUnlock()
	needRestart := false
	if raw, ok := data["port"]; ok {
		p, valid := asInt(raw)
		if !valid {
			fail(w, r, 400, "port_not_number")
			return
		}
		if p < 1 || p > 65535 {
			fail(w, r, 400, "port_range")
			return
		}
		if p != ServerPort {
			needRestart = true
		}
		c.Port = p
	}
	if raw, ok := data["max_file_mb"]; ok {
		mf, valid := asInt(raw)
		if !valid {
			fail(w, r, 400, "max_not_number")
			return
		}
		if mf < 0 || mf > 102400 {
			fail(w, r, 400, "max_range")
			return
		}
		c.MaxFileMB = mf
	}
	if raw, ok := data["ssid"]; ok {
		if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
			c.WifiSSID = strings.TrimSpace(s)
			c.SSID = c.WifiSSID
		}
	}
	// Auto-save support: hotspot password + open-network flag persist to
	// the session (length validated here too, not only at start time).
	if raw, ok := data["password"]; ok {
		if s, ok := raw.(string); ok {
			if s != "" {
				if errKey := validateHotspotPassword(s, false); errKey != "" {
					fail(w, r, 400, errKey)
					return
				}
				if !validHotspotCreds(c.WifiSSID, s) {
					fail(w, r, 400, "bad_hotspot_cred")
					return
				}
			}
			c.WifiPassword = s
			c.HotspotPassword = s
		}
	}
	if raw, ok := data["open"]; ok {
		if b, ok := raw.(bool); ok {
			c.WifiOpen = b
		}
	}
	// Manual Wi-Fi join credentials (hybrid with auto-detect): shown in the
	// QR when the OS hides the PSK (needs root/admin). Owner-only.
	if raw, ok := data["wifi_ssid"]; ok {
		if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
			c.WifiSSID = strings.TrimSpace(s)
			c.SSID = c.WifiSSID
		}
	}
	if raw, ok := data["wifi_password"]; ok {
		if s, ok := raw.(string); ok {
			if s != "" && strings.ContainsAny(s, "\r\n\x00") {
				fail(w, r, 400, "pass_bad_chars")
				return
			}
			c.WifiPassword = s
			c.HotspotPassword = s
		}
	}
	if raw, ok := data["wifi_security"]; ok {
		if s, ok := raw.(string); ok {
			s = strings.ToLower(strings.TrimSpace(s))
			if s == "open" || s == "wpa" || s == "nopass" {
				if s == "nopass" {
					s = "open"
				}
				c.WifiSecurity = strings.ToUpper(s)
				if s == "open" {
					c.WifiSecurity = "open"
				}
			}
		}
	}
	if raw, ok := data["default_lang"]; ok {
		s, ok := raw.(string)
		if !ok {
			fail(w, r, 400, "lang_bad")
			return
		}
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "ar" && s != "en" {
			fail(w, r, 400, "lang_bad")
			return
		}
		c.DefaultLang = s
	}
	// temp_dir is session-only like every other setting: changing it MOVES
	// nothing — bytes already in the old temp stay there, and all new
	// sessions/retained guest bytes go to the new dir from now on.
	if raw, ok := data["temp_dir"]; ok {
		s, ok := raw.(string)
		if !ok || strings.TrimSpace(s) == "" {
			sendJSON(w, r, 400, map[string]interface{}{
				"error": tr(reqLang(r), "temp_bad_path"), "code": "temp_bad_path"})
			return
		}
		s = filepath.Clean(strings.TrimSpace(s))
		if !filepath.IsAbs(s) {
			sendJSON(w, r, 400, map[string]interface{}{
				"error": tr(reqLang(r), "temp_need_abs"), "code": "temp_need_abs"})
			return
		}
		// Harden temp_dir: reject filesystem roots and top-level system
		// dirs even if shareBlocked misses a spelling.
		low := strings.ToLower(s)
		for _, blocked := range []string{"/", "/home", "/root", "/tmp", "/var", "/etc", "/usr", "/bin", "/sbin", "/boot", "/dev", "/proc", "/sys", "/run"} {
			if low == blocked {
				sendJSON(w, r, 403, map[string]interface{}{
					"error": tr(reqLang(r), "share_blocked"), "code": "share_blocked"})
				return
			}
		}
		if shareBlocked(s) {
			sendJSON(w, r, 403, map[string]interface{}{
				"error": tr(reqLang(r), "share_blocked"), "code": "share_blocked"})
			return
		}
		if err := os.MkdirAll(s, 0755); err != nil {
			sendJSON(w, r, 500, map[string]interface{}{
				"error": tr(reqLang(r), "temp_bad_path"), "code": "temp_bad_path"})
			return
		}
		c.TempDir = s
	}
	if err := saveConfig(c); err != nil {
		fail(w, r, 500, "config_save_fail")
		return
	}
	// Apply a temp_dir change to the live globals (session-only, like all
	// settings). Nothing is MOVED: old temp bytes stay where they are.
	if _, ok := data["temp_dir"]; ok {
		CfgMu.RLock()
		nd := Cfg.TempDir
		CfgMu.RUnlock()
		if nd != "" {
			TempDir = nd
			SharedDir = nd // deprecated alias stays pointed at temp
			_ = os.MkdirAll(nd, 0755)
		}
	}
	writeLog(clientIP(r), "config_save", "need_restart="+strconv.FormatBool(needRestart))
	if _, ok := data["temp_dir"]; ok {
		writeLog(clientIP(r), "config_temp_dir", Cfg.TempDir)
	}
	msg := tr(reqLang(r), "config_saved")
	if needRestart {
		msg += tr(reqLang(r), "config_restart_note")
	}
	CfgMu.RLock()
	defer CfgMu.RUnlock()
	sendJSON(w, r, 200, map[string]interface{}{"ok": true, "msg": msg,
		"need_restart": needRestart, "config": Cfg})
}
func handleServerStop(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	if !isLoopback(clientIP(r)) {
		sendJSON(w, r, 403, map[string]interface{}{"error": tr(reqLang(r), "server_stop_local")})
		return
	}
	writeLog(clientIP(r), "server_stop", "ok")
	sendJSON(w, r, 200, map[string]interface{}{"ok": true, "msg": tr(reqLang(r), "server_stopped")})
	go func() {
		time.Sleep(500 * time.Millisecond)
		Shutdown()
	}()
}
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
