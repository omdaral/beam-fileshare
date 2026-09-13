package beamcore

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// safeFilename blocks path traversal, like fileshare.safe_filename.
// Returns "" when invalid. (Dotfile rules are enforced by callers,
// mirroring fileshare.py exactly.)
func safeFilename(name string) string {
	if name == "" {
		return ""
	}
	name = strings.ReplaceAll(name, "\\", "/")
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return ""
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return ""
	}
	root := name
	if idx := strings.Index(name, "."); idx >= 0 {
		root = name[:idx]
	}
	upper := strings.ToUpper(root)
	if upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL" {
		return ""
	}
	if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) {
		d := upper[3]
		if d >= '0' && d <= '9' {
			return ""
		}
	}
	return name
}

// safeRelPath validates a share-relative path like "Docs/2026/a.pdf".
// Every segment must pass the safeFilename rules and must not be a
// dotfile/dotdir; depth and total length are capped. Returns "" when
// invalid. Use for any file or folder inside Shared/.
func safeRelPath(name string) string {
	if name == "" {
		return ""
	}
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimSpace(name)
	if name == "" || strings.HasPrefix(name, "/") {
		return ""
	}
	if len(name) > maxRelLen {
		return ""
	}
	segs := strings.Split(name, "/")
	if len(segs) > maxRelDepth {
		return ""
	}
	clean := make([]string, 0, len(segs))
	for _, s := range segs {
		s = strings.TrimSpace(s)
		if s == "" {
			return ""
		}
		if safeFilename(s) != s {
			return ""
		}
		if strings.HasPrefix(s, ".") {
			return ""
		}
		clean = append(clean, s)
	}
	return strings.Join(clean, "/")
}

// splitParent splits a validated rel path into dir and base ("a/b/c" ->
// "a/b", "c"; "c" -> "", "c").
func splitParent(rel string) (string, string) {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[:i], rel[i+1:]
	}
	return "", rel
}

// uniquePath returns a non-existing path, appending " (i)" like fileshare.
// Bounded at 10000 attempts, then falls back to a timestamp suffix so a
// malicious dir with a million collisions can't hang the server.
func uniquePath(directory, name string) string {
	candidate := filepath.Join(directory, name)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; i <= uniqueTries; i++ {
		candidate = filepath.Join(directory, base+" ("+strconv.FormatInt(int64(i), 10)+")"+ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return filepath.Join(directory, base+" ("+strconv.FormatInt(nowNano(), 10)+")"+ext)
}

// percentEncode mimics urllib.parse.quote(name) for attachment filenames.
// Kept (not url.PathEscape): verified stricter — also escapes + = @,
// which PathEscape leaves raw inside path segments. See TestStdlibDivergence.
func percentEncode(s string) string {
	const unreserved = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_.-~"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if strings.IndexByte(unreserved, c) >= 0 {
			b.WriteByte(c)
		} else {
			const hexd = "0123456789ABCDEF"
			b.WriteByte('%')
			b.WriteByte(hexd[c>>4])
			b.WriteByte(hexd[c&15])
		}
	}
	return b.String()
}
