package beamcore

import (
	"net/http"
	"strconv"
)

func handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	ips := GetLANIPs()
	gw := gatewayIP()
	ip := gw
	if ip == "" && len(ips) > 0 {
		ip = ips[0]
	}
	NSMu.RLock()
	ssid, lanMode, hsRunning, security, wifiPass := Net.SSID, Net.LanMode,
		Net.HotspotRunning, Net.Security, Net.WifiPassword
	NSMu.RUnlock()
	var clients interface{} = "—"
	if hsRunning {
		clients = HotspotStatus()["clients"]
	}
	if ssid == "" {
		ssid = "Beam"
	}
	wp := wifiPass // wifi password is visible to all: the page is the sharing channel.
	CfgMu.RLock()
	defaultLang := NormalizeLang(Cfg.DefaultLang)
	CfgMu.RUnlock()
	sendJSON(w, r, 200, map[string]interface{}{
		"running": true, "ssid": ssid, "ip": ip, "port": ServerPort,
		"url":     "http://" + ip + ":" + strconv.Itoa(ServerPort),
		"clients": clients, "lan_mode": lanMode, "security": security,
		"wifi_password": wp, "version": AppVersion, "is_admin": hasAdmin(r),
		"default_lang": defaultLang, "shared_dir": SharedDir,
		"idle_seconds": idleSecondsLeft(),
		"mdns_url":     "http://beam.local:" + strconv.Itoa(ServerPort),
		"plain_url":    "http://beam:" + strconv.Itoa(ServerPort),
		"limits":       LimitsSnapshot(),
	})
}
func handleNetStatus(w http.ResponseWriter, r *http.Request) {
	ips := GetLANIPs()
	gw := gatewayIP()
	ip := gw
	if ip == "" && len(ips) > 0 {
		ip = ips[0]
	}
	admin := hasAdmin(r)
	avail, capable := netCapabilities()
	clients := hotspotClients()
	NSMu.RLock()
	ssid, lanMode, hsRunning, security, wifiPass := Net.SSID, Net.LanMode,
		Net.HotspotRunning, Net.Security, Net.WifiPassword
	NSMu.RUnlock()
	CfgMu.RLock()
	wifiOpen := Cfg.WifiOpen
	defaultLang := NormalizeLang(Cfg.DefaultLang)
	CfgMu.RUnlock()
	var clientsOut interface{} = len(clients)
	if admin {
		clientsOut = clients
	}
	wp := wifiPass // visible to all for the wifi QR (page is the sharing channel).
	if ssid == "" {
		ssid = "Beam"
	}
	sendJSON(w, r, 200, map[string]interface{}{
		"running": true, "ssid": ssid, "ip": ip, "port": ServerPort,
		"url":     "http://" + ip + ":" + strconv.Itoa(ServerPort),
		"clients": clientsOut, "clients_count": len(clients),
		"lan_mode": lanMode, "hotspot_running": hsRunning,
		"security": security, "wifi_password": wp, "wifi_open": wifiOpen,
		"hotspot_available": avail, "admin_capable": capable,
		"pkexec_available": pkexecAvailable(), "is_admin": admin,
		"version": AppVersion, "default_lang": defaultLang,
		"shared_dir": SharedDir, "idle_seconds": idleSecondsLeft(),
		"mdns_url":  "http://beam.local:" + strconv.Itoa(ServerPort),
		"plain_url": "http://beam:" + strconv.Itoa(ServerPort),
	})
}

// handleNetClients is a fast live path: hotspot state + arp only,
// without the heavier WinRT/nmcli live verification of full status.
func handleNetClients(w http.ResponseWriter, r *http.Request) {
	admin := hasAdmin(r)
	clients := hotspotClients()
	var out interface{} = len(clients)
	if admin {
		out = clients
	}
	sendJSON(w, r, 200, map[string]interface{}{
		"clients": out, "clients_count": len(clients), "is_admin": admin,
	})
}
