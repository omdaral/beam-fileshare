package beamcore

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var boundaryRe = regexp.MustCompile(`boundary=([^;,\s]+)`)
var filenameQuotedRe = regexp.MustCompile(`filename="([^"]*)"`)
var filenameBareRe = regexp.MustCompile(`filename=([^;\r\n]+)`)

func handleUploadMultipart(w http.ResponseWriter, r *http.Request) {
	ctype := r.Header.Get("Content-Type")
	length := r.ContentLength
	if !strings.Contains(ctype, "multipart/form-data") {
		fail(w, r, 400, "up_use_chunked")
		return
	}
	if length <= 0 || length > int64(multipartMax) {
		fail(w, r, 413, "up_multipart_limit")
		return
	}
	m := boundaryRe.FindStringSubmatch(ctype)
	if m == nil {
		fail(w, r, 400, "up_multipart_boundary")
		return
	}
	boundary := strings.Trim(m[1], "\"")
	if len(boundary) < 4 || len(boundary) > 200 {
		fail(w, r, 400, "up_multipart_bad")
		return
	}
	// Streaming path: never buffer the whole body in RAM (old code did
	// body=append up to 128MB + bytes.Split under a global lock, which
	// blocked parallel uploads and spiked GC). Each file streams straight
	// to its reserved dest; the global lock is held only for name
	// reservation (microseconds), never across the network read.
	saved, errMsg, errStatus := saveMultipartStream(w, r, boundary, reqLang(r))
	if errStatus != 0 {
		// saveMultipartStream already wrote nothing; report JSON.
		if errStatus == 413 {
			fail(w, r, errStatus, errMsg)
			return
		}
		sendJSON(w, r, errStatus, map[string]interface{}{"error": errMsg})
		return
	}
	if len(saved) == 0 {
		fail(w, r, 400, "up_multipart_empty")
		return
	}
	invalidateListCaches()
	sendJSON(w, r, 200, map[string]interface{}{"ok": true, "saved": saved})
}

// saveMultipartStream streams a multipart body file-by-file to disk.
// Returns (saved basenames, errMsg key, http status or 0 on success).
func saveMultipartStream(w http.ResponseWriter, r *http.Request, boundary, lang string) ([]string, string, int) {
	max := maxBytes()
	r.Body = http.MaxBytesReader(w, r.Body, int64(multipartMax)+int64(multipartSlack))
	defer r.Body.Close()
	mr := multipart.NewReader(r.Body, boundary)
	saved := []string{}
	buf := getCopyBuf()
	defer putCopyBuf(buf)
	var total int64
	for {
		if r.Context().Err() != nil {
			break
		}
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			if len(saved) == 0 {
				return nil, tr(lang, "up_multipart_bad"), 400
			}
			break
		}
		fn := part.FileName()
		if fn == "" {
			_, _ = io.Copy(io.Discard, io.LimitReader(part, 1<<20))
			continue
		}
		base := fn
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		if i := strings.LastIndex(base, "\\"); i >= 0 {
			base = base[i+1:]
		}
		name := safeFilename(base)
		if name == "" || strings.HasPrefix(name, ".") {
			_, _ = io.Copy(io.Discard, io.LimitReader(part, 1<<20))
			continue
		}
		// Reserve dest under the naming lock only (never across I/O).
		uploadLock.Lock()
		dest := uniquePath(SharedDir, name)
		f, cerr := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		uploadLock.Unlock()
		if cerr != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(part, 1<<20))
			continue
		}
		// Per-file cap: max+1 to detect overflow without trusting headers.
		written, werr := io.CopyBuffer(f, io.LimitReader(part, max+1), buf)
		cerr = f.Close()
		total += written
		if werr != nil || cerr != nil {
			_ = os.Remove(dest)
			return nil, tr(lang, "up_multipart_save"), 500
		}
		if written == 0 || written > max {
			_ = os.Remove(dest)
			if written > max {
				return nil, tr(lang, "up_too_big"), 400
			}
			continue
		}
		if total > int64(multipartMax)+int64(multipartSlack) {
			_ = os.Remove(dest)
			return nil, "up_multipart_limit", 413
		}
		saved = append(saved, filepath.Base(dest))
		writeLog("multipart", "upload", saved[len(saved)-1]+" ("+strconv.FormatInt(written, 10)+"b)")
	}
	return saved, "", 0
}
func saveMultipart(body []byte, boundary string, lang string) ([]string, string, int) {
	max := maxBytes()
	saved := []string{}
	delim := []byte("--" + boundary)
	parts := bytes.Split(body, delim)
	for _, part := range parts {
		if len(bytes.TrimSpace(part)) == 0 {
			continue
		}
		if string(bytes.TrimSpace(part)) == "--" {
			continue
		}
		idx := bytes.Index(part, []byte("\r\n\r\n"))
		if idx < 0 {
			continue
		}
		rawHeaders := string(part[:idx])
		data := part[idx+4:]
		data = bytesTrimRightCRLF(data)
		var fn *string
		if m := filenameQuotedRe.FindStringSubmatch(rawHeaders); m != nil {
			s := m[1]
			fn = &s
		} else if m := filenameBareRe.FindStringSubmatch(rawHeaders); m != nil {
			s := strings.Trim(strings.TrimSpace(m[1]), "\"")
			fn = &s
		}
		if fn == nil {
			continue
		}
		base := *fn
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		if i := strings.LastIndex(base, "\\"); i >= 0 {
			base = base[i+1:]
		}
		name := safeFilename(base)
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		if len(data) == 0 || int64(len(data)) > max {
			return nil, tr(lang, "up_too_big"), 400
		}
		uploadLock.Lock()
		dest := uniquePath(SharedDir, name)
		werr := os.WriteFile(dest, data, 0644)
		uploadLock.Unlock()
		if werr != nil {
			return nil, tr(lang, "up_multipart_save"), 500
		}
		saved = append(saved, filepath.Base(dest))
		writeLog("multipart", "upload", saved[len(saved)-1]+" ("+strconv.FormatInt(int64(len(data)), 10)+"b)")
	}
	return saved, "", 0
}

func bytesTrimRightCRLF(b []byte) []byte {
	if len(b) >= 2 && b[len(b)-2] == '\r' && b[len(b)-1] == '\n' {
		b = b[:len(b)-2]
	}
	if len(b) >= 2 && b[len(b)-2] == '-' && b[len(b)-1] == '-' {
		b = b[:len(b)-2]
		for len(b) >= 2 && b[len(b)-2] == '\r' && b[len(b)-1] == '\n' {
			b = b[:len(b)-2]
		}
	}
	return b
}
