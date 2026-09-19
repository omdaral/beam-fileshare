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
	lanMode, hsRunning := Net.LanMode, Net.HotspotRunning
	NSMu.RUnlock()
	// QR + card show the real join credentials: hotspot we run, else the
	// OS-detected Wi-Fi (manual fallback when the PSK needs privileges).
	eff := effectiveWifi(reqLang(r))
	ssid := eff.SSID
	security := eff.Security
	if ssid == "" {
		ssid = "Beam"
	}
	if security == "" {
		security = "wpa"
	}
	var clients interface{} = "—"
	if hsRunning && !IsPhoneBuild {
		// On the phone the LOHS client list needs privileged APIs and
		// HotspotStatus would redo gateway/exec probes per poll for
		// nothing — skip it (keeps /api/status fast so /health never starves).
		clients = HotspotStatus()["clients"]
	}
	if ssid == "" {
		ssid = "Beam"
	}
	wp := eff.Password // wifi password is visible to all: the page is the sharing channel.
	CfgMu.RLock()
	defaultLang := NormalizeLang(Cfg.DefaultLang)
	CfgMu.RUnlock()
	resp := map[string]interface{}{
		"running": true, "ssid": ssid, "ip": ip, "port": ServerPort,
		"url":     BaseURL(ip, ServerPort),
		"clients": clients, "lan_mode": lanMode, "security": security,
		"wifi_password": wp, "wifi_ssid": ssid, "wifi_source": eff.Source,
		"version": AppVersion, "is_admin": hasAdmin(r),
		"is_phone":     IsPhoneBuild,
		"scheme":       TLSScheme(),
		"tls":          TLSEnabled,
		"tls_fingerprint": TLSFingerprint,
		"default_lang": defaultLang, "shared_dir": tempBase(),
		"idle_seconds": idleSecondsLeft(),
		"mdns_url":     TLSScheme() + "://beam.local:" + strconv.Itoa(ServerPort),
		"plain_url":    TLSScheme() + "://beam:" + strconv.Itoa(ServerPort),
		"limits":       LimitsSnapshot(),
	}
	// Owner token disclosed only to proven owners (never to guests).
	if tok := ownerTokenFor(r); tok != "" {
		resp["owner_token"] = tok
	}
	sendJSON(w, r, 200, resp)
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
	lanMode, hsRunning := Net.LanMode, Net.HotspotRunning
	NSMu.RUnlock()
	eff := effectiveWifi(reqLang(r))
	ssid := eff.SSID
	security := eff.Security
	if ssid == "" {
		ssid = "Beam"
	}
	if security == "" {
		security = "wpa"
	}
	CfgMu.RLock()
	wifiOpen := Cfg.WifiOpen
	defaultLang := NormalizeLang(Cfg.DefaultLang)
	CfgMu.RUnlock()
	var clientsOut interface{} = len(clients)
	if admin {
		clientsOut = clients
	}
	wp := eff.Password // visible to all for the wifi QR (page is the sharing channel).
	if ssid == "" {
		ssid = "Beam"
	}
	sendJSON(w, r, 200, map[string]interface{}{
		"running": true, "ssid": ssid, "ip": ip, "port": ServerPort,
		"url":     BaseURL(ip, ServerPort),
		"clients": clientsOut, "clients_count": len(clients),
		"lan_mode": lanMode, "hotspot_running": hsRunning,
		"security": security, "wifi_password": wp, "wifi_ssid": ssid,
		"wifi_source": eff.Source, "wifi_open": wifiOpen,
		"hotspot_available": avail, "admin_capable": capable,
		"pkexec_available": pkexecAvailable(), "is_admin": admin,
		"is_phone": IsPhoneBuild,
		"scheme":   TLSScheme(), "tls": TLSEnabled,
		"tls_fingerprint": TLSFingerprint,
		"version":  AppVersion, "default_lang": defaultLang,
		"shared_dir": tempBase(), "idle_seconds": idleSecondsLeft(),
		"mdns_url":  TLSScheme() + "://beam.local:" + strconv.Itoa(ServerPort),
		"plain_url": TLSScheme() + "://beam:" + strconv.Itoa(ServerPort),
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
