package beamcore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Simulates a restart: files on disk, empty memory registry, then restore.
// Managed model: loose files keep original names; legacy hidden .shares
// orphans are MIGRATED to real managed files (<temp>/restored-<short>).
func TestRestoreTempShares(t *testing.T) {
	setupTestEnv(t)

	// Loose host file in the temp root.
	if err := os.WriteFile(filepath.Join(TempDir, "old-report.pdf"), []byte("0123456789abcdef"), 0644); err != nil {
		t.Fatal(err)
	}
	// Noise that must NOT come back: dot file, empty file, symlink.
	if err := os.WriteFile(filepath.Join(TempDir, ".hidden"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(TempDir, "empty.txt"), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink("old-report.pdf", filepath.Join(TempDir, "link.pdf"))
	// Legacy hidden guest bytes (pre-restart share id as filename).
	sd := sharesDir()
	if err := os.MkdirAll(sd, 0755); err != nil {
		t.Fatal(err)
	}
	orphanID := "0123456789abcdef01234567"
	if err := os.WriteFile(filepath.Join(sd, orphanID), []byte("guest-bytes-123"), 0644); err != nil {
		t.Fatal(err)
	}

	// Restart: wipe memory, restore from disk.
	resetRegistryState()
	loose, adopted := RestoreTempShares()
	if loose != 1 {
		t.Fatalf("loose = %d, want 1", loose)
	}
	if adopted != 1 {
		t.Fatalf("adopted = %d, want 1", adopted)
	}

	// Both served via /api/shares as available.
	rec := reqWithPeer("/api/shares", "127.0.0.1:1", "")
	if rec.Code != 200 {
		t.Fatalf("GET /api/shares = %d", rec.Code)
	}
	var j struct {
		Shares []map[string]interface{} `json:"shares"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &j); err != nil {
		t.Fatal(err)
	}
	if len(j.Shares) != 2 {
		t.Fatalf("shares = %d, want 2: %v", len(j.Shares), j.Shares)
	}
	seen := map[string]bool{}
	for _, s := range j.Shares {
		if s["available"] != true {
			t.Errorf("share not available: %v", s)
		}
		seen[s["name"].(string)] = true
	}
	if !seen["old-report.pdf"] {
		t.Errorf("loose file not restored: %v", j.Shares)
	}

	// Legacy orphan migrated to a REAL managed file with full data
	// (not a ghost entry pointing at hidden bytes).
	short := orphanID[:8]
	migratedName := "restored-" + short
	if !seen[migratedName] {
		t.Errorf("migrated orphan %q missing: %v", migratedName, j.Shares)
	}
	migratedPath := filepath.Join(TempDir, migratedName)
	if got, err := os.ReadFile(migratedPath); err != nil || string(got) != "guest-bytes-123" {
		t.Errorf("migrated file %q = %q, %v; want %q", migratedPath, string(got), err, "guest-bytes-123")
	}
	if _, err := os.Stat(filepath.Join(sd, orphanID)); !os.IsNotExist(err) {
		t.Errorf("legacy hidden file should be gone after migration: %v", err)
	}

	// Second restart is stable: migrated file is now a loose file (2 loose,
	// 0 adopted), nothing duplicated, nothing swept.
	resetRegistryState()
	loose2, adopted2 := RestoreTempShares()
	if loose2 != 2 || adopted2 != 0 {
		t.Fatalf("second restore = (%d,%d), want (2,0)", loose2, adopted2)
	}
	SweepTempOrphans()
	regMu.Lock()
	n := len(shares)
	regMu.Unlock()
	if n != 2 {
		t.Fatalf("shares after sweep = %d, want 2", n)
	}

	// Old (>24h) hidden orphans are also migrated (full data preserved).
	oldID := "abcdef0123456789abcdef01"
	oldPath := filepath.Join(sd, oldID)
	if err := os.WriteFile(oldPath, []byte("old-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(oldPath, old, old); err != nil {
		t.Fatal(err)
	}
	resetRegistryState()
	// 2 loose files still there + 1 newly migrated orphan.
	loose3, adopted3 := RestoreTempShares()
	if loose3 != 2 || adopted3 != 1 {
		t.Fatalf("restore with aged orphan = (%d,%d), want (2,1)", loose3, adopted3)
	}
}
