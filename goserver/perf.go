package beamcore

// Shared performance helpers: pooled copy buffers, conditional-GET helpers,
// download concurrency bound, and tiny TTL caches for hot read paths.
//
// Policy: ZIP-only. Beam generates STORE (uncompressed) zips only so every
// stock OS/phone can open them without extra codecs. No gzip/br for HTTP,
// no deflate-on-generate. Reading foreign zips (incl. deflated entries)
// still works — this file changes nothing about extraction.

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// pooled copy buffers (1MB default, tunable via BEAM_COPY_BUF)
// ---------------------------------------------------------------------------

var copyBufPool sync.Pool

func getCopyBuf() []byte {
	if v := copyBufPool.Get(); v != nil {
		if b, ok := v.([]byte); ok && len(b) >= 32*1024 {
			// Clamp to the live tunable: longer pooled bufs are sliced,
			// shorter ones are discarded to avoid tiny writes.
			if len(b) >= copyBufSize {
				return b[:copyBufSize]
			}
			if cap(b) >= copyBufSize {
				return b[:copyBufSize]
			}
		}
	}
	return make([]byte, copyBufSize)
}

func putCopyBuf(b []byte) {
	// Keep the pool from hoarding odd sizes across BEAM_COPY_BUF changes.
	if cap(b) < 32*1024 || cap(b) > 8*1024*1024 {
		return
	}
	copyBufPool.Put(b)
}

// ---------------------------------------------------------------------------
// download concurrency bound (memory safety, not throttling)
// ---------------------------------------------------------------------------

var (
	downloadSemMu sync.Mutex
	downloadSem   = make(chan struct{}, 32)
)

// downloadMaxConc is tunable via BEAM_DOWNLOAD_CONC (8..256).
var downloadMaxConc = 32

func setDownloadSemSize(n int) {
	if n < 8 {
		n = 8
	}
	if n > 256 {
		n = 256
	}
	downloadSemMu.Lock()
	old := downloadSem
	// Preserve live permits: move up to min(queued, n) into the new
	// channel so resizing never leaks slots or lifts the bound.
	next := make(chan struct{}, n)
	drained := 0
	for drained < n {
		select {
		case v := <-old:
			select {
			case next <- v:
				drained++
			default:
			}
		default:
			drained = n
		}
	}
	downloadSem = next
	downloadMaxConc = n
	downloadSemMu.Unlock()
}

func currentDownloadSem() chan struct{} {
	downloadSemMu.Lock()
	defer downloadSemMu.Unlock()
	return downloadSem
}

// acquireDownloadSlot queues (never rejects) but bounds concurrent bodies so
// 1MB buffers + zip writers can't OOM the box. Returns false when the client
// went away while queued.
func acquireDownloadSlot(r *http.Request) bool {
	sem := currentDownloadSem()
	select {
	case sem <- struct{}{}:
		return true
	case <-r.Context().Done():
		return false
	}
}

func releaseDownloadSlot() {
	sem := currentDownloadSem()
	select {
	case <-sem:
	default:
	}
}

// ---------------------------------------------------------------------------
// conditional GET helpers (ETag + Last-Modified)
// ---------------------------------------------------------------------------

func etagMatches(r *http.Request, etag string) bool {
	inm := strings.TrimSpace(r.Header.Get("If-None-Match"))
	if inm == "" {
		return false
	}
	if inm == "*" {
		return true
	}
	// Multiple tags: `If-None-Match: "a", "b"`.
	for _, tag := range strings.Split(inm, ",") {
		tag = strings.TrimSpace(tag)
		if tag == etag {
			return true
		}
		// Weak comparison: W/"etag" matches "etag" for GET.
		if strings.HasPrefix(tag, "W/") {
			inner := strings.TrimSpace(strings.TrimPrefix(tag, "W/"))
			if inner == etag {
				return true
			}
		}
	}
	return false
}

func modifiedSinceAllowsNotModified(r *http.Request, lastMod string) bool {
	ims := strings.TrimSpace(r.Header.Get("If-Modified-Since"))
	if ims == "" || strings.Contains(r.Header.Get("If-None-Match"), "*") || r.Header.Get("If-None-Match") != "" {
		// Per RFC 7232 §3.3: If-None-Match takes precedence; only use
		// If-Modified-Since when If-None-Match is absent.
		return false
	}
	if lastMod == "" {
		return false
	}
	imsT, err1 := http.ParseTime(ims)
	lmT, err2 := http.ParseTime(lastMod)
	if err1 != nil || err2 != nil {
		return false
	}
	return !lmT.After(imsT)
}

// notModified writes a 304 with validators preserved (caches revalidate).
func notModified(w http.ResponseWriter, etag, lastMod string) {
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	if lastMod != "" {
		w.Header().Set("Last-Modified", lastMod)
	}
	w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
	w.WriteHeader(http.StatusNotModified)
}

// ifRangeAllowsPartial reports whether a Range request may stay partial.
// When If-Range carries a stale validator, the client must get the full
// representation (200) instead of a mismatched 206.
func ifRangeAllowsPartial(r *http.Request, etag, lastMod string) bool {
	ir := strings.TrimSpace(r.Header.Get("If-Range"))
	if ir == "" {
		return true
	}
	if strings.HasPrefix(ir, "\"") || strings.HasPrefix(ir, "W/") {
		return ir == etag || etagMatches(&http.Request{Header: http.Header{"If-None-Match": []string{ir}}}, etag)
	}
	// Otherwise it's an HTTP-date: match against Last-Modified.
	if lastMod == "" {
		return false
	}
	irT, err1 := http.ParseTime(ir)
	lmT, err2 := http.ParseTime(lastMod)
	if err1 != nil || err2 != nil {
		return false
	}
	return irT.Equal(lmT) || irT.After(lmT)
}

// ---------------------------------------------------------------------------
// tiny TTL caches for hot read paths (all invalidated on mutation + tests)
// ---------------------------------------------------------------------------

const (
	listCacheTTL  = 2.0 // seconds for /api/files + /download_zip pre-scans
	shareStatTTL  = 3.0
	tempSizeTTL   = 5.0
	absListTTL    = 2.0
	presenceSweep = 60.0 // min seconds between presence-triggered sweeps
)

type listCacheEntry struct {
	at        float64
	entries   []sharedEntry
	truncated bool
	deepUnder map[string]bool
}

var (
	listCacheMu sync.Mutex
	listCache   = map[string]*listCacheEntry{}
)

func listCacheKey(root string, limit int) string {
	return root + "|" + strconv.Itoa(limit)
}

func getCachedWalkShared(limit int) ([]sharedEntry, bool, map[string]bool, bool) {
	key := listCacheKey(SharedDir, limit)
	now := nowUnix()
	listCacheMu.Lock()
	defer listCacheMu.Unlock()
	e, ok := listCache[key]
	if !ok || now-e.at > listCacheTTL {
		return nil, false, nil, false
	}
	out := append([]sharedEntry{}, e.entries...)
	deep := map[string]bool{}
	for k, v := range e.deepUnder {
		deep[k] = v
	}
	return out, e.truncated, deep, true
}

func setCachedWalkShared(limit int, entries []sharedEntry, truncated bool, deepUnder map[string]bool) {
	key := listCacheKey(SharedDir, limit)
	deep := map[string]bool{}
	for k, v := range deepUnder {
		deep[k] = v
	}
	listCacheMu.Lock()
	listCache[key] = &listCacheEntry{
		at: nowUnix(), entries: append([]sharedEntry{}, entries...),
		truncated: truncated, deepUnder: deep,
	}
	listCacheMu.Unlock()
}

func invalidateListCaches() {
	listCacheMu.Lock()
	listCache = map[string]*listCacheEntry{}
	listCacheMu.Unlock()
	absCacheMu.Lock()
	absCache = map[string]*absCacheEntry{}
	absCacheMu.Unlock()
	shareStatMu.Lock()
	shareStatCache = map[string]*shareStatEntry{}
	shareStatMu.Unlock()
	tempSizeMu.Lock()
	tempSizeCache = map[string]*tempSizeEntry{}
	tempSizeMu.Unlock()
}

// --- abs dir (host shares + /r/<dir> zip) ---

type absCacheEntry struct {
	at        float64
	rootMtime int64
	entries   []absEntry
	truncated bool
}

var (
	absCacheMu sync.Mutex
	absCache   = map[string]*absCacheEntry{}
)

func getCachedWalkAbs(root string, limit int) ([]absEntry, bool, bool) {
	now := nowUnix()
	var rootMtime int64
	if st, err := os.Stat(root); err == nil {
		rootMtime = st.ModTime().UnixNano()
	}
	key := root + "|" + strconv.Itoa(limit)
	absCacheMu.Lock()
	defer absCacheMu.Unlock()
	e, ok := absCache[key]
	if !ok || now-e.at > absListTTL || e.rootMtime != rootMtime {
		return nil, false, false
	}
	return append([]absEntry{}, e.entries...), e.truncated, true
}

func setCachedWalkAbs(root string, limit int, entries []absEntry, truncated bool) {
	var rootMtime int64
	if st, err := os.Stat(root); err == nil {
		rootMtime = st.ModTime().UnixNano()
	}
	key := root + "|" + strconv.Itoa(limit)
	absCacheMu.Lock()
	absCache[key] = &absCacheEntry{
		at: nowUnix(), rootMtime: rootMtime,
		entries: append([]absEntry{}, entries...), truncated: truncated,
	}
	absCacheMu.Unlock()
}

// --- per-share stat (available + mtime) ---

type shareStatEntry struct {
	at        float64
	available bool
	mtime     int64
	hostPath  string
}

var (
	shareStatMu    sync.Mutex
	shareStatCache = map[string]*shareStatEntry{}
)

func getCachedShareStat(id, hostPath string) (bool, int64, bool) {
	now := nowUnix()
	shareStatMu.Lock()
	defer shareStatMu.Unlock()
	e, ok := shareStatCache[id]
	if !ok || now-e.at > shareStatTTL || e.hostPath != hostPath {
		return false, 0, false
	}
	return e.available, e.mtime, true
}

func setCachedShareStat(id, hostPath string, available bool, mtime int64) {
	shareStatMu.Lock()
	shareStatCache[id] = &shareStatEntry{
		at: nowUnix(), available: available, mtime: mtime, hostPath: hostPath,
	}
	shareStatMu.Unlock()
}

func invalidateShareStat(id string) {
	shareStatMu.Lock()
	delete(shareStatCache, id)
	shareStatMu.Unlock()
}

// --- temp dir size (GET /api/temp walks the whole tree) ---

type tempSizeEntry struct {
	at   float64
	size int64
}

var (
	tempSizeMu    sync.Mutex
	tempSizeCache = map[string]*tempSizeEntry{}
)

func getCachedTempSize(dir string) (int64, bool) {
	now := nowUnix()
	tempSizeMu.Lock()
	defer tempSizeMu.Unlock()
	if e, ok := tempSizeCache[dir]; ok && now-e.at < tempSizeTTL {
		return e.size, true
	}
	return 0, false
}

func setCachedTempSize(dir string, size int64) {
	tempSizeMu.Lock()
	tempSizeCache[dir] = &tempSizeEntry{at: nowUnix(), size: size}
	tempSizeMu.Unlock()
}

// --- presence-triggered sweep throttle (disk-storm guard) ---

var (
	sweepMu sync.Mutex
	sweepAt float64
)

func shouldSweepNow() bool {
	now := nowUnix()
	sweepMu.Lock()
	defer sweepMu.Unlock()
	if now-sweepAt < presenceSweep {
		return false
	}
	sweepAt = now
	return true
}

func resetPerfCachesForTests() {
	invalidateListCaches()
	sweepMu.Lock()
	sweepAt = 0
	sweepMu.Unlock()
}
