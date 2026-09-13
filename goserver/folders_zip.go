package beamcore

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	errTooMany = errors.New("too many files")
	errTooBig  = errors.New("too big")
	errEmpty   = errors.New("empty")
)

// extractZipAsync unpacks an uploaded folder-zip in the background, AFTER
// the 200 response. Transfer is never touched: on any failure the zip
// stays as is and only a log line is written.
// sharedDir is snapshotted at spawn time (race-free vs tests reconfiguring
// the global SharedDir while the background job still runs).
func extractZipAsync(zipPath, rel, cip string) {
	sharedDir := SharedDir
	n, target, err := extractZipIn(sharedDir, zipPath)
	if err != nil {
		writeLog(cip, "unzip_fail", rel+" ("+truncateRunes(err.Error(), 120)+")")
		return
	}
	_ = os.Remove(zipPath)
	invalidateFileHashIn(sharedDir, rel)
	writeLog(cip, "unzip", target+" ("+strconv.FormatInt(int64(n), 10)+" files)")
}

// extractZip unpacks zipPath into Shared/, validating every entry with
// safeRelPath (zip-slip safe), skipping symlinks/dotfiles, never
// overwriting (uniquePath), within caps. Returns file count + top dir.
func extractZip(zipPath string) (int, string, error) {
	return extractZipIn(SharedDir, zipPath)
}

// extractZipIn is the dir-parameterized core (see extractZipAsync).
func extractZipIn(sharedDir, zipPath string) (int, string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, "", err
	}
	defer zr.Close()
	if len(zr.File) > maxZipFiles+100 {
		return 0, "", errTooMany
	}
	var total uint64
	for _, f := range zr.File {
		total += f.UncompressedSize64
		if total > uint64(maxZipBytes) {
			return 0, "", errTooBig
		}
	}
	n, skipped := 0, 0
	top := ""
	for _, f := range zr.File {
		rel := safeRelPath(strings.ReplaceAll(f.Name, "\\", "/"))
		if rel == "" {
			skipped++
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			skipped++
			continue
		}
		if n >= maxZipFiles {
			break
		}
		if top == "" {
			top, _ = splitParent(rel)
			if top == "" {
				top = rel
			} else if i := strings.Index(top, "/"); i >= 0 {
				top = top[:i]
			}
		}
		dest := filepath.Join(sharedDir, rel)
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(dest, 0755)
			continue
		}
		// Narrow lock: only name reservation under uploadLock.
		// The actual io.Copy runs unlocked (O_EXCL guards races).
		uploadLock.Lock()
		out := uniquePath(sharedDir, rel)
		_ = os.MkdirAll(filepath.Dir(out), 0755)
		uploadLock.Unlock()
		werr := func() error {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			w, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			if err != nil {
				return err
			}
			_, cerr := io.Copy(w, rc)
			xerr := w.Close()
			if cerr != nil {
				_ = os.Remove(out)
				return cerr
			}
			return xerr
		}()
		if werr != nil {
			skipped++
			continue
		}
		n++
	}
	if n == 0 {
		return 0, "", errEmpty
	}
	return n, top, nil
}
func invalidateFileHashPrefix(prefix string) {
	invalidateFileHashPrefixIn(SharedDir, prefix)
}
func invalidateFileHashPrefixIn(sharedDir, prefix string) {
	cachePath := filepath.Join(sharedDir, hashcacheName)
	uploadLock.Lock()
	defer uploadLock.Unlock()
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return
	}
	cache := map[string]hashEntry{}
	if err := json.Unmarshal(data, &cache); err != nil {
		return
	}
	changed := false
	for k := range cache {
		if strings.HasPrefix(k, prefix) {
			delete(cache, k)
			changed = true
		}
	}
	if changed {
		if out, err := json.Marshal(cache); err == nil {
			_ = os.WriteFile(cachePath, out, 0644)
		}
	}
}

// pruneEmptyParents removes newly-emptied ancestor dirs of a deleted path.
// os.Remove only removes empty dirs, so non-empty ones stop the climb.
func pruneEmptyParents(rel string) {
	dir, _ := splitParent(rel)
	for dir != "" {
		if err := os.Remove(filepath.Join(SharedDir, dir)); err != nil {
			return
		}
		dir, _ = splitParent(dir)
	}
}

// handleDownloadZip streams a folder as a zip archive (GET only).
// Pre-scans first so cap failures return clean JSON instead of a broken zip.
func handleDownloadZip(w http.ResponseWriter, r *http.Request) {
	rel := safeRelPath(r.URL.Query().Get("dir"))
	if rel == "" {
		fail(w, r, 400, "dir_bad")
		return
	}
	if _, _, err := sharedDirStat(rel); err != nil {
		fail(w, r, 404, "dir_gone")
		return
	}
	entries, truncated := walkSharedDir(rel, maxZipFiles+1)
	if len(entries) == 0 {
		sendJSON(w, r, 404, map[string]interface{}{"error": tr(reqLang(r), "zip_empty")})
		return
	}
	if truncated || len(entries) > maxZipFiles {
		fail(w, r, 413, "zip_too_big")
		return
	}
	var total int64
	for _, e := range entries {
		total += e.Size
		if total > maxZipBytes {
			fail(w, r, 413, "zip_too_big")
			return
		}
	}
	_, base := splitParent(rel)
	if base == "" {
		base = rel
	}
	zipName := base + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+percentEncode(zipName))
	w.WriteHeader(200)
	if r.Method == "HEAD" {
		return
	}
	zw := zip.NewWriter(w)
	for _, e := range entries {
		nameInZip := strings.TrimPrefix(e.Path, rel+"/")
		if nameInZip == "" || nameInZip == e.Path {
			continue
		}
		f, _, _, err := openSharedFile(e.Path)
		if err != nil {
			continue
		}
		fh := &zip.FileHeader{Name: nameInZip, Method: zip.Deflate}
		fh.SetModTime(time.Unix(int64(e.Mtime), 0))
		fw, err := zw.CreateHeader(fh)
		if err != nil {
			f.Close()
			break
		}
		_, _ = io.Copy(fw, f)
		f.Close()
	}
	_ = zw.Close()
	writeLog(clientIP(r), "download_zip",
		rel+" ("+strconv.FormatInt(int64(len(entries)), 10)+" files, "+strconv.FormatInt(total, 10)+"b)")
}
