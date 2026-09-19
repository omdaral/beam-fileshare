package beamcore

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Hybrid Wi-Fi sharing: status exposes effective credentials, detect is
// owner-only, and manual wifi_* fields persist via /api/config.
func TestWifiStatusExposesEffectiveCreds(t *testing.T) {
	setupTestEnv(t)
	// Manual fallback: no hotspot, auto-detect likely empty in CI.
	CfgMu.Lock()
	Cfg.WifiSSID = "HomeWifi"
	Cfg.WifiPassword = "secret123"
	CfgMu.Unlock()
	NSMu.Lock()
	Net = NetState{SSID: "Beam", LanMode: true, Security: "wpa"}
	NSMu.Unlock()
	wifiCacheMu.Lock()
	wifiCache = WifiInfo{Source: "none"}
	wifiCacheAt = time.Now().Add(-wifiCacheTTL - time.Second)
	// Freeze with a fresh auto probe result of "none" so effectiveWifi
	// falls back to manual deterministically in CI (no nmcli).
	wifiCache = WifiInfo{Source: "none"}
	wifiCacheAt = time.Now()
	wifiCacheMu.Unlock()

	rec := reqWithPeer("/api/status", "127.0.0.1:1", "")
	if rec.Code != 200 {
		t.Fatalf("GET /api/status = %d", rec.Code)
	}
	var j map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &j); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"wifi_ssid", "wifi_password", "wifi_source"} {
		if _, ok := j[k]; !ok {
			t.Errorf("status missing %q: %v", k, j)
		}
	}
}

func TestWifiDetectOwnerOnly(t *testing.T) {
	setupTestEnv(t)
	rotateOwnerToken()
	// Guest (LAN, no token): 403.
	g := httptest.NewRequest("GET", "/api/wifi/detect", nil)
	g.RemoteAddr = "192.168.1.9:1"
	rec := httptest.NewRecorder()
	Route(rec, g)
	if rec.Code != 403 {
		t.Fatalf("guest detect = %d, want 403", rec.Code)
	}
	// Owner (loopback): 200 with ok/source keys (ok may be false in CI).
	o := httptest.NewRequest("GET", "/api/wifi/detect", nil)
	o.RemoteAddr = "127.0.0.1:1"
	rec2 := httptest.NewRecorder()
	Route(rec2, o)
	if rec2.Code != 200 {
		t.Fatalf("owner detect = %d, want 200", rec2.Code)
	}
	var j map[string]interface{}
	if err := json.Unmarshal(rec2.Body.Bytes(), &j); err != nil {
		t.Fatal(err)
	}
	if _, ok := j["source"]; !ok {
		t.Errorf("detect missing source: %v", j)
	}
}

func TestWifiConfigManualFields(t *testing.T) {
	setupTestEnv(t)
	rotateOwnerToken()
	body := `{"wifi_ssid":"CafeNet","wifi_password":"cafe1234"}`
	req := httptest.NewRequest("POST", "/api/config", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:1"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	Route(rec, req)
	if rec.Code != 200 {
		t.Fatalf("POST /api/config = %d: %s", rec.Code, rec.Body.String())
	}
	CfgMu.RLock()
	defer CfgMu.RUnlock()
	if Cfg.WifiSSID != "CafeNet" || Cfg.WifiPassword != "cafe1234" {
		t.Fatalf("manual wifi not saved: %+v", Cfg)
	}
}
