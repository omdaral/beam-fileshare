package beamcore

import (
	"io"
	"net/http"
	"testing"
)

// Guest shares must use the V2 piece engine (the web client only speaks
// /upload_piece) and appear available in /api/shares after completion.
func TestGuestShareV2Flow(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	payload := []byte("guest-bytes-12345")
	st, j, _ := doJSON(t, "POST", ts.URL+"/api/share/guest",
		map[string]interface{}{
			"name": "hello.txt", "size": len(payload), "kind": "file",
			"owner_id": "abcdef1234567890abcdef1234567890",
			"owner":    "tester",
		}, nil)
	if st != 200 {
		t.Fatalf("announce = %d %v", st, j)
	}
	id, _ := j["id"].(string)
	sess, _ := j["session"].(string)
	if id == "" || sess == "" {
		t.Fatalf("bad announce resp %v", j)
	}

	// Status must be V2 with full missing list (not empty-queue fake).
	st, j, _ = doJSON(t, "GET", ts.URL+"/upload_status?id="+sess, nil, nil)
	if st != 200 {
		t.Fatalf("status = %d %v", st, j)
	}
	if _, ok := j["missing"]; !ok {
		t.Fatalf("status missing V2 framing: %v", j)
	}
	if _, ok := j["piece_len"]; !ok {
		t.Fatalf("status missing piece_len: %v", j)
	}

	// Upload single piece with hash.
	h := shaHex(payload)
	if st, _ := postBin(t, ts.URL+"/upload_piece?id="+sess+"&index=0&hash="+h, payload); st != 200 {
		t.Fatalf("piece = %d", st)
	}

	st, j, _ = doJSON(t, "POST", ts.URL+"/upload_complete",
		map[string]interface{}{"id": sess}, nil)
	if st != 200 {
		t.Fatalf("complete = %d %v", st, j)
	}

	// Presence heartbeat required when owner_id is set.
	st, j, _ = doJSON(t, "POST", ts.URL+"/api/presence",
		map[string]interface{}{"owner_id": "abcdef1234567890abcdef1234567890", "name": "tester"}, nil)
	if st != 200 {
		t.Fatalf("presence = %d %v", st, j)
	}

	st, j, _ = doJSON(t, "GET", ts.URL+"/api/shares", nil, nil)
	if st != 200 {
		t.Fatalf("shares = %d", st)
	}
	shares, _ := j["shares"].([]interface{})
	found := false
	for _, s := range shares {
		m, _ := s.(map[string]interface{})
		if m["id"] == id {
			found = true
			if avail, _ := m["available"].(bool); !avail {
				t.Fatalf("share not available: %v", m)
			}
			if m["name"] != "hello.txt" {
				t.Fatalf("bad name %v", m)
			}
		}
	}
	if !found {
		t.Fatalf("share %s missing in %v", id, j)
	}

	// Download via /r/<id> must return the exact bytes.
	resp, err := http.Get(ts.URL + "/r/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != string(payload) {
		t.Fatalf("download = %d %q", resp.StatusCode, string(body))
	}
}

// Legacy announces without owner_id stay available once complete (no presence gate).
func TestGuestShareNoOwnerStillAvailable(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	payload := []byte("no-owner-bytes")
	st, j, _ := doJSON(t, "POST", ts.URL+"/api/share/guest",
		map[string]interface{}{"name": "plain.txt", "size": len(payload)}, nil)
	if st != 200 {
		t.Fatalf("announce = %d %v", st, j)
	}
	id, _ := j["id"].(string)
	sess, _ := j["session"].(string)

	h := shaHex(payload)
	if st, _ := postBin(t, ts.URL+"/upload_piece?id="+sess+"&index=0&hash="+h, payload); st != 200 {
		t.Fatalf("piece = %d", st)
	}
	st, j, _ = doJSON(t, "POST", ts.URL+"/upload_complete",
		map[string]interface{}{"id": sess}, nil)
	if st != 200 {
		t.Fatalf("complete = %d %v", st, j)
	}
	st, j, _ = doJSON(t, "GET", ts.URL+"/api/shares", nil, nil)
	if st != 200 {
		t.Fatalf("shares = %d", st)
	}
	for _, s := range j["shares"].([]interface{}) {
		m, _ := s.(map[string]interface{})
		if m["id"] == id {
			if avail, _ := m["available"].(bool); !avail {
				t.Fatalf("ownerless share should be available: %v", m)
			}
			return
		}
	}
	t.Fatalf("share missing %v", j)
}
