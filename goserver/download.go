package beamcore

import (
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// parseRange parses "bytes=s-e" (first spec only), mirroring fileshare.py.
// Returns start, end, partial.
func parseRange(header string, size int64) (int64, int64, bool) {
	if size <= 0 {
		return 0, -1, false
	}
	start, end := int64(0), size-1
	partial := false
	h := strings.TrimSpace(header)
	if len(h) < 6 || !strings.EqualFold(h[:6], "bytes=") {
		return start, end, false
	}
	spec := strings.TrimSpace(strings.SplitN(h[6:], ",", 2)[0])
	if strings.HasPrefix(spec, "-") {
		n, err := strconv.ParseInt(strings.TrimSpace(spec[1:]), 10, 64)
		if err != nil {
			return 0, size - 1, false
		}
		start = size - n
		if start < 0 {
			start = 0
		}
	} else {
		parts := strings.SplitN(spec, "-", 2)
		s := strings.TrimSpace(parts[0])
		e := ""
		if len(parts) > 1 {
			e = strings.TrimSpace(parts[1])
		}
		var err error
		if s == "" {
			start = 0
		} else {
			start, err = strconv.ParseInt(s, 10, 64)
			if err != nil {
				return 0, size - 1, false
			}
			if start < 0 {
				start = 0
			}
		}
		if e != "" {
			end, err = strconv.ParseInt(e, 10, 64)
			if err != nil {
				return 0, size - 1, false
			}
			if end > size-1 {
				end = size - 1
			}
		}
	}
	if 0 <= start && start <= end && end < size {
		partial = true
	} else {
		start, end = 0, size-1
	}
	return start, end, partial
}
func mimeByName(name string) string {
	ctype := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	if i := strings.Index(ctype, ";"); i >= 0 {
		ctype = strings.TrimSpace(ctype[:i])
	}
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	return ctype
}
func handleDownload(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("file")
	name, ok := mustRel(raw)
	if !ok {
		fail(w, r, 400, "up_bad_name")
		return
	}
	f, st, _, _ := openSharedFile(name)
	if f == nil || st == nil {
		fail(w, r, 404, "delete_missing")
		return
	}
	size := st.Size()
	ctype := mimeByName(name)
	disp := "attachment; filename*=UTF-8''" + percentEncode(filepath.Base(name))
	etag := "\"" + strconv.FormatInt(size, 10) + "-" + strconv.FormatInt(int64(st.ModTime().Unix()), 10) + "\""
	lastMod := st.ModTime().UTC().Format(http.TimeFormat)
	// Conditional GET: revalidation without re-download (RFC 7232).
	// If-None-Match wins over If-Modified-Since.
	if etagMatches(r, etag) || modifiedSinceAllowsNotModified(r, lastMod) {
		f.Close()
		notModified(w, etag, lastMod)
		return
	}
	if r.Method == "HEAD" {
		f.Close()
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.Header().Set("Content-Disposition", disp)
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", lastMod)
		w.WriteHeader(200)
		return
	}
	rawRange := r.Header.Get("Range")
	start, end, partial := parseRange(rawRange, size)
	// Stale If-Range validator: fall back to full 200 instead of a
	// mismatched 206 (resume-safe).
	if partial && !ifRangeAllowsPartial(r, etag, lastMod) {
		partial = false
		start, end = 0, size-1
	}
	if strings.TrimSpace(rawRange) != "" && !partial && r.Header.Get("If-Range") == "" {
		// A Range header was present but unsatisfiable (start>end,
		// past EOF, or unparsable): 416, not a silent 200 full body.
		w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(size, 10))
		f.Close()
		fail(w, r, 416, "up_range_invalid")
		return
	}
	if partial {
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.Header().Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+strconv.FormatInt(end, 10)+"/"+strconv.FormatInt(size, 10))
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Disposition", disp)
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", lastMod)
		w.WriteHeader(206)
	} else {
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Disposition", disp)
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", lastMod)
		w.WriteHeader(200)
	}
	t0 := nowNano()
	var sent int64
	defer f.Close()
	if !acquireDownloadSlot(r) {
		return
	}
	defer releaseDownloadSlot()
	if partial {
		_, _ = f.Seek(start, io.SeekStart)
		sent = streamCopy(w, f, end-start+1)
		dt := float64(nowNano()-t0) / 1e9
		if dt < 0.001 {
			dt = 0.001
		}
		writeLog(clientIP(r), "download",
			name+" ["+strconv.FormatInt(start, 10)+"-"+strconv.FormatInt(end, 10)+"] ("+strconv.FormatInt(sent, 10)+"b, "+ftoa1(dt)+"s, "+fmtSpeed(sent, dt)+")")
		return
	}
	sent = streamCopy(w, f, size)
	dt := float64(nowNano()-t0) / 1e9
	if dt < 0.001 {
		dt = 0.001
	}
	writeLog(clientIP(r), "download",
		name+" ("+strconv.FormatInt(sent, 10)+"b, "+ftoa1(dt)+"s, "+fmtSpeed(sent, dt)+")")
}

// streamCopy copies up to n bytes using a pooled buffer via io.CopyBuffer,
// which lets the runtime use sendfile(2) (zero-copy) for file→socket paths.
// Write errors (gone client) stop the copy silently like BrokenPipe handling.
func streamCopy(w http.ResponseWriter, f *os.File, n int64) int64 {
	if n <= 0 {
		return 0
	}
	buf := getCopyBuf()
	defer putCopyBuf(buf)
	sent, _ := io.CopyBuffer(w, io.LimitReader(f, n), buf)
	if sent < 0 {
		return 0
	}
	return sent
}
