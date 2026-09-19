package beamcore

import (
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"
)

// WifiInfo is the best-known join credential for the current LAN Wi-Fi.
// Source is one of: "hotspot" (network we created), "auto" (OS-detected),
// "manual" (owner-typed fallback), "none" (nothing known).
type WifiInfo struct {
	SSID     string
	Password string
	Security string
	Source   string
}

var (
	wifiCacheMu sync.Mutex
	wifiCacheAt time.Time
	wifiCache   WifiInfo
)

const wifiCacheTTL = 30 * time.Second

// detectConnectedWifiOnce queries the OS for the currently-joined Wi-Fi.
// Never errors hard: unknown pieces come back empty (caller falls back to
// manual fields). Short timeouts so status polls stay fast.
func detectConnectedWifiOnce(lang string) WifiInfo {
	switch runtime.GOOS {
	case "linux":
		return detectLinuxWifi(lang)
	case "windows":
		return detectWindowsWifi(lang)
	case "darwin":
		return detectDarwinWifi(lang)
	default:
		return WifiInfo{Source: "none"}
	}
}

func detectLinuxWifi(lang string) WifiInfo {
	if !hasCommand("nmcli") {
		return WifiInfo{Source: "none"}
	}
	// Active SSID: lines like "yes:MyWifi".
	rc, out, _ := runCmd([]string{"nmcli", "-t", "-f", "active,ssid", "dev", "wifi"}, 3*time.Second, lang)
	if rc != 0 {
		return WifiInfo{Source: "none"}
	}
	ssid := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) == "yes" && strings.TrimSpace(parts[1]) != "" {
			ssid = strings.TrimSpace(parts[1])
			break
		}
	}
	if ssid == "" {
		return WifiInfo{Source: "none"}
	}
	// PSK needs root/polkit on most distros — best effort only.
	pass := ""
	if rc2, out2, _ := runCmd([]string{"nmcli", "-s", "-g", "802-11-wireless-security.psk", "connection", "show", ssid}, 3*time.Second, lang); rc2 == 0 {
		p := strings.TrimSpace(out2)
		if p != "" && p != "--" && !strings.Contains(strings.ToLower(p), "no such") && !strings.Contains(strings.ToLower(p), "not found") {
			// nmcli prints the psk on the first non-empty line.
			for _, ln := range strings.Split(p, "\n") {
				ln = strings.TrimSpace(ln)
				if ln != "" && ln != "--" {
					pass = ln
					break
				}
			}
		}
	}
	sec := "wpa"
	if pass == "" {
		sec = "wpa"
	}
	return WifiInfo{SSID: ssid, Password: pass, Security: sec, Source: "auto"}
}

func parseNetshValue(out, key string) string {
	key = strings.ToLower(key)
	for _, line := range strings.Split(out, "\n") {
		low := strings.ToLower(line)
		if !strings.Contains(low, key) || !strings.Contains(line, ":") {
			continue
		}
		ix := strings.Index(line, ":")
		v := strings.TrimSpace(line[ix+1:])
		if v != "" {
			return v
		}
	}
	return ""
}

func detectWindowsWifi(lang string) WifiInfo {
	if !hasCommand("netsh") {
		return WifiInfo{Source: "none"}
	}
	rc, out, _ := runCmd([]string{"netsh", "wlan", "show", "interfaces"}, 4*time.Second, lang)
	if rc != 0 {
		return WifiInfo{Source: "none"}
	}
	// Avoid the BSSID line: match " SSID " with surrounding spaces.
	ssid := ""
	for _, line := range strings.Split(out, "\n") {
		low := strings.ToLower(line)
		if strings.Contains(low, "bssid") || !strings.Contains(low, "ssid") || !strings.Contains(line, ":") {
			continue
		}
		ix := strings.Index(line, ":")
		v := strings.TrimSpace(line[ix+1:])
		if v != "" && !strings.EqualFold(v, "disconnected") {
			ssid = v
			break
		}
	}
	if ssid == "" {
		return WifiInfo{Source: "none"}
	}
	pass := ""
	if rc2, out2, _ := runCmd([]string{"netsh", "wlan", "show", "profile", "name=" + ssid, "key=clear"}, 4*time.Second, lang); rc2 == 0 {
		if v := parseNetshValue(out2, "key content"); v != "" {
			pass = v
		}
	}
	return WifiInfo{SSID: ssid, Password: pass, Security: "wpa", Source: "auto"}
}

func detectDarwinWifi(lang string) WifiInfo {
	if !hasCommand("networksetup") {
		return WifiInfo{Source: "none"}
	}
	rc, out, _ := runCmd([]string{"networksetup", "-getairportnetwork", "en0"}, 3*time.Second, lang)
	if rc != 0 {
		return WifiInfo{Source: "none"}
	}
	// "Current Wi-Fi Network: MyWifi"
	ix := strings.Index(out, ":")
	if ix < 0 {
		return WifiInfo{Source: "none"}
	}
	ssid := strings.TrimSpace(out[ix+1:])
	if ssid == "" || strings.Contains(strings.ToLower(ssid), "not associated") {
		return WifiInfo{Source: "none"}
	}
	// Keychain password needs user approval — leave to manual entry.
	return WifiInfo{SSID: ssid, Security: "wpa", Source: "auto"}
}

// cachedWifi returns the auto-detected LAN Wi-Fi (30s cache, "none" when unknown).
func cachedWifi(lang string) WifiInfo {
	wifiCacheMu.Lock()
	defer wifiCacheMu.Unlock()
	if time.Since(wifiCacheAt) < wifiCacheTTL {
		return wifiCache
	}
	w := detectConnectedWifiOnce(lang)
	wifiCache = w
	wifiCacheAt = time.Now()
	return w
}

// effectiveWifi picks what the QR + status show:
// hotspot (when we run one) > OS auto-detect > owner manual > Net fallback.
func effectiveWifi(lang string) WifiInfo {
	NSMu.RLock()
	hsRunning := Net.HotspotRunning
	lanMode := Net.LanMode
	netSSID := Net.SSID
	netPass := Net.WifiPassword
	netSec := Net.Security
	NSMu.RUnlock()
	if hsRunning || !lanMode {
		sec := netSec
		if sec == "" {
			sec = "wpa"
		}
		if netPass == "" && sec != "open" {
			sec = "wpa"
		}
		return WifiInfo{SSID: netSSID, Password: netPass, Security: sec, Source: "hotspot"}
	}
	if auto := cachedWifi(lang); auto.SSID != "" {
		// Auto SSID is authoritative; password falls back to manual when the
		// OS hides the PSK (needs root/admin).
		CfgMu.RLock()
		manualPass := Cfg.WifiPassword
		manualSec := Cfg.WifiSecurity
		CfgMu.RUnlock()
		pass := auto.Password
		if pass == "" {
			pass = manualPass
		}
		sec := auto.Security
		if sec == "" {
			sec = manualSec
		}
		if sec == "" {
			sec = "wpa"
		}
		return WifiInfo{SSID: auto.SSID, Password: pass, Security: sec, Source: "auto"}
	}
	CfgMu.RLock()
	mSSID, mPass, mSec := Cfg.WifiSSID, Cfg.WifiPassword, Cfg.WifiSecurity
	CfgMu.RUnlock()
	if mSSID == "" {
		mSSID = netSSID
	}
	if mSec == "" {
		mSec = "wpa"
	}
	if mSSID != "" && (mPass != "" || mSSID != "Beam") {
		return WifiInfo{SSID: mSSID, Password: mPass, Security: mSec, Source: "manual"}
	}
	if netSSID == "" {
		netSSID = "Beam"
	}
	return WifiInfo{SSID: netSSID, Password: netPass, Security: "wpa", Source: "none"}
}

// handleWifiDetect is owner-only: force a fresh OS probe (bypass cache)
// so the "detect" button gives immediate feedback.
func handleWifiDetect(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	lang := reqLang(r)
	wifiCacheMu.Lock()
	wifiCache = detectConnectedWifiOnce(lang)
	wifiCacheAt = time.Now()
	info := wifiCache
	wifiCacheMu.Unlock()
	if info.SSID == "" {
		sendJSON(w, r, 200, map[string]interface{}{"ok": false, "source": "none"})
		return
	}
	sendJSON(w, r, 200, map[string]interface{}{
		"ok": true, "ssid": info.SSID, "password": info.Password,
		"security": info.Security, "source": info.Source,
	})
}
