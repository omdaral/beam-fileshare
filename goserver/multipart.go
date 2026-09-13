package beamcore

import (
	"bytes"
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
	// Cap body: single multipart request is capped at 128MB by design
	// (chunked protocol is the path for big files).
	r.Body = http.MaxBytesReader(w, r.Body, int64(multipartMax)+int64(multipartSlack))
	body := make([]byte, 0, length)
	if r.Body != nil {
		defer r.Body.Close()
		buf := make([]byte, copyBufSize)
		var total int64
		for {
			if r.Context().Err() != nil {
				break
			}
			nr, err := r.Body.Read(buf)
			if nr > 0 {
				total += int64(nr)
				if total > int64(multipartMax)+int64(multipartSlack) {
					fail(w, r, 413, "up_multipart_limit")
					return
				}
				body = append(body, buf[:nr]...)
			}
			if err != nil {
				break
			}
		}
	}
	saved, errMsg, errStatus := saveMultipart(body, boundary, reqLang(r))
	if errStatus != 0 {
		sendJSON(w, r, errStatus, map[string]interface{}{"error": errMsg})
		return
	}
	if len(saved) == 0 {
		fail(w, r, 400, "up_multipart_empty")
		return
	}
	sendJSON(w, r, 200, map[string]interface{}{"ok": true, "saved": saved})
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
