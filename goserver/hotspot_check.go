package beamcore

import (
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func hotspotIsAdmin() bool {
	if runtime.GOOS == "windows" {
		code, _, _ := runCmd([]string{"net", "session"}, cmdTimeout, "ar")
		return code == 0
	}
	// os.Geteuid does not compile on Windows, so use `id -u` instead.
	if out, err := exec.Command("id", "-u").Output(); err == nil &&
		strings.TrimSpace(string(out)) == "0" {
		return true
	}
	if !hasCommand("nmcli") {
		return false
	}
	code, out, _ := runCmd([]string{"nmcli", "general", "permissions"}, 5*time.Second, "ar")
	if code != 0 || out == "" {
		return false
	}
	m := regexp.MustCompile(`(?i)wifi\.share\.(?:protected|open)\s*:\s*(yes|no|auth)`).FindStringSubmatch(out)
	if m != nil {
		return strings.ToLower(m[1]) == "yes"
	}
	return true
}
func hotspotSupportsAP(lang string) (bool, string) {
	if runtime.GOOS == "windows" {
		if !hasCommand("netsh") {
			return false, tr(lang, "hs_no_netsh")
		}
		code, out, errStr := runCmd([]string{"netsh", "wlan", "show", "drivers"}, cmdTimeout, lang)
		if code != 0 {
			detail := strings.TrimSpace(errStr)
			if detail == "" {
				detail = strings.TrimSpace(out)
			}
			if len(detail) > 120 {
				detail = detail[:120]
			}
			return false, tr(lang, "hs_no_drivers", detail)
		}
		if m := regexp.MustCompile(`(?i)hosted\s*network\s*supported\s*:\s*(yes|no)`).FindStringSubmatch(out); m != nil {
			if strings.ToLower(m[1]) == "yes" {
				return true, tr(lang, "hs_ap_ok")
			}
			return false, tr(lang, "hs_no_hosted")
		}
		if (strings.Contains(out, "نعم") || strings.Contains(out, "Yes")) &&
			regexp.MustCompile(`(?i)Hosted|مستضافة|استضافة`).MatchString(out) {
			return true, tr(lang, "hs_ap_ok")
		}
		return false, tr(lang, "hs_unsure")
	}
	if hasCommand("iw") {
		code, out, errStr := runCmd([]string{"iw", "list"}, cmdTimeout, lang)
		if code != 0 {
			detail := strings.TrimSpace(errStr)
			if detail == "" {
				detail = strings.TrimSpace(out)
			}
			if len(detail) > 120 {
				detail = detail[:120]
			}
			return false, tr(lang, "hs_no_iw", detail)
		}
		if regexp.MustCompile(`\*\s*AP\b`).MatchString(out) {
			return true, tr(lang, "hs_ap_linux_ok")
		}
		return false, tr(lang, "hs_no_ap")
	}
	if hasCommand("nmcli") {
		return true, tr(lang, "hs_assumed")
	}
	return false, tr(lang, "hs_no_tools")
}
func openPort(port int) {
	if runtime.GOOS == "windows" {
		rule := "Beam-" + strconv.Itoa(port)
		runCmd([]string{"netsh", "advfirewall", "firewall", "add", "rule",
			"name=" + rule, "dir=in", "action=allow",
			"protocol=TCP", "localport=" + strconv.Itoa(port)}, cmdTimeout, "ar")
		return
	}
	if hasCommand("firewall-cmd") {
		runCmd([]string{"firewall-cmd", "--add-port=" + strconv.Itoa(port) + "/tcp"}, 8*time.Second, "ar")
		return
	}
	if hasCommand("ufw") {
		runCmd([]string{"ufw", "allow", strconv.Itoa(port) + "/tcp"}, 8*time.Second, "ar")
	}
}
func LanFallbackInfo(port int) map[string]interface{} {
	if ValidatePort(port) < 0 {
		port = defaultPort
	}
	ip := currentLANIP()
	url := "http://" + ip + ":" + strconv.Itoa(port)
	return map[string]interface{}{
		"mode": "lan", "ssid": nil, "ip": ip, "port": port, "url": url,
		"message_ar": "الهوتسبوت غير متاح على هذا الجهاز. الحل: اشتغل بوضع LAN — " +
			"اتصل بواي فاي ثم افتح الرابط من الأجهزة: " + url,
	}
}
