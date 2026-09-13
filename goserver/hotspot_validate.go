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
