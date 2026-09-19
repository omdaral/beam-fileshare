package beamcore

import (
	"os"
	"path/filepath"
	"strings"
)

// Config holds the server settings for this session only.
//
// Beam intentionally keeps ZERO side files: there is no config.json on
// disk. The server boots from these built-in defaults (fixed port 2004),
// the owner can tweak them from the web UI (applied in memory until the
// server stops), and the owner's browser keeps a copy in localStorage so
// the fields are prefilled and re-applied on the next run.
type Config struct {
	Port            int    `json:"port"`
	SharedDir       string `json:"shared_dir"`
	TempDir         string `json:"temp_dir"`
	MaxFileMB       int    `json:"max_file_mb"`
	WifiSSID        string `json:"wifi_ssid"`
	WifiPassword    string `json:"wifi_password"`
	WifiSecurity    string `json:"wifi_security"`
	NetMode         string `json:"net_mode"`
	WifiOpen        bool   `json:"wifi_open"`
	SSID            string `json:"ssid"`
	HotspotPassword string `json:"hotspot_password"`
	Mode            string `json:"mode"`
	DefaultLang     string `json:"default_lang"`
}

func defaultConfig() Config {
	return Config{
		Port:            defaultPort,
		SharedDir:       "",
		MaxFileMB:       0,
		WifiSSID:        "Beam",
		WifiPassword:    "",
		WifiSecurity:    "WPA",
		NetMode:         "lan",
		WifiOpen:        false,
		SSID:            "Beam",
		HotspotPassword: "",
		Mode:            "lan",
		DefaultLang:     "ar",
	}
}

// applyDefaultsAndMirror fills missing values and syncs mirror key pairs.
// MaxFileMB == 0 means unlimited (no per-file cap). Negative values are
// normalized to 0 so a typo can never impose a hidden limit.
func (c *Config) applyDefaultsAndMirror() {
	d := defaultConfig()
	if c.Port == 0 {
		c.Port = d.Port
	}
	if c.MaxFileMB < 0 {
		c.MaxFileMB = 0
	}
	if c.WifiSecurity == "" {
		c.WifiSecurity = d.WifiSecurity
	}
	if c.WifiSSID == "" {
		c.WifiSSID = c.SSID
	}
	if c.SSID == "" {
		c.SSID = c.WifiSSID
	}
	if c.SSID == "" {
		c.SSID = d.SSID
		c.WifiSSID = d.WifiSSID
	}
	if c.WifiPassword == "" {
		c.WifiPassword = c.HotspotPassword
	}
	if c.HotspotPassword == "" {
		c.HotspotPassword = c.WifiPassword
	}
	if c.NetMode == "" {
		c.NetMode = c.Mode
	}
	if c.Mode == "" {
		c.Mode = c.NetMode
	}
	if c.NetMode == "" {
		c.NetMode = d.NetMode
		c.Mode = d.Mode
	}
	c.DefaultLang = NormalizeLang(c.DefaultLang)
}

// NormalizeLang keeps the guest-default UI language to ar/en (default ar).
func NormalizeLang(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "en":
		return "en"
	default:
		return "ar"
	}
}

// LoadConfig returns the built-in defaults (no file is ever read).
func LoadConfig() Config {
	return defaultConfig()
}

// saveConfig applies settings to the running session only (no file).
// It mirrors the old file-based behavior for callers and tests.
func saveConfig(c Config) error {
	c.applyDefaultsAndMirror()
	CfgMu.Lock()
	Cfg = c
	CfgMu.Unlock()
	NSMu.Lock()
	Net.SSID = c.WifiSSID
	if Net.SSID == "" {
		Net.SSID = c.SSID
	}
	NSMu.Unlock()
	return nil
}

// cfgSnapshot returns a copy of the live session config (for tests/UI).
func cfgSnapshot() Config {
	CfgMu.RLock()
	defer CfgMu.RUnlock()
	return Cfg
}

// maxBytes returns the effective per-file limit in bytes.
// MaxFileMB <= 0 means unlimited: returns a huge sentinel (1<<62) so all
// size checks pass and only disk space / OS limits apply.
func maxBytes() int64 {
	CfgMu.RLock()
	defer CfgMu.RUnlock()
	if Cfg.MaxFileMB <= 0 {
		return int64(1) << 62
	}
	return int64(Cfg.MaxFileMB) * 1024 * 1024
}

// LoadVersion reads VERSION next to the binary.
// Falls back to the compiled-in AppVersion (ldflags -X), so installed
// packages (/usr/libexec, /app/bin) report correctly without a VERSION file.
func LoadVersion(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "VERSION"))
	if err != nil {
		if AppVersion != "" {
			return AppVersion
		}
		return "1.6.0"
	}
	v := string(data)
	// strip whitespace like Python .strip()
	start := 0
	for start < len(v) && (v[start] == ' ' || v[start] == '\t' || v[start] == '\n' || v[start] == '\r') {
		start++
	}
	end := len(v)
	for end > start && (v[end-1] == ' ' || v[end-1] == '\t' || v[end-1] == '\n' || v[end-1] == '\r') {
		end--
	}
	v = v[start:end]
	if v == "" {
		if AppVersion != "" {
			return AppVersion
		}
		return "1.6.0"
	}
	return v
}
