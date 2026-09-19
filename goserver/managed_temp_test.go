package beamcore

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// Managed model: completed browser uploads land as REAL files with original
// names, survive restart with the same name, and vanish on unshare.
func TestManagedUploadKeepsNameAcrossRestart(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	payload := []byte("managed-bytes-abcdef-123456")
	st, j, _ := doJSON(t, "POST", ts.URL+"/api/share/guest",
		map[string]interface{}{
			"name": "MyFolder/hello.txt", "size": len(payload), "kind": "file",
			"owner": "owner-phone",
		}, nil)
	if st != 200 {
		t.Fatalf("announce = %d %v", st, j)
	}
	id, _ := j["id"].(string)
	sess, _ := j["session"].(string)
	if id == "" || sess == "" {
		t.Fatalf("bad announce %v", j)
	}
	h := shaHex(payload)
	if st, _ := postBin(t, ts.URL+"/upload_piece?id="+sess+"&index=0&hash="+h, payload); st != 200 {
		t.Fatalf("piece = %d", st)
	}
	st, j, _ = doJSON(t, "POST", ts.URL+"/upload_complete",
		map[string]interface{}{"id": sess}, nil)
	if st != 200 {
		t.Fatalf("complete = %d %v", st, j)
	}

	// Real file with original rel path + full data.
	realPath := filepath.Join(TempDir, "MyFolder", "hello.txt")
	if got, err := os.ReadFile(realPath); err != nil || string(got) != string(payload) {
		t.Fatalf("real file %q = %q, %v", realPath, string(got), err)
	}

	// Listed with original name, available.
	st, j, _ = doJSON(t, "GET", ts.URL+"/api/shares", nil, nil)
	if st != 200 {
		t.Fatalf("shares = %d", st)
	}
	found := false
	for _, s := range j["shares"].([]interface{}) {
		mm, _ := s.(map[string]interface{})
		if mm["id"] == id {
			found = true
			if mm["name"] != "MyFolder/hello.txt" {
				t.Fatalf("bad name %v", mm)
			}
			if avail, _ := mm["available"].(bool); !avail {
				t.Fatalf("not available %v", mm)
			}
		}
	}
	if !found {
		t.Fatalf("share %s missing in %v", id, j)
	}

	// Download via /r/<id> returns exact bytes.
	resp, err := http.Get(ts.URL + "/r/" + id)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != string(payload) {
		t.Fatalf("download = %d %q", resp.StatusCode, string(body))
	}

	// Restart: memory wiped, files stay with ORIGINAL names (no restored-xxx).
	resetRegistryState()
	loose, adopted := RestoreTempShares()
	if adopted != 0 {
		t.Fatalf("adopted = %d, want 0 (no hidden .shares anymore)", adopted)
	}
	if loose < 1 {
		t.Fatalf("loose = %d, want >=1", loose)
	}
	rec := reqWithPeer("/api/shares", "127.0.0.1:1", "")
	if rec.Code != 200 {
		t.Fatalf("shares after restart = %d", rec.Code)
	}
	var js struct {
		Shares []map[string]interface{} `json:"shares"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &js); err != nil {
		t.Fatal(err)
	}
	seenFolder := false
	for _, s := range js.Shares {
		nm, _ := s["name"].(string)
		if nm == "MyFolder" || nm == "MyFolder/hello.txt" {
			seenFolder = true
			if s["available"] != true {
				t.Fatalf("not available after restart: %v", s)
			}
		}
		if nm == "restored-01234567" || (len(nm) > 9 && nm[:9] == "restored-") {
			t.Fatalf("ghost restored-xxx entry reappeared: %v", s)
		}
	}
	if !seenFolder {
		t.Fatalf("folder missing after restart: %v", js.Shares)
	}
	// Real bytes still intact.
	if got, err := os.ReadFile(realPath); err != nil || string(got) != string(payload) {
		t.Fatalf("bytes after restart = %q, %v", string(got), err)
	}
}

// Unshare of a managed file deletes the real bytes (never reappears).
func TestManagedUnshareDeletesRealFile(t *testing.T) {
	setupTestEnv(t)
	ts := testServer()
	defer ts.Close()

	payload := []byte("delete-me-bytes")
	st, j, _ := doJSON(t, "POST", ts.URL+"/api/share/guest",
		map[string]interface{}{"name": "gone.txt", "size": len(payload)}, nil)
	if st != 200 {
		t.Fatalf("announce = %d %v", st, j)
	}
	id, _ := j["id"].(string)
	sess, _ := j["session"].(string)
	h := shaHex(payload)
	if st, _ := postBin(t, ts.URL+"/upload_piece?id="+sess+"&index=0&hash="+h, payload); st != 200 {
		t.Fatalf("piece = %d", st)
	}
	if st2, _, _ := doJSON(t, "POST", ts.URL+"/upload_complete", map[string]interface{}{"id": sess}, nil); st2 != 200 {
		t.Fatalf("complete = %d", st2)
	}
	if _, err := os.Stat(filepath.Join(TempDir, "gone.txt")); err != nil {
		t.Fatalf("real file missing: %v", err)
	}
	st, _, _ = doJSON(t, "POST", ts.URL+"/api/unshare", map[string]interface{}{"id": id}, nil)
	if st != 200 {
		t.Fatalf("unshare = %d", st)
	}
	if _, err := os.Stat(filepath.Join(TempDir, "gone.txt")); !os.IsNotExist(err) {
		t.Fatalf("real file should be deleted, err=%v", err)
	}
	// Restart must not resurrect it.
	resetRegistryState()
	loose, adopted := RestoreTempShares()
	if loose+adopted != 0 {
		t.Fatalf("restore after delete = (%d,%d), want (0,0)", loose, adopted)
	}
}

// Live sync: manual copy into Beam-Temp appears without restart;
// external delete disappears.
func TestManagedLiveSync(t *testing.T) {
	setupTestEnv(t)

	if err := os.WriteFile(filepath.Join(TempDir, "manual.txt"), []byte("manual-data"), 0644); err != nil {
		t.Fatal(err)
	}
	rec := reqWithPeer("/api/shares", "127.0.0.1:1", "")
	if rec.Code != 200 {
		t.Fatalf("shares = %d", rec.Code)
	}
	var j struct {
		Shares []map[string]interface{} `json:"shares"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &j); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range j.Shares {
		if s["name"] == "manual.txt" && s["available"] == true {
			found = true
		}
	}
	if !found {
		t.Fatalf("manual copy not live-synced: %v", j.Shares)
	}
	// External delete disappears.
	if err := os.Remove(filepath.Join(TempDir, "manual.txt")); err != nil {
		t.Fatal(err)
	}
	rec2 := reqWithPeer("/api/shares", "127.0.0.1:1", "")
	var j2 struct {
		Shares []map[string]interface{} `json:"shares"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &j2); err != nil {
		t.Fatal(err)
	}
	for _, s := range j2.Shares {
		if s["name"] == "manual.txt" {
			t.Fatalf("deleted file still listed: %v", s)
		}
	}
}
