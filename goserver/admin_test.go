package beamcore

import (
	"encoding/json"
	"testing"
)

func TestDefaultLang(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	// 1) Status endpoints advertise the default ("ar" from defaultConfig).
	st, j, _ := doJSON(t, "GET", ts.URL+"/api/status", nil, nil)
	if st != 200 || j["default_lang"] != "ar" {
		t.Fatalf("status default_lang = %d %v", st, j)
	}
	st, j, _ = doJSON(t, "GET", ts.URL+"/api/net/status", nil, nil)
	if st != 200 || j["default_lang"] != "ar" {
		t.Fatalf("net/status default_lang = %d %v", st, j)
	}

	// 2) Guest cannot change it.
	oldLoopback := isLoopback
	isLoopback = func(string) bool { return false }
	st, _, _ = doJSON(t, "POST", ts.URL+"/api/config",
		map[string]interface{}{"default_lang": "en"}, nil)
	if st != 403 {
		t.Errorf("guest config default_lang = %d, want 403", st)
	}
	isLoopback = oldLoopback

	// 3) Owner sets en: persisted + advertised.
	st, j, _ = doJSON(t, "POST", ts.URL+"/api/config",
		map[string]interface{}{"default_lang": "en"}, nil)
	if st != 200 || j["ok"] != true {
		t.Fatalf("config default_lang save = %d %v", st, j)
	}
	st, j, _ = doJSON(t, "GET", ts.URL+"/api/status", nil, nil)
	if st != 200 || j["default_lang"] != "en" {
		t.Fatalf("status after save = %d %v", st, j)
	}
	raw, _ := json.Marshal(cfgSnapshot())
	var saved map[string]interface{}
	_ = json.Unmarshal(raw, &saved)
	if saved["default_lang"] != "en" {
		t.Fatalf("config not applied to session: %v", saved)
	}

	// 4) Invalid values are rejected, valid one still stands.
	for _, bad := range []interface{}{"xx", "", 5, true} {
		st, _, _ = doJSON(t, "POST", ts.URL+"/api/config",
			map[string]interface{}{"default_lang": bad}, nil)
		if st != 400 {
			t.Errorf("bad default_lang %v = %d, want 400", bad, st)
		}
	}
	st, j, _ = doJSON(t, "GET", ts.URL+"/api/status", nil, nil)
	if st != 200 || j["default_lang"] != "en" {
		t.Fatalf("default_lang changed by invalid input: %d %v", st, j)
	}
}

func TestAdminFlow(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	// 1) localhost is admin in /api/status.
	st, j, _ := doJSON(t, "GET", ts.URL+"/api/status", nil, nil)
	if st != 200 || j["is_admin"] != true || j["version"] == nil {
		t.Fatalf("status = %d %v", st, j)
	}
	// 2) /api/net/status fields.
	st, j, _ = doJSON(t, "GET", ts.URL+"/api/net/status", nil, nil)
	for _, k := range []string{"ssid", "url", "lan_mode", "hotspot_running",
		"clients_count", "hotspot_available", "admin_capable", "is_admin", "version"} {
		if _, ok := j[k]; !ok {
			t.Fatalf("net/status missing %s: %v", k, j)
		}
	}

	// 3) Guest: public read ok WITH wifi password (page is the sharing
	// channel), IP list hidden, every mutation rejected — no exceptions.
	oldLoopback := isLoopback
	isLoopback = func(string) bool { return false }
	defer func() { isLoopback = oldLoopback }()
	st, j, _ = doJSON(t, "GET", ts.URL+"/api/net/status", nil, nil)
	if st != 200 || j["is_admin"] != false || j["wifi_password"] != "Share12345" {
		t.Fatalf("guest net = %d %v", st, j)
	}
	if _, isList := j["clients"].([]interface{}); isList {
		t.Errorf("guest must not see client list: %v", j["clients"])
	}
	st, _, _ = doJSON(t, "POST", ts.URL+"/api/net/stop", map[string]interface{}{}, nil)
	if st != 403 {
		t.Errorf("guest stop = %d", st)
	}
	st, _, _ = doJSON(t, "POST", ts.URL+"/api/net/start", map[string]interface{}{"mode": "lan"}, nil)
	if st != 403 {
		t.Errorf("guest start = %d", st)
	}
	st, _, _ = doJSON(t, "GET", ts.URL+"/api/logs?tail=5", nil, nil)
	if st != 403 {
		t.Errorf("guest logs = %d", st)
	}
	st, _, _ = doJSON(t, "POST", ts.URL+"/api/logs/clear", map[string]interface{}{}, nil)
	if st != 403 {
		t.Errorf("guest logs-clear = %d", st)
	}
	st, _, _ = doJSON(t, "GET", ts.URL+"/api/config", nil, nil)
	if st != 403 {
		t.Errorf("guest config = %d", st)
	}
	st, j, _ = doJSON(t, "GET", ts.URL+"/api/net/clients", nil, nil)
	if st != 200 {
		t.Fatalf("guest clients = %d", st)
	}
	if _, isList := j["clients"].([]interface{}); isList {
		t.Errorf("guest clients endpoint must return count only: %v", j)
	}

	// 4) Remote codes are gone: no route, cookies and tokens mean nothing.
	st, _, _ = doJSON(t, "POST", ts.URL+"/api/admin/unlock",
		map[string]interface{}{"code": "testcode99"}, nil)
	if st != 404 {
		t.Errorf("unlock route should be gone, = %d", st)
	}
	st, _, _ = doJSON(t, "POST", ts.URL+"/api/net/stop", map[string]interface{}{},
		map[string]string{"Cookie": "cs_admin=fake-token"})
	if st != 403 {
		t.Errorf("fake cookie must not grant admin, = %d", st)
	}
	st, _, _ = doJSON(t, "GET", ts.URL+"/api/logs?tail=5", nil,
		map[string]string{"X-Admin-Token": "fake-token"})
	if st != 403 {
		t.Errorf("fake token must not grant admin, = %d", st)
	}

	// 5) Evil origin rejected even for admin (restore loopback first).
	isLoopback = oldLoopback
	st, _, _ = doJSON(t, "POST", ts.URL+"/api/net/stop", map[string]interface{}{},
		map[string]string{"Origin": "http://evil.example"})
	if st != 403 {
		t.Errorf("evil origin = %d", st)
	}

	// 6) Config validation + persistence + restart flag.
	st, _, _ = doJSON(t, "POST", ts.URL+"/api/config", map[string]interface{}{"port": 99999}, nil)
	if st != 400 {
		t.Errorf("bad port = %d", st)
	}
	st, _, _ = doJSON(t, "POST", ts.URL+"/api/config", map[string]interface{}{"max_file_mb": 0}, nil)
	if st != 400 {
		t.Errorf("bad max = %d", st)
	}
	st, j, _ = doJSON(t, "POST", ts.URL+"/api/config",
		map[string]interface{}{"port": ServerPort, "max_file_mb": 100}, nil)
	if st != 200 || j["need_restart"] != false {
		t.Fatalf("config save = %d %v", st, j)
	}
	raw, _ := json.Marshal(cfgSnapshot())
	var saved map[string]interface{}
	_ = json.Unmarshal(raw, &saved)
	if int(saved["max_file_mb"].(float64)) != 100 {
		t.Fatalf("config not applied to session: %v", saved)
	}
	st, j, _ = doJSON(t, "POST", ts.URL+"/api/config",
		map[string]interface{}{"port": ServerPort + 1}, nil)
	if st != 200 || j["need_restart"] != true {
		t.Fatalf("config restart flag = %d %v", st, j)
	}
	doJSON(t, "POST", ts.URL+"/api/config", map[string]interface{}{"port": ServerPort}, nil)
	// access_code is gone: silently ignored and dropped from the file.
	st, j, _ = doJSON(t, "POST", ts.URL+"/api/config",
		map[string]interface{}{"access_code": "zzz"}, nil)
	if st != 200 {
		t.Fatalf("config with stale access_code = %d %v", st, j)
	}
	raw2, _ := json.Marshal(cfgSnapshot())
	var saved2 map[string]interface{}
	_ = json.Unmarshal(raw2, &saved2)
	if _, ok := saved2["access_code"]; ok {
		t.Errorf("access_code should be dropped from config: %v", saved2)
	}

	// 6b) Owner clears the log: file becomes empty.
	writeLog("127.0.0.1", "test", "before-clear")
	st, j, _ = doJSON(t, "POST", ts.URL+"/api/logs/clear", map[string]interface{}{}, nil)
	if st != 200 || j["ok"] != true {
		t.Fatalf("logs clear = %d %v", st, j)
	}
	st, j, _ = doJSON(t, "GET", ts.URL+"/api/logs?tail=5", nil, nil)
	if st != 200 || len(j["lines"].([]interface{})) != 0 {
		t.Fatalf("log should be empty after clear: %d %v", st, j)
	}

	// 6c) Owner sees the client list shape.
	st, j, _ = doJSON(t, "GET", ts.URL+"/api/net/clients", nil, nil)
	if st != 200 {
		t.Fatalf("local clients = %d", st)
	}
	if _, isList := j["clients"].([]interface{}); !isList {
		t.Errorf("owner must see client list: %v", j)
	}
	if _, ok := j["clients_count"]; !ok {
		t.Errorf("clients_count missing: %v", j)
	}

	// 7) LAN start from browser updates state + file.
	st, j, _ = doJSON(t, "POST", ts.URL+"/api/net/start",
		map[string]interface{}{"mode": "lan", "ssid": "TestLAN"}, nil)
	if st != 200 || j["ok"] != true {
		t.Fatalf("net start lan = %d %v", st, j)
	}
	NSMu.RLock()
	lan, ssid := Net.LanMode, Net.SSID
	NSMu.RUnlock()
	if !lan || ssid != "TestLAN" {
		t.Fatalf("net state = lan:%v ssid:%q", lan, ssid)
	}

	// 8) Server stop: guest rejected, localhost allowed (last).
	isLoopback = func(string) bool { return false }
	st, _, _ = doJSON(t, "POST", ts.URL+"/api/server/stop", map[string]interface{}{}, nil)
	if st != 403 {
		t.Errorf("guest server stop = %d", st)
	}
	isLoopback = oldLoopback
	st, _, _ = doJSON(t, "POST", ts.URL+"/api/server/stop", map[string]interface{}{}, nil)
	if st != 200 {
		t.Errorf("local server stop = %d", st)
	}
}
