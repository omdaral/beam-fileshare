package beamcore

import (
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultSSID    = "Beam"
	minPasswordLen = 8
	windowsGateway = "192.168.137.1"
	linuxGateway   = "10.42.0.1"
	cmdTimeout     = 10 * time.Second
)

var (
	hsMu         sync.Mutex
	hsRunning    bool
	hsSSID       string
	hsIP         string
	hsPort       = 2004
	hsSecurity   = "wpa"
	hsPassword   string
	hsOwnProfile bool
)

// recordHotspot stores the running state and builds the info map.
func recordHotspot(ssid, password string, port int, openNet bool, lang string) (string, map[string]interface{}) {
	ip := gatewayIP()
	security := "wpa"
	if openNet {
		security = "open"
	}
	hsMu.Lock()
	hsRunning = true
	hsSSID = ssid
	hsIP = ip
	hsPort = port
	hsSecurity = security
	if openNet {
		hsPassword = ""
	} else {
		hsPassword = password
	}
	hsOwnProfile = true
	hsMu.Unlock()
	info := map[string]interface{}{"ssid": ssid, "ip": ip, "port": port,
		"url": "http://" + ip + ":" + strconv.Itoa(port), "security": security}
	warn := ""
	if openNet {
		warn = tr(lang, "hs_warn_open")
	}
	return tr(lang, "hs_started", ssid, info["url"].(string), warn), info
}

// HotspotStart starts the hotspot and opens the port.
// Returns (ok, msg_ar, info).
func HotspotStart(ssid, password string, port int, openNet bool, lang string) (bool, string, map[string]interface{}) {
	return HotspotStartWithPkexec(ssid, password, port, openNet, lang, false)
}

// HotspotStartWithPkexec threads pkexec explicitly (no global env).
func HotspotStartWithPkexec(ssid, password string, port int, openNet bool, lang string, usePkexec bool) (bool, string, map[string]interface{}) {
	ssid = validateSSID(ssid)
	if ValidatePort(port) < 0 {
		return false, tr(lang, "hs_bad_port"),
			map[string]interface{}{}
	}
	if runtime.GOOS == "windows" && openNet {
		return false, tr(lang, "hs_open_windows"),
			map[string]interface{}{}
	}
	if !openNet && len(password) < minPasswordLen {
		return false, tr(lang, "hs_short_pw", len(password), minPasswordLen, minPasswordLen), map[string]interface{}{}
	}
	if !hotspotIsAdmin() {
		return false, tr(lang, "hs_need_admin"), map[string]interface{}{}
	}
	if okAP, reason := hotspotSupportsAP(lang); !okAP {
		fb := LanFallbackInfo(port)
		return false, reason + " " + tr(lang, "hs_use_lan", fb["url"].(string)),
			map[string]interface{}{}
	}
	if runtime.GOOS == "windows" {
		if ok, detail := bringUpWindows(ssid, password, lang); !ok {
			return false, detail, map[string]interface{}{}
		}
	} else {
		if !hasCommand("nmcli") {
			fb := LanFallbackInfo(port)
			return false, tr(lang, "hs_no_nmcli", fb["url"].(string)), map[string]interface{}{}
		}
		if detail := bringUpLinuxHotspot(ssid, password, openNet, lang, usePkexec); detail != "" {
			return false, detail, map[string]interface{}{}
		}
	}
	openPort(port)
	msg, info := recordHotspot(ssid, password, port, openNet, lang)
	return true, msg, info
}

// HotspotStop stops the hotspot. Returns (ok, msg_ar).
func HotspotStop(lang string) (bool, string) {
	return HotspotStopWithPkexec(lang, false)
}

// HotspotStopWithPkexec threads pkexec explicitly (no global env).
func HotspotStopWithPkexec(lang string, usePkexec bool) (bool, string) {
	hsMu.Lock()
	running := hsRunning
	hsMu.Unlock()
	if !running {
		if runtime.GOOS == "windows" {
			runCmd([]string{"netsh", "wlan", "stop", "hostednetwork"}, cmdTimeout, lang)
		} else if hasCommand("nmcli") {
			runCmdPkexec([]string{"nmcli", "connection", "down", "Hotspot"}, 8*time.Second, lang, usePkexec)
		}
		return true, tr(lang, "hs_already")
	}
	if runtime.GOOS == "windows" {
		if winOK, _ := winTetherStop(lang); !winOK {
			runCmd([]string{"netsh", "wlan", "stop", "hostednetwork"}, cmdTimeout, lang)
		}
	} else {
		if hasCommand("nmcli") {
			code, out, errStr := runCmdPkexec([]string{"nmcli", "connection", "down", "Hotspot"}, 8*time.Second, lang, usePkexec)
			if code != 0 {
				return false, tr(lang, "hs_stop_fail", cmdDetail(out, errStr, tr(lang, "nmcli_err")))
			}
			hsMu.Lock()
			own, ssid := hsOwnProfile, hsSSID
			hsMu.Unlock()
			if own {
				runCmdPkexec([]string{"nmcli", "connection", "delete", "Hotspot"}, 8*time.Second, lang, usePkexec)
			} else if ssid != "" {
				if code2, out2, _ := runCmdPkexec([]string{"nmcli", "-t", "-f", "802-11-wireless.ssid",
					"connection", "show", "Hotspot"}, 8*time.Second, lang, usePkexec); code2 == 0 &&
					strings.TrimSpace(out2) == ssid {
					runCmdPkexec([]string{"nmcli", "connection", "delete", "Hotspot"}, 8*time.Second, lang, usePkexec)
				}
			}
		}
	}
	hsMu.Lock()
	hsRunning = false
	hsSSID = ""
	hsIP = ""
	hsSecurity = "wpa"
	hsPassword = ""
	hsOwnProfile = false
	hsMu.Unlock()
	return true, tr(lang, "hs_stopped")
}
