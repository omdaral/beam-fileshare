package beamcore

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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
// stays as is (staging is removed) and only a log line is written.
// sharedDir is snapshotted at spawn time (race-free vs tests reconfiguring
// the global SharedDir while the background job still runs).
func extractZipAsync(zipPath, rel, cip string) {
	sharedDir := SharedDir
	n, skipped, tops, err := extractZipIn(sharedDir, zipPath)
	if err != nil {
		msg := rel + " (" + truncateRunes(err.Error(), 120) + ")"
		if skipped > 0 {
			msg += ", skipped " + strconv.FormatInt(int64(skipped), 10)
		}
		writeLog(cip, "unzip_fail", msg)
		return
	}
	_ = os.Remove(zipPath)
	for _, top := range tops {
		invalidateFileHashPrefixIn(sharedDir, top+"/")
	}
	invalidateListCaches()
	msg := strings.Join(tops, ", ") + " (" + strconv.FormatInt(int64(n), 10) + " files)"
	if skipped > 0 {
		msg += ", skipped " + strconv.FormatInt(int64(skipped), 10)
	}
	writeLog(cip, "unzip", msg)
}

// extractZip unpacks zipPath into Shared/, validating every entry with
// safeRelPath (zip-slip safe), skipping symlinks/dotfiles, never
// overwriting (uniquePath), within caps. Returns file count + top dir.
func extractZip(zipPath string) (int, string, error) {
	n, _, tops, err := extractZipIn(SharedDir, zipPath)
	top := ""
	if len(tops) > 0 {
		top = tops[0]
	}
	return n, top, err
}

// extractZipIn is the dir-parameterized core (see extractZipAsync). It
// stages the tree under SharedDir/.extract-<zipbase>-<nanotime>/ (the dot
// prefix keeps it out of /files listings, like .uploads), enforces live
// byte/file caps while copying (header sizes are attacker-controlled for
// deflated entries, so the pre-scan total alone is not trusted), then
// renames top-level entries into place without overwriting. On failure the
// staging dir is removed and the source zip is kept; the caller removes the
// zip only on success. Returns files written, entries skipped, and the
// placed top-level names (post-uniquePath, share-relative).
func extractZipIn(sharedDir, zipPath string) (int, int, []string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, 0, nil, err
	}
	defer zr.Close()
	if len(zr.File) > maxZipFiles+100 {
		return 0, 0, nil, errTooMany
	}
	var total uint64
	for _, f := range zr.File {
		total += f.UncompressedSize64
		if total > uint64(maxZipBytes) {
			return 0, 0, nil, errTooBig
		}
	}
	stage := filepath.Join(sharedDir, ".extract-"+filepath.Base(zipPath)+"-"+strconv.FormatInt(nowNano(), 10))
	if err := os.MkdirAll(stage, 0755); err != nil {
		return 0, 0, nil, err
	}
	drop := func() {
		_ = os.RemoveAll(stage)
	}
	var liveTotal uint64
	n, skipped := 0, 0
	dirMods := map[string]time.Time{}
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
			drop()
			return 0, skipped, nil, errTooMany
		}
		dest := filepath.Join(stage, rel)
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(dest, 0755)
			if mt := f.Modified; !mt.IsZero() {
				dirMods[dest] = mt
			}
			continue
		}
		if f.UncompressedSize64 > uint64(maxZipBytes) {
			drop()
			return 0, skipped, nil, errTooBig
		}
		_ = os.MkdirAll(filepath.Dir(dest), 0755)
		werr := func() error {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			w, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			if err != nil {
				return err
			}
			got, cerr := io.Copy(w, io.LimitReader(rc, maxZipBytes+1))
			xerr := w.Close()
			if cerr != nil {
				_ = os.Remove(dest)
				return cerr
			}
			if xerr != nil {
				_ = os.Remove(dest)
				return xerr
			}
			if uint64(got) > uint64(maxZipBytes) {
				_ = os.Remove(dest)
				return errTooBig
			}
			liveTotal += uint64(got)
			if liveTotal > uint64(maxZipBytes) {
				_ = os.Remove(dest)
				return errTooBig
			}
			return nil
		}()
		if werr != nil {
			if werr == errTooBig {
				drop()
				return 0, skipped, nil, werr
			}
			skipped++
			continue
		}
		// Best-effort mtime (was discarded before).
		if mt := f.Modified; !mt.IsZero() {
			_ = os.Chtimes(dest, mt, mt)
		}
		n++
	}
	if n == 0 {
		drop()
		return 0, skipped, nil, errEmpty
	}
	for dest, mt := range dirMods {
		_ = os.Chtimes(dest, mt, mt)
	}
	// Move top-level entries into place: reserve + rename under one lock so
	// the non-overwrite placement cannot lose a rename race.
	rd, err := os.ReadDir(stage)
	if err != nil {
		drop()
		return 0, skipped, nil, err
	}
	tops := []string{}
	for _, de := range rd {
		name := de.Name()
		uploadLock.Lock()
		out := uniquePath(sharedDir, name)
		merr := os.Rename(filepath.Join(stage, name), out)
		uploadLock.Unlock()
		if merr != nil {
			skipped++
			continue
		}
		if rp, rerr := filepath.Rel(sharedDir, out); rerr == nil {
			tops = append(tops, filepath.ToSlash(rp))
		} else {
			tops = append(tops, name)
		}
	}
	drop()
	if len(tops) == 0 {
		return 0, skipped, nil, errEmpty
	}
	return n, skipped, tops, nil
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

// handleDownloadZip streams a folder as a ZIP archive (GET only).
// ZIP-only policy: STORE (uncompressed) always — every stock OS/phone opens
// it, no CPU spent on deflate, exact Content-Length whenever the archive
// fits plain zip32 limits (chunked fallback otherwise).
// Pre-scans first so cap failures return clean JSON instead of a broken zip.
// With ?preflight=1 it returns {"ok":true,"files":N,"bytes":B,"name":"X.zip","method":"store"}
// without streaming, so the web UI can surface server errors as readable
// text before starting the download (an <a download> click cannot display
// a JSON error body — the user just sees "nothing downloads").
// ?method is accepted for compat but only store is served: deflate (or any
// other codec) returns 400 zip_store_only.
// NOTE: WriteHeader(200) is sent before streaming, so any failure after
// that point cannot change the status — it is only reported via the
// download_zip_fail / download_zip_empty log lines.
func handleDownloadZip(w http.ResponseWriter, r *http.Request) {
	rel := safeRelPath(r.URL.Query().Get("dir"))
	if rel == "" {
		fail(w, r, 400, "dir_bad")
		return
	}
	method := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("method")))
	if method == "" {
		method = "store"
	}
	if method != "store" {
		// ZIP-only: no deflate/gzip/br/rar/7z on generate (compat for
		// the widest devices). Reading foreign zips still supports
		// deflated entries via extractZipIn.
		fail(w, r, 400, "zip_store_only")
		return
	}
	if _, _, err := sharedDirStat(rel); err != nil {
		fail(w, r, 404, "dir_gone")
		return
	}
	entries, dirs, truncated := walkSharedDirWithEmpty(rel, maxZipFiles+1)
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
	if r.URL.Query().Get("preflight") == "1" {
		sendJSON(w, r, 200, map[string]interface{}{
			"ok": true, "files": len(entries), "bytes": total, "name": zipName, "method": "store",
		})
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+asciiFallbackName(zipName)+"\"; filename*=UTF-8''"+percentEncode(zipName))
	w.Header().Set("Accept-Ranges", "none")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	var wantWire int64 = -1
	if wire, ok := storeZipLength(rel, entries, dirs); ok {
		wantWire = wire
		w.Header().Set("Content-Length", strconv.FormatInt(wire, 10))
	}
	// else: past plain zip32 limits — fall back to chunked streaming.
	w.WriteHeader(200)
	if r.Method == "HEAD" {
		return
	}
	if !acquireDownloadSlot(r) {
		return
	}
	defer releaseDownloadSlot()
	bc := &byteCounter{w: w}
	zw := zip.NewWriter(bc)
	written, skipped := 0, 0
	var streamErr error
	for _, d := range dirs {
		fh := &zip.FileHeader{Name: d.name, Method: zip.Store}
		fh.SetModTime(d.mtime)
		if _, err := zw.CreateHeader(fh); err != nil {
			skipped++
			continue
		}
	}
	buf := getCopyBuf()
	defer putCopyBuf(buf)
	for _, e := range entries {
		nameInZip := strings.TrimPrefix(e.Path, rel+"/")
		if nameInZip == "" || nameInZip == e.Path {
			skipped++
			continue
		}
		f, _, _, err := openSharedFile(e.Path)
		if err != nil {
			skipped++
			continue
		}
		fh := &zip.FileHeader{Name: nameInZip, Method: zip.Store}
		fh.SetModTime(time.Unix(int64(e.Mtime), 0))
		fw, err := zw.CreateHeader(fh)
		if err != nil {
			f.Close()
			skipped++
			break
		}
		if _, err := io.CopyBuffer(fw, f, buf); err != nil {
			f.Close()
			skipped++
			streamErr = err
			break
		}
		f.Close()
		written++
	}
	cip := clientIP(r)
	if cerr := zw.Close(); cerr != nil {
		writeLog(cip, "download_zip_fail", rel+" ("+truncateRunes(cerr.Error(), 120)+")")
		return
	}
	if streamErr != nil {
		writeLog(cip, "download_zip_fail", rel+" ("+truncateRunes(streamErr.Error(), 120)+")")
		return
	}
	if written == 0 {
		writeLog(cip, "download_zip_empty", rel+" (all "+strconv.FormatInt(int64(skipped), 10)+" files skipped)")
		return
	}
	msg := rel + " (" + strconv.FormatInt(int64(written), 10) + " files, " + strconv.FormatInt(total, 10) + "b, method store, wire " + strconv.FormatInt(bc.n, 10) + "b"
	if wantWire >= 0 {
		msg += "/expected " + strconv.FormatInt(wantWire, 10) + "b"
	}
	if skipped > 0 {
		msg += ", skipped " + strconv.FormatInt(int64(skipped), 10)
	}
	writeLog(cip, "download_zip", msg)
}

// byteCounter wraps the ResponseWriter to count the ACTUAL wire bytes of a
// streamed zip (headers included), so the success log can compare them
// against the pre-scan Content-Length math.
type byteCounter struct {
	w http.ResponseWriter
	n int64
}

func (c *byteCounter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// asciiFallbackName sanitizes a zip filename to printable ASCII for the
// legacy filename="..." disposition parameter (old Safari ignores
// filename*=); anything else becomes an underscore.
func asciiFallbackName(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x20 && c <= 0x7e && c != '"' && c != '\\' {
			b.WriteByte(c)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "folder.zip"
	}
	return b.String()
}

// zipDirEntry is one explicit empty-directory entry for the zip stream
// (name keeps the trailing slash, relative to the requested folder).
type zipDirEntry struct {
	name  string
	mtime time.Time
}

// collectEmptyDirs finds file-less subdirectories under rel for explicit zip
// entries (walkSharedDir only yields files, so empty dirs would otherwise be
// lost). Mirrors the walker exclusions (dot names, symlinks, depth cap) and
// returns names relative to rel with a trailing slash, sorted.
func collectEmptyDirs(rel string, entries []sharedEntry) []zipDirEntry {
	prefix := rel + "/"
	ancestors := map[string]bool{}
	for _, e := range entries {
		name := strings.TrimPrefix(e.Path, prefix)
		for {
			i := strings.LastIndex(name, "/")
			if i < 0 {
				break
			}
			name = name[:i]
			ancestors[name] = true
		}
	}
	baseDepth := len(strings.Split(rel, "/"))
	type item struct {
		disk  string
		sub   string // path relative to rel, "" for rel itself
		depth int
	}
	out := []zipDirEntry{}
	stack := []item{{disk: filepath.Join(SharedDir, rel), depth: baseDepth}}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		rd, err := os.ReadDir(cur.disk)
		if err != nil {
			continue
		}
		for _, de := range rd {
			nm := de.Name()
			if strings.HasPrefix(nm, ".") {
				continue
			}
			if de.Type()&os.ModeSymlink != 0 {
				continue
			}
			if !de.IsDir() {
				continue
			}
			sub := nm
			if cur.sub != "" {
				sub = cur.sub + "/" + nm
			}
			if cur.depth+1 >= maxRelDepth {
				continue
			}
			info, err := de.Info()
			if err != nil {
				continue
			}
			stack = append(stack, item{disk: filepath.Join(cur.disk, nm), sub: sub, depth: cur.depth + 1})
			if !ancestors[sub] {
				out = append(out, zipDirEntry{name: sub + "/", mtime: info.ModTime()})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// storeZipLength returns the exact wire size of the STORE archive Go's
// archive/zip writer will emit for these entries, or false when the archive
// does not fit plain zip32 limits (the caller then streams chunked).
// Per file entry the writer emits: local header (30) + name +
// extended-timestamp extra (9, always appended when Modified is set via
// SetModTime) + data + data descriptor (16, always written by CreateHeader
// for non-directory entries); per directory entry: local header (30) + name
// + extra (9), no descriptor; per entry central record: 46 + name + extra
// (9); EOCD is 22. UTF-8 names only set the flag bit (no extra field).
// Zip64 kicks in conservatively (any size/offset reaching 4GiB-1, or 65535+
// records), so anything near the limits falls back instead of guessing.
func storeZipLength(rel string, entries []sharedEntry, dirs []zipDirEntry) (int64, bool) {
	if int64(len(entries)+len(dirs)) >= 65535 {
		return 0, false
	}
	var total int64
	add := func(n int64) bool {
		total += n
		return total < 1<<32
	}
	for _, e := range entries {
		name := strings.TrimPrefix(e.Path, rel+"/")
		if name == "" || name == e.Path || len(name) > 0xffff {
			return 0, false
		}
		if e.Size < 0 || uint64(e.Size) >= 0xffffffff {
			return 0, false
		}
		// local 30 + extra 9 + descriptor 16, then central 46 + extra 9.
		if !add(30 + 9 + int64(len(name)) + e.Size + 16) {
			return 0, false
		}
		if !add(46 + 9 + int64(len(name))) {
			return 0, false
		}
	}
	for _, d := range dirs {
		if len(d.name) == 0 || len(d.name) > 0xffff {
			return 0, false
		}
		if !add(30 + 9 + int64(len(d.name))) {
			return 0, false
		}
		if !add(46 + 9 + int64(len(d.name))) {
			return 0, false
		}
	}
	if !add(22) {
		return 0, false
	}
	return total, true
}
