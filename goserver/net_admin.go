package beamcore

import (
	"net/http"
	"strings"
)

// netStartParams is the validated /api/net/start body.
type netStartParams struct {
	mode     string
	ssid     string
	password string
	openNet  bool
}

func parseNetStart(data map[string]interface{}) (*netStartParams, string) {
	p := &netStartParams{
		mode:     strings.ToLower(strings.TrimSpace(jStr(data, "mode"))),
		ssid:     strings.TrimSpace(jStr(data, "ssid")),
		password: jStr(data, "password"),
		openNet:  jBool(data, "open"),
	}
	if p.mode == "" {
		p.mode = "lan"
	}
	if p.ssid == "" {
		p.ssid = defaultSSID
	}
	if p.mode != "lan" && p.mode != "hotspot" {
		return nil, "bad_mode"
	}
	return p, ""
}

// applyNetMode mirrors the requested mode into Net + session Config.
func applyNetMode(p *netStartParams) {
	NSMu.Lock()
	Net.LanMode = p.mode == "lan"
	Net.HotspotRunning = p.mode == "hotspot"
	Net.SSID = p.ssid
	Net.Security = "wpa"
	if p.mode == "hotspot" {
		if p.openNet {
			Net.Security = "open"
			Net.WifiPassword = ""
		} else {
			Net.WifiPassword = p.password
		}
	} else {
		Net.WifiPassword = ""
	}
	NSMu.Unlock()
	CfgMu.RLock()
	c := Cfg
	CfgMu.RUnlock()
	c.NetMode = p.mode
	c.Mode = p.mode
	c.WifiSSID = p.ssid
	c.SSID = p.ssid
	c.WifiOpen = p.openNet && p.mode == "hotspot"
	if p.mode == "hotspot" {
		if p.openNet {
			c.WifiPassword = ""
			c.HotspotPassword = ""
		} else {
			c.WifiPassword = p.password
			c.HotspotPassword = p.password
		}
	}
	_ = saveConfig(c)
}

// startLan stops any hotspot and reports the LAN join URL.
func startLan(w http.ResponseWriter, r *http.Request, p *netStartParams, cip string) {
	NSMu.RLock()
	wasRunning := Net.HotspotRunning
	NSMu.RUnlock()
	if wasRunning {
		hotspotStopRequest(r.RemoteAddr, reqLang(r))
	}
	applyNetMode(p)
	ips := GetLANIPs()
	lip := "127.0.0.1"
	if len(ips) > 0 {
		lip = ips[0]
	}
	url := BaseURL(lip, ServerPort)
	writeLog(cip, "net_start", "lan "+p.ssid)
	sendJSON(w, r, 200, map[string]interface{}{"ok": true, "mode": "lan",
		"msg":  tr(reqLang(r), "net_lan_ok", url),
		"info": map[string]interface{}{"ssid": p.ssid, "url": url}})
}

// startHotspot brings the hotspot up, verifies it stayed alive, and mirrors
// the live info (ssid/security/password) into Net + session Config.
func startHotspot(w http.ResponseWriter, r *http.Request, p *netStartParams, cip string) {
	ok, msg, info := hotspotStartRequest(p.ssid, p.password, ServerPort, p.openNet, r.RemoteAddr, reqLang(r))
	if !ok {
		writeLog(cip, "net_start_fail", truncateRunes(msg, 120))
		sendJSON(w, r, 400, map[string]interface{}{"error": msg, "mode": "hotspot"})
		return
	}
	// Verify the hotspot is really alive before reporting success:
	// nmcli may accept the request and still drop the network right
	// after (saved-WiFi autoconnect, driver issues). A false "ok"
	// used to flip the UI to hotspot and silently revert on next poll.
	if st := HotspotStatus(); !st["running"].(bool) {
		hotspotStopRequest(r.RemoteAddr, reqLang(r))
		revert := &netStartParams{mode: "lan", ssid: p.ssid}
		applyNetMode(revert)
		writeLog(cip, "net_start_fail", "verify")
		sendJSON(w, r, 400, map[string]interface{}{"error": tr(reqLang(r), "hs_verify_fail"), "mode": "hotspot"})
		return
	}
	NSMu.Lock()
	Net.LanMode = false
	Net.HotspotRunning = true
	if s, ok := info["ssid"].(string); ok {
		Net.SSID = s
	} else {
		Net.SSID = p.ssid
	}
	if s, ok := info["security"].(string); ok {
		Net.Security = s
	} else if p.openNet {
		Net.Security = "open"
	} else {
		Net.Security = "wpa"
	}
	if p.openNet {
		Net.WifiPassword = ""
	} else {
		Net.WifiPassword = p.password
	}
	NSMu.Unlock()
	applyNetModePassword(p)
	writeLog(cip, "net_start", "hotspot "+p.ssid)
	sendJSON(w, r, 200, map[string]interface{}{"ok": true, "mode": "hotspot",
		"msg": msg, "info": info})
}

// applyNetModePassword syncs the session Config after a verified start
// (mode/ssid/password/open flag).
func applyNetModePassword(p *netStartParams) {
	CfgMu.RLock()
	c := Cfg
	CfgMu.RUnlock()
	c.NetMode = p.mode
	c.Mode = p.mode
	c.WifiSSID = p.ssid
	c.SSID = p.ssid
	if p.openNet {
		c.WifiPassword = ""
		c.HotspotPassword = ""
	} else {
		c.WifiPassword = p.password
		c.HotspotPassword = p.password
	}
	c.WifiOpen = p.openNet
	_ = saveConfig(c)
}

func handleNetStart(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	p, errKey := parseNetStart(readJSONBody(r, jsonSmallMax))
	if errKey != "" {
		fail(w, r, 400, errKey)
		return
	}
	cip := clientIP(r)
	if p.mode == "lan" {
		startLan(w, r, p, cip)
		return
	}
	startHotspot(w, r, p, cip)
}

func handleNetStop(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	cip := clientIP(r)
	NSMu.RLock()
	wasRunning := Net.HotspotRunning
	NSMu.RUnlock()
	if wasRunning {
		lang := reqLang(r)
		ok, msg := hotspotStopRequest(r.RemoteAddr, lang)
		NSMu.Lock()
		Net.HotspotRunning = false
		Net.LanMode = true
		Net.Security = "wpa"
		Net.WifiPassword = ""
		NSMu.Unlock()
		writeLog(cip, "net_stop", truncateRunes(msg, 120))
		sendJSON(w, r, 200, map[string]interface{}{"ok": ok, "msg": tr(reqLang(r), "net_stop_ok", msg)})
		return
	}
	sendJSON(w, r, 200, map[string]interface{}{"ok": true, "msg": tr(reqLang(r), "net_already")})
}
