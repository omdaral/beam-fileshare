package beamcore

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// downloadsDir returns the user's well-known Downloads folder on any OS:
//   - Linux: `xdg-user-dir DOWNLOAD` (handles localized names), else ~/Downloads
//   - Windows/macOS: ~/Downloads
//
// Falls back to "" when the home directory cannot be determined.
func downloadsDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	if runtime.GOOS == "linux" {
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = ctx
			_ = cancel
		}()
		if d := xdgDownloadDir(home); d != "" {
			return d
		}
		if xdg := strings.TrimSpace(os.Getenv("XDG_DOWNLOAD_DIR")); xdg != "" {
			if !filepath.IsAbs(xdg) {
				xdg = filepath.Join(home, xdg)
			}
			return xdg
		}
	}
	return filepath.Join(home, "Downloads")
}

// xdgDownloadDir queries xdg-user-dir with a 2s timeout (never hang boot).
func xdgDownloadDir(home string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "xdg-user-dir", "DOWNLOAD").Output()
	if err != nil {
		return ""
	}
	// xdg-user-dir prints $HOME itself when no user-dirs are
	// configured: that means "unknown", not "home is downloads".
	if d := strings.TrimSpace(string(out)); d != "" && d != home && filepath.IsAbs(d) {
		return d
	}
	return ""
}

// beamHomeOverride redirects ~/Downloads in tests (never set in prod).
var beamHomeOverride = ""

// SharedDefaultDir is the single well-known share folder: <Downloads>/Beam.
// It keeps the user's files visible in a place they already know instead
// of a hidden folder next to the binary (no bloatware behavior).
// Falls back to Beam-Shared next to the binary when home is unknown.
func SharedDefaultDir() string {
	if beamHomeOverride != "" {
		return filepath.Join(beamHomeOverride, "Downloads", "Beam")
	}
	if d := downloadsDir(); d != "" {
		return filepath.Join(d, "Beam")
	}
	return filepath.Join(BaseDir, "Beam-Shared")
}

// MigrateLegacyShared moves files from the old ./Shared folder next to the
// binary (pre-v1.6 layout) into the new Downloads/Beam folder, once.
// Existing names are never overwritten (a " (n)" suffix is added).
// The old folder is left in place when empty or moved-from.
func MigrateLegacyShared(newDir string) {
	old := filepath.Join(BaseDir, "Shared")
	if samePath(old, newDir) {
		return
	}
	entries, err := os.ReadDir(old)
	if err != nil || len(entries) == 0 {
		return
	}
	moved := 0
	for _, e := range entries {
		name := e.Name()
		if name == ".uploads" || name == ".hashcache.json" {
			continue // internal resumable-upload state, not user files
		}
		src := filepath.Join(old, name)
		dst := filepath.Join(newDir, name)
		if _, err := os.Stat(dst); err == nil {
			dst = uniquePath(newDir, name)
		}
		if err := os.Rename(src, dst); err == nil {
			moved++
			continue
		} else if isCrossDevice(err) {
			// EXDEV: Downloads on another mount — copy then remove.
			if copyPath(src, dst) == nil {
				_ = os.RemoveAll(src)
				moved++
			}
		}
	}
	if moved > 0 {
		fmt.Printf("  نقلنا %d ملف/مجلد من المجلد القديم إلى: %s\n", moved, newDir)
	}
}

func samePath(a, b string) bool {
	ra, errA := filepath.Abs(a)
	rb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return a == b
	}
	return ra == rb
}

func isCrossDevice(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "cross-device") ||
		strings.Contains(s, "invalid cross-device") ||
		strings.Contains(s, "exdev")
}

// copyPath copies file or dir tree src -> dst (dst must not exist).
func copyPath(src, dst string) error {
	st, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return copyFile(src, dst, st.Mode())
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyPath(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	_ = os.MkdirAll(filepath.Dir(dst), 0755)
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
	if err != nil {
		return err
	}
	_, cerr := io.Copy(out, in)
	xerr := out.Close()
	if cerr != nil {
		_ = os.Remove(dst)
		return cerr
	}
	return xerr
}
