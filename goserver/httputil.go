package beamcore

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// clientIP returns the peer IP without port.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// isLoopback reports localhost peers (variable so tests can simulate guests).
// Whole 127/8 + ::1 are loopback per RFC1122 (net.IP.IsLoopback).
var isLoopback = func(ip string) bool {
	ip = strings.TrimSpace(ip)
	if i := strings.LastIndex(ip, "%"); i >= 0 {
		ip = ip[:i] // strip zone: ::1%lo
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback()
}

// sendJSON writes a JSON object with UTF-8 body (no HTML escaping).
func sendJSON(w http.ResponseWriter, r *http.Request, status int, obj interface{}) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(obj); err != nil {
		status = 500
		buf.Reset()
		buf.WriteString(`{"error":"تعذر الترميز"}` + "\n")
	}
	body := bytes.TrimRight(buf.Bytes(), "\n")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Never cache API payloads (status/config/sessions leak topology).
	if r != nil && r.URL != nil {
		p := r.URL.Path
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/upload") ||
			p == "/files" || p == "/file_hash" {
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
		}
	}
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(body)), 10))
	w.WriteHeader(status)
	if r.Method == "HEAD" {
		return
	}
	_, _ = w.Write(body)
}

// sendEmpty writes a status with zero body (mirrors _send_empty).
func sendEmpty(w http.ResponseWriter, r *http.Request, status int) {
	w.Header().Set("Content-Length", "0")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
}

// readJSONBody decodes a JSON object body (empty/invalid -> empty map).
func readJSONBody(r *http.Request, maxBytes int64) map[string]interface{} {
	out := map[string]interface{}{}
	if r.Body == nil {
		return out
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
	if err != nil || int64(len(body)) > maxBytes {
		return out
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return out
	}
	_ = json.Unmarshal(body, &out)
	if out == nil {
		out = map[string]interface{}{}
	}
	return out
}
func jStr(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
func jInt(m map[string]interface{}, key string, def int64) int64 {
	if v, ok := m[key]; ok {
		if i, valid := asInt(v); valid {
			return int64(i)
		}
	}
	return def
}
func jBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
func jHas(m map[string]interface{}, key string) bool {
	_, ok := m[key]
	return ok
}
func nowUnix() float64 {
	return float64(nowNano()) / 1e9
}
func nowNano() int64 {
	return time.Now().UnixNano()
}
