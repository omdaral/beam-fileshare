package beamcore

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func reqWithPeer(path, peer, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.RemoteAddr = peer
	if token != "" {
		req.Header.Set("X-Beam-Owner", token)
	}
	rec := httptest.NewRecorder()
	Route(rec, req)
	return rec
}

func TestOwnerLoopbackNeedsNoToken(t *testing.T) {
	rotateOwnerToken()
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	if !hasAdmin(req) {
		t.Fatal("loopback peer must be admin without any token")
	}
	req6 := httptest.NewRequest("GET", "/api/status", nil)
	req6.RemoteAddr = "[::1]:1234"
	if !hasAdmin(req6) {
		t.Fatal("::1 peer must be admin without any token")
	}
}

func TestOwnerTokenLANFullOwner(t *testing.T) {
	rotateOwnerToken()
	tok := OwnerTokenHex()
	if len(tok) != 64 {
		t.Fatalf("token must be 64 hex chars, got %q", tok)
	}
	// LAN peer without token: guest.
	guest := httptest.NewRequest("GET", "/api/status", nil)
	guest.RemoteAddr = "192.168.1.9:1234"
	if hasAdmin(guest) {
		t.Fatal("LAN peer without token must NOT be admin")
	}
	// LAN peer with valid token: full owner.
	owner := httptest.NewRequest("GET", "/api/status", nil)
	owner.RemoteAddr = "192.168.1.9:1234"
	owner.Header.Set("X-Beam-Owner", tok)
	if !hasAdmin(owner) {
		t.Fatal("LAN peer with valid token must be admin")
	}
	// Wrong token: guest.
	bad := httptest.NewRequest("GET", "/api/status", nil)
	bad.RemoteAddr = "192.168.1.9:1234"
	bad.Header.Set("X-Beam-Owner", strings.Repeat("0", 64))
	if hasAdmin(bad) {
		t.Fatal("wrong token must NOT be admin")
	}
	// Empty token never validates (even if server token were empty).
	if validOwnerToken("") {
		t.Fatal("empty token must never validate")
	}
}

func TestOwnerTokenDisclosedOnlyToOwners(t *testing.T) {
	rotateOwnerToken()
	// Guest response must not contain the key at all.
	rec := reqWithPeer("/api/status", "192.168.1.9:1234", "")
	var guest map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &guest); err != nil {
		t.Fatalf("guest status not JSON: %v", err)
	}
	if _, ok := guest["owner_token"]; ok {
		t.Fatal("guest response must not contain owner_token key")
	}
	if guest["is_admin"] != false {
		t.Fatalf("guest is_admin must be false, got %v", guest["is_admin"])
	}
	// Loopback response carries the token and admin flag.
	rec2 := reqWithPeer("/api/status", "127.0.0.1:1234", "")
	var own map[string]interface{}
	if err := json.Unmarshal(rec2.Body.Bytes(), &own); err != nil {
		t.Fatalf("owner status not JSON: %v", err)
	}
	tok, _ := own["owner_token"].(string)
	if len(tok) != 64 || tok != OwnerTokenHex() {
		t.Fatalf("owner response must carry the session token, got %v", own["owner_token"])
	}
	if own["is_admin"] != true {
		t.Fatalf("loopback is_admin must be true, got %v", own["is_admin"])
	}
	// A LAN client presenting that token is admin too.
	rec3 := reqWithPeer("/api/status", "192.168.1.9:1234", tok)
	var paired map[string]interface{}
	if err := json.Unmarshal(rec3.Body.Bytes(), &paired); err != nil {
		t.Fatalf("paired status not JSON: %v", err)
	}
	if paired["is_admin"] != true {
		t.Fatalf("token-paired LAN client must be admin, got %v", paired["is_admin"])
	}
}

func TestOwnerTokenRotatesPerStart(t *testing.T) {
	rotateOwnerToken()
	first := OwnerTokenHex()
	rotateOwnerToken()
	second := OwnerTokenHex()
	if first == second {
		t.Fatal("token must rotate on every start")
	}
	old := httptest.NewRequest("GET", "/api/status", nil)
	old.RemoteAddr = "192.168.1.9:1234"
	old.Header.Set("X-Beam-Owner", first)
	if hasAdmin(old) {
		t.Fatal("previous session token must be rejected after rotation")
	}
}
