package beamcore

import (
	"strings"
)

func validateSSID(ssid string) string {
	ssid = strings.TrimSpace(ssid)
	// SSID max 32 bytes + reject control chars/CR/LF (injection safe).
	ssid = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, ssid)
	ssid = strings.TrimSpace(ssid)
	if ssid == "" {
		ssid = defaultSSID
	}
	if len([]byte(ssid)) > 32 {
		// Truncate on rune boundary to 32 bytes.
		b := []byte(ssid)
		for len(b) > 32 {
			r := []rune(string(b))
			r = r[:len(r)-1]
			b = []byte(string(r))
		}
		ssid = string(b)
	}
	if ssid == "" {
		ssid = defaultSSID
	}
	return ssid
}
func ValidatePort(port int) int {
	if port >= 1 && port <= 65535 {
		return port
	}
	return -1
}

// validateHotspotPassword enforces 8..63 chars with no CR/LF (injection
// safe). Empty is allowed only for open networks.
func validateHotspotPassword(pw string, openNet bool) string {
	if openNet {
		return ""
	}
	if len([]rune(pw)) < 8 || len(pw) > 63 {
		return "pass_short"
	}
	if strings.ContainsAny(pw, "\r\n\x00") {
		return "pass_bad_chars"
	}
	return ""
}

// validHotspotCreds rejects newline/leading-dash SSID/password tricks
// that could become option-injection in netsh/nmcli arg building.
func validHotspotCreds(ssid, password string) bool {
	if strings.ContainsAny(ssid, "\r\n\x00") || strings.ContainsAny(password, "\r\n\x00") {
		return false
	}
	if strings.HasPrefix(strings.TrimSpace(ssid), "-") {
		return false
	}
	return true
}
