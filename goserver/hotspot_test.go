package beamcore

import (
	"strings"
	"testing"
)

func TestPsQuote(t *testing.T) {
	if got := psQuote("it's"); got != "it''s" {
		t.Errorf("psQuote = %q", got)
	}
	if got := psQuote("plain"); got != "plain" {
		t.Errorf("psQuote = %q", got)
	}
}

func TestNetshShowsStopped(t *testing.T) {
	if !netshShowsStopped("Status                 : Not started\r\n") {
		t.Error("english stopped not detected")
	}
	if !netshShowsStopped("الحالة : متوقف") {
		t.Error("arabic stopped not detected")
	}
	if netshShowsStopped("Status                 : Started") {
		t.Error("started misdetected as stopped")
	}
	if netshShowsStopped("no colon line") {
		t.Error("colon-less line misdetected")
	}
	if netshShowsStopped("") {
		t.Error("empty misdetected")
	}
}

func TestParseArpOutput(t *testing.T) {
	out := "? (192.168.1.5) at aa:bb:cc [ether] on wlan0\n" +
		"? (224.0.0.1) at 00:00:00 on wlan0\n" +
		"? (192.168.1.5) at aa:bb:cc [ether] on wlan0\n" +
		"? (10.0.0.9) at dd:ee:ff on eth0\n"
	got := parseArpOutput(out)
	if len(got) != 2 || got[0] != "192.168.1.5" || got[1] != "10.0.0.9" {
		t.Errorf("parseArpOutput = %v", got)
	}
}

func TestLanFallbackInfo(t *testing.T) {
	setupTestEnv(t)
	fb := LanFallbackInfo(2004)
	if fb["mode"] != "lan" {
		t.Errorf("mode = %v", fb["mode"])
	}
	url, _ := fb["url"].(string)
	if !strings.HasPrefix(url, "http://") || !strings.HasSuffix(url, ":2004") {
		t.Errorf("url = %q", url)
	}
	if msg, _ := fb["message_ar"].(string); !strings.Contains(msg, url) {
		t.Errorf("message missing url: %q", msg)
	}
	fb = LanFallbackInfo(0)
	if !strings.HasSuffix(fb["url"].(string), ":2004") {
		t.Errorf("bad port should fall back to 2004: %v", fb["url"])
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("abcdef", 4); got != "abcd" {
		t.Errorf("truncate = %q", got)
	}
	// Arabic is multibyte; truncation must not split runes.
	if got := truncateRunes("أبجد", 2); got != "أب" {
		t.Errorf("arabic truncate = %q", got)
	}
	if got := truncateRunes("ab", 5); got != "ab" {
		t.Errorf("short truncate = %q", got)
	}
}

func TestHotspotValidationPaths(t *testing.T) {
	setupTestEnv(t)
	ok, msg, _ := HotspotStart("S", "short", 2004, false, "ar")
	if ok || !strings.Contains(msg, "قصيرة") {
		t.Errorf("short password should fail: %v %q", ok, msg)
	}
	ok, msg, _ = HotspotStart("S", "longenough1", 0, false, "ar")
	if ok || !strings.Contains(msg, "البورت") {
		t.Errorf("bad port should fail: %v %q", ok, msg)
	}
	_, msgEn, _ := HotspotStart("S", "short", 2004, false, "en")
	if !strings.Contains(msgEn, "short") {
		t.Errorf("english message expected, got %q", msgEn)
	}
}
