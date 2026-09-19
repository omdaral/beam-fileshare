package beamcore

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// setupTestEnv points all globals at temp dirs (mirrors test_*.py setup).
// Beam keeps no side files: settings + log + registry live in memory, the
// temp dir (uploads staging, retained guest bytes) lives under tmp.
func setupTestEnv(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	TempDir = filepath.Join(tmp, "Beam-Temp")
	if err := os.MkdirAll(TempDir, 0755); err != nil {
		t.Fatal(err)
	}
	SharedDir = TempDir // deprecated alias stays pointed at temp
	beamHomeOverride = tmp // TempDefaultDir() resolves under tmp in tests
	Cfg = defaultConfig()
	Cfg.TempDir = TempDir
	Cfg.Port = 18793
	Cfg.WifiPassword = "Share12345"
	Cfg.HotspotPassword = "Share12345"
	Cfg.NetMode = "lan"
	Cfg.Mode = "lan"
	resetRegistryState()
	clearLog()
	TouchActivity()
	NSMu.Lock()
	Net = NetState{SSID: "Beam", LanMode: true, Security: "wpa",
		WifiPassword: "Share12345"}
	NSMu.Unlock()
	hsMu.Lock()
	hsRunning = false
	hsSSID = ""
	hsIP = ""
	hsPort = 2004
	hsSecurity = "wpa"
	hsPassword = ""
	hsOwnProfile = false
	hsMu.Unlock()
	sessMu.Lock()
	sessLocks = map[string]*sync.Mutex{}
	sessMu.Unlock()
	netcapMu.Lock()
	netcapAt = 0
	netcapMu.Unlock()
	// Freeze Wi-Fi auto-detect: tests must never depend on the host's real
	// Wi-Fi (nmcli on the build machine leaks SSIDs like "micro" into
	// /api/status). Fresh "none" cache => effectiveWifi() falls back to the
	// deterministic manual Cfg values set above.
	wifiCacheMu.Lock()
	wifiCache = WifiInfo{Source: "none"}
	wifiCacheAt = time.Now()
	wifiCacheMu.Unlock()
	ServerPort = 18793
	AppVersion = "1.2.1"
	return tmp
}

func TestSafeFilename(t *testing.T) {
	valid := map[string]string{
		"a.bin":           "a.bin",
		"  spaced.txt  ":  "spaced.txt",
		`C:\dir\file.txt`: "file.txt",
		"/tmp/x/y.dat":    "y.dat",
		"../x":            "x", // basename only, like Python
		"a/b":             "b",
		"Report (1).PDF":  "Report (1).PDF",
		"ملف عربي.mp4":    "ملف عربي.mp4",
	}
	for in, want := range valid {
		if got := safeFilename(in); got != want {
			t.Errorf("safeFilename(%q) = %q, want %q", in, got, want)
		}
	}
	// Note: dotfiles pass safeFilename; callers enforce the dot rule.
	for _, in := range []string{"", ".", "..", "CON", "con.txt",
		"PRN.log", "aux", "NUL.dat", "COM1", "com9.txt", "LPT1", "lpt5.dat", "a\x00b"} {
		if got := safeFilename(in); got != "" {
			t.Errorf("safeFilename(%q) = %q, want empty", in, got)
		}
	}
	if got := safeFilename(".hidden"); got != ".hidden" {
		t.Errorf("dotfiles pass safeFilename (callers reject), got %q", got)
	}
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	if got := uniquePath(dir, "a.txt"); filepath.Base(got) != "a.txt" {
		t.Fatalf("fresh name changed: %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := uniquePath(dir, "a.txt"); filepath.Base(got) != "a (1).txt" {
		t.Fatalf("collision name = %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "a (1).txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := uniquePath(dir, "a.txt"); filepath.Base(got) != "a (2).txt" {
		t.Fatalf("second collision name = %q", got)
	}
	if got := uniquePath(dir, "noext"); filepath.Base(got) != "noext" {
		t.Fatalf("extensionless changed: %q", got)
	}
}

func TestPercentEncode(t *testing.T) {
	if got := percentEncode("abc-_.~123"); got != "abc-_.~123" {
		t.Errorf("unreserved changed: %q", got)
	}
	if got := percentEncode("a b.txt"); got != "a%20b.txt" {
		t.Errorf("space encoding = %q", got)
	}
	// Must match Python urllib.parse.quote (uppercase hex, UTF-8 bytes).
	if got := percentEncode("ملف.mp4"); got != "%D9%85%D9%84%D9%81.mp4" {
		t.Errorf("arabic encoding = %q", got)
	}
}

func TestFmtSpeed(t *testing.T) {
	cases := []struct {
		n    int64
		sec  float64
		want string
	}{
		{500, 1, "500 بايت/ث"},
		{2048, 1, "2.0 ك.ب/ث"},
		{3 * 1048576, 1, "3.0 م.ب/ث"},
		{3 * 1073741824, 2, "1.50 ج.ب/ث"},
		{100, 0, "97.7 ك.ب/ث"}, // seconds<=0 -> 0.001, like Python
	}
	for _, c := range cases {
		if got := fmtSpeed(c.n, c.sec); got != c.want {
			t.Errorf("fmtSpeed(%d,%v) = %q, want %q", c.n, c.sec, got, c.want)
		}
	}
}

func TestIsLoopback(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "127.0.0.2", "127.1.2.3", "::1", "::ffff:127.0.0.1", "::1%lo"} {
		if !isLoopback(ip) {
			t.Errorf("%s should be loopback", ip)
		}
	}
	for _, ip := range []string{"192.168.1.5", "10.0.0.2", "", "not-an-ip", "::ffff:192.168.1.5"} {
		if isLoopback(ip) {
			t.Errorf("%s should not be loopback", ip)
		}
	}
}

func TestParseRange(t *testing.T) {
	s, e, p := parseRange("bytes=0-99", 1000)
	if !p || s != 0 || e != 99 {
		t.Errorf("simple range = %d-%d partial=%v", s, e, p)
	}
	s, e, p = parseRange("bytes=900-", 1000)
	if !p || s != 900 || e != 999 {
		t.Errorf("open range = %d-%d partial=%v", s, e, p)
	}
	s, e, p = parseRange("bytes=-100", 1000)
	if !p || s != 900 || e != 999 {
		t.Errorf("suffix range = %d-%d partial=%v", s, e, p)
	}
	if _, _, p = parseRange("bytes=999-1", 1000); p {
		t.Error("inverted range should not be partial")
	}
	if _, _, p = parseRange("bytes=0-9999", 1000); !p {
		t.Error("clamped range should be partial")
	} else if e != 999 {
		t.Errorf("clamped end = %d", e)
	}
	if _, _, p = parseRange("garbage", 1000); p {
		t.Error("garbage should not be partial")
	}
	if _, _, p = parseRange("", 1000); p {
		t.Error("empty should not be partial")
	}
}

func TestMimeByName(t *testing.T) {
	if got := mimeByName("a.mp4"); got != "video/mp4" {
		t.Errorf("mp4 = %q", got)
	}
	if got := mimeByName("a.unknownext123"); got != "application/octet-stream" {
		t.Errorf("unknown = %q", got)
	}
	if got := mimeByName("a.txt"); got == "" || strings.Contains(got, ";") {
		t.Errorf("txt should be plain mime without params, got %q", got)
	}
}

func TestValidatePortSSID(t *testing.T) {
	if ValidatePort(8000) != 8000 || ValidatePort(0) != -1 ||
		ValidatePort(70000) != -1 || ValidatePort(1) != 1 {
		t.Error("ValidatePort wrong")
	}
	if validateSSID("  ") != "Beam" || validateSSID(" X ") != "X" {
		t.Error("validateSSID wrong")
	}
}
