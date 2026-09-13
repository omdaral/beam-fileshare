package beamcore

import (
	"strings"
)

// cmdDetail picks stderr, else stdout, trimmed to 200 runes, else fallback.
func cmdDetail(out, errStr, fallback string) string {
	detail := strings.TrimSpace(errStr)
	if detail == "" {
		detail = strings.TrimSpace(out)
	}
	if len(detail) > 200 {
		detail = detail[:200]
	}
	if detail == "" {
		detail = fallback
	}
	return detail
}

// bringUpWindows tries WinRT tethering, falling back to netsh hostednetwork.
func bringUpWindows(ssid, password, lang string) (bool, string) {
	winOK, winDetail := winTetherStart(ssid, password, lang)
	if winOK {
		return true, ""
	}
	code, out, errStr := runCmd([]string{"netsh", "wlan", "set", "hostednetwork",
		"mode=allow", "ssid=" + ssid, "key=" + password}, cmdTimeout, lang)
	if code == 0 {
		code, out, errStr = runCmd([]string{"netsh", "wlan", "start", "hostednetwork"}, cmdTimeout, lang)
	}
	if code != 0 {
		if winDetail == "" {
			winDetail = tr(lang, "unknown_err")
		}
		return false, tr(lang, "hs_win_fail", winDetail,
			cmdDetail(out, errStr, tr(lang, "netsh_err")))
	}
	return true, ""
}

// bringUpLinuxHotspot creates the nmcli Hotspot profile (and converts it to
// an open network when requested). Returns errKey detail ("" when ok).
func bringUpLinuxHotspot(ssid, password string, openNet bool, lang string, usePkexec bool) string {
	pw := password
	if pw == "" {
		pw = "OpenNet00"
	}
	code, out, errStr := runCmdPkexec([]string{"nmcli", "device", "wifi", "hotspot",
		"con-name", "Hotspot", "ssid", ssid, "password", pw}, cmdTimeout, lang, usePkexec)
	if code != 0 {
		return tr(lang, "hs_nmcli_fail", cmdDetail(out, errStr, tr(lang, "nmcli_err")))
	}
	if !openNet {
		return ""
	}
	runCmdPkexec([]string{"nmcli", "connection", "modify", "Hotspot", "wifi-sec.key-mgmt", "none"}, cmdTimeout, lang, usePkexec)
	runCmdPkexec([]string{"nmcli", "connection", "modify", "Hotspot", "-wifi-sec.psk"}, cmdTimeout, lang, usePkexec)
	runCmdPkexec([]string{"nmcli", "connection", "down", "Hotspot"}, cmdTimeout, lang, usePkexec)
	code, out, errStr = runCmdPkexec([]string{"nmcli", "connection", "up", "Hotspot"}, cmdTimeout, lang, usePkexec)
	if code != 0 {
		runCmdPkexec([]string{"nmcli", "connection", "delete", "Hotspot"}, cmdTimeout, lang, usePkexec)
		return tr(lang, "hs_open_fail", cmdDetail(out, errStr, tr(lang, "nmcli_err")))
	}
	return ""
}
