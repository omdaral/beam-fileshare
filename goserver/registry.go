package beamcore

// Registry-based sharing backend (replaces the deleted direct-share).
//
// Model: the owner indexes arbitrary host-disk files/dirs (POST /api/share,
// owner-only) and guests offer files from their own devices
// (POST /api/share/guest). The registry is memory-only and mutex-guarded.
//
// Beam-Temp is the single managed store (user's simpler model):
//   - Every completed upload (owner or guest browser upload) lands as a REAL
//     file under <temp>/<original-rel-path> with full name/size/mtime.
//   - Anything present in <temp> is directly downloadable; removing a share
//     whose bytes live inside <temp> deletes the real file (pruning empty
//     parents), so it never reappears.
//   - Host absolute shares pointing OUTSIDE <temp> are index-only: unshare
//     just drops the index and keeps the disk file.
//   - On boot RestoreTempShares() re-registers loose temp files (original
//     names, fresh ids) and migrates legacy hidden .shares/<id> orphans into
//     <temp>/restored-<short> real files, so no "restored-xxx" ghost entries
//     with lost names remain. GET /api/shares also live-syncs <temp> so a
//     manual copy appears without a restart.
//
// Temp layout (all under the single auto-created temp dir):
//   <temp>/<rel-path>                            real managed files (visible)
//   <temp>/.uploads/<session>/data.part+meta.json  staging (legacy + guest)
//   <temp>/.shares/<shareID>                     LEGACY retained guest bytes
//     (new uploads no longer land here; orphans are migrated to real files)
// Guest staging reuses the stock upload engine (.uploads/<session>), so the
// bytes land inside the temp dir; on /upload_complete they are MOVED to
// <temp>/<name> (served even after the owner leaves) instead of being
// hidden under .shares/<shareID>.
//
// Changing temp_dir MOVES nothing: old bytes stay, new sessions/temp go to
// the new dir from that moment on.

import (
	"archive/zip"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// tables (memory-only)
// ---------------------------------------------------------------------------

type shareEntry struct {
	ID        string
	Name      string
	Kind      string // "file" | "dir"
	Size      int64
	Owner     string
	OwnerID   string
	Source    string // "host" | "guest"
	HostPath  string // absolute resolved path (host only)
	SessionID string // upload session (guest only)
	Completed bool   // guest: upload finished + retained
	Created   float64
}

type presenceEntry struct {
	Name     string
	LastSeen float64
}

var (
	regMu      sync.Mutex
	shares     = map[string]*shareEntry{}
	shareOrder = []string{}
	// sessToShare maps a guest upload session id -> share id.
	sessToShare = map[string]string{}
	presence    = map[string]*presenceEntry{}
)

// resetRegistryState clears all tables (tests only — callers are sequential).
func resetRegistryState() {
	regMu.Lock()
	shares = map[string]*shareEntry{}
	shareOrder = []string{}
	sessToShare = map[string]string{}
	presence = map[string]*presenceEntry{}
	regMu.Unlock()
	relayMu.Lock()
	relayJobs = map[string]*relayJob{}
	relayPending = map[string][]*relayJob{}
	relayWaiters = map[string][]chan struct{}{}
	relayMu.Unlock()
	resetPerfCachesForTests()
}

// ---------------------------------------------------------------------------
// temp helpers
// ---------------------------------------------------------------------------

// tempBase is the live temp dir; SharedDir stays as a deprecated alias that
// always points at the same path (legacy helpers still read it).
func tempBase() string {
	if TempDir != "" {
		return TempDir
	}
	return SharedDir
}

func sharesDir() string {
	return filepath.Join(tempBase(), ".shares")
}

func retainedPath(shareID string) string {
	return filepath.Join(sharesDir(), shareID)
}

// isInsideTemp reports whether an absolute path lives inside the managed
// temp dir. Managed bytes: present => downloadable, unshare => delete.
// Outside paths (host absolute shares): index-only, unshare keeps the file.
func isInsideTemp(absPath string) bool {
	base := tempBase()
	if base == "" || absPath == "" {
		return false
	}
	cleanBase := filepath.Clean(base)
	cleanP := filepath.Clean(absPath)
	if cleanP == cleanBase {
		return true
	}
	sep := string(os.PathSeparator)
	// Resolve symlinks best-effort for the base (cheap, cached via sharedBase).
	if rb, err := filepath.EvalSymlinks(cleanBase); err == nil {
		cleanBase = rb
	}
	if rp, err := filepath.EvalSymlinks(cleanP); err == nil {
		// EvalSymlinks fails for not-yet-existing dest: fall back to cleanP.
		cleanP = rp
	}
	if cleanP == cleanBase {
		return true
	}
	return strings.HasPrefix(cleanP, cleanBase+sep)
}

// guestRealPath returns the on-disk bytes for a guest entry, preferring the
// managed real file (HostPath, new model) and falling back to the legacy
// hidden retained path (.shares/<id>).
func guestRealPath(e *shareEntry) string {
	if e == nil {
		return ""
	}
	if e.HostPath != "" {
		return e.HostPath
	}
	return retainedPath(e.ID)
}

// tempDirSize sums every byte under dir (dot dirs included).
func tempDirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

// ---------------------------------------------------------------------------
// ids
// ---------------------------------------------------------------------------

func randHex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		for i := range b {
			b[i] = byte(nowNano() >> uint(i%8))
		}
	}
	return hex.EncodeToString(b)
}

func newShareID() string {
	regMu.Lock()
	defer regMu.Unlock()
	for {
		id := randHex(12)
		if _, dup := shares[id]; !dup {
			return id
		}
	}
}

func newGuestSessionID() string {
	regMu.Lock()
	defer regMu.Unlock()
	for {
		// 32 hex chars: satisfies uuidRe so the stock upload engine
		// (upload_init/chunk/piece/complete/status) accepts the session.
		id := randHex(16)
		if _, dup := sessToShare[id]; dup {
			continue
		}
		if loadMeta(id) != nil {
			continue
		}
		return id
	}
}

// guestShareForSession returns the share id linked to an upload session.
func guestShareForSession(uid string) (string, bool) {
	regMu.Lock()
	defer regMu.Unlock()
	id, ok := sessToShare[strings.ToLower(uid)]
	return id, ok
}

// ---------------------------------------------------------------------------
// blocklist (reimplemented after directshare.go was deleted)
// ---------------------------------------------------------------------------

var unixBlockedRoots = []string{
	"/etc", "/proc", "/sys", "/dev", "/run",
	"/boot", "/bin", "/sbin", "/lib", "/lib64", "/usr",
}

// shareBlocked reports system roots that must never be shared or used as
// the temp dir: /etc /proc /sys /dev /run (+ siblings) and C:\Windows etc.
// Works on any OS (Windows patterns are matched textually) so unit tests
// hold everywhere.
func shareBlocked(p string) bool {
	clean := filepath.Clean(strings.TrimSpace(p))
	if clean == "" {
		return false
	}
	// Windows absolute pattern, any drive: C:\... / C:/...
	w := strings.ReplaceAll(clean, "\\", "/")
	up := strings.ToUpper(w)
	if len(up) >= 3 && up[1] == ':' && up[2] == '/' &&
		up[0] >= 'A' && up[0] <= 'Z' {
		rest := up[3:]
		if rest == "" {
			return true // bare drive root
		}
		for _, sys := range []string{"WINDOWS", "PROGRAM FILES",
			"PROGRAM FILES (X86)", "SYSTEM VOLUME INFORMATION",
			"$RECYCLE.BIN", "WINDOWS/SYSTEM32"} {
			if rest == sys || strings.HasPrefix(rest, sys+"/") {
				return true
			}
		}
		return false
	}
	if !filepath.IsAbs(clean) {
		return false
	}
	if clean == "/" {
		return true
	}
	for _, root := range unixBlockedRoots {
		if clean == root || strings.HasPrefix(clean, root+"/") {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// presence
// ---------------------------------------------------------------------------

func presenceAlive(ownerID string) bool {
	if ownerID == "" {
		return false
	}
	regMu.Lock()
	defer regMu.Unlock()
	pe, ok := presence[ownerID]
	if !ok {
		return false
	}
	return nowUnix()-pe.LastSeen <= float64(presenceTTLsec)
}

// ---------------------------------------------------------------------------
// JSON shapes (FROZEN contract)
// ---------------------------------------------------------------------------

func shareFail(w http.ResponseWriter, r *http.Request, status int, key string) {
	sendJSON(w, r, status, map[string]interface{}{
		"error": tr(reqLang(r), key), "code": key})
}

func shareAvailable(e *shareEntry) bool {
	if e.Source == "host" {
		// Fast path: cached stat (3s TTL) — /api/shares polls every 5s.
		if avail, _, ok := getCachedShareStat(e.ID, e.HostPath); ok {
			return avail
		}
		st, err := os.Stat(e.HostPath)
		avail := err == nil && (st.IsDir() || st.Mode().IsRegular())
		var mt int64
		if err == nil {
			mt = st.ModTime().Unix()
		} else {
			mt = int64(e.Created)
		}
		setCachedShareStat(e.ID, e.HostPath, avail, mt)
		return avail
	}
	// Guest: once the upload is complete the bytes live on this server as a
	// REAL managed file (<temp>/<name>) — they stay downloadable with full
	// name/size/mtime even if the owner closes the tab. Presence only gates
	// the *incomplete* case where live relay bytes are needed. Legacy
	// entries without HostPath fall back to .shares/<id>.
	if !e.Completed {
		return false
	}
	rp := guestRealPath(e)
	st, err := os.Stat(rp)
	if err != nil || st.Size() <= 0 {
		return false
	}
	if e.Size > 0 && st.Size() < e.Size {
		// Partial prefix alone is not "available" as a complete
		// file — but it is still served via Range/relay when the owner is
		// around. Report available only when the full bytes are on disk,
		// otherwise fall through to the presence-gated relay path.
		if e.OwnerID != "" && !presenceAlive(e.OwnerID) {
			// Owner gone + incomplete bytes = still show as available?
			// No: keep listed but mark unavailable only when nothing usable.
			// Since prefix exists, allow direct Range serving.
			return true
		}
		return true
	}
	return true
}

func shareMtime(e *shareEntry) int64 {
	if e.Source == "host" {
		if avail, mt, ok := getCachedShareStat(e.ID, e.HostPath); ok {
			_ = avail
			return mt
		}
		if st, err := os.Stat(e.HostPath); err == nil {
			mt := st.ModTime().Unix()
			setCachedShareStat(e.ID, e.HostPath, st.IsDir() || st.Mode().IsRegular(), mt)
			return mt
		}
	}
	if st, err := os.Stat(guestRealPath(e)); err == nil {
		return st.ModTime().Unix()
	}
	return int64(e.Created)
}

func shareJSON(e *shareEntry) map[string]interface{} {
	return map[string]interface{}{
		"id": e.ID, "name": e.Name, "kind": e.Kind, "size": e.Size,
		"owner": e.Owner, "owner_id": e.OwnerID, "source": e.Source,
		"available": shareAvailable(e), "mtime": shareMtime(e),
	}
}

// syncTempLive makes <temp> the source of truth for managed files:
//   - Any top-level non-dot file/dir present on disk but missing from the
//     registry is auto-registered (original name, fresh id) so a manual copy
//     via the file manager appears without a restart.
//   - Any live entry whose managed HostPath vanished is pruned, so an
//     external delete disappears instead of lingering as unavailable.
// Incomplete guest sessions (no HostPath yet) are never pruned here.
func syncTempLive() {
	base := tempBase()
	if base == "" {
		return
	}
	// Snapshot live paths under lock (no disk I/O while holding regMu).
	regMu.Lock()
	byPath := make(map[string]string, len(shares))
	coveredTop := map[string]bool{}
	liveCount := len(shares)
	type liveHP struct {
		id string
		hp string
	}
	var managed []liveHP
	for id, e := range shares {
		if e.HostPath != "" && isInsideTempLocked(e.HostPath, base) {
			byPath[filepath.Clean(e.HostPath)] = id
			managed = append(managed, liveHP{id: id, hp: e.HostPath})
			rel, err := filepath.Rel(base, filepath.Clean(e.HostPath))
			if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
				top := rel
				if i := strings.Index(rel, string(os.PathSeparator)); i >= 0 {
					top = rel[:i]
				} else if i := strings.Index(rel, "/"); i >= 0 {
					top = rel[:i]
				}
				if top != "" {
					coveredTop[top] = true
				}
			}
		} else if e.Source == "guest" && e.HostPath == "" {
			byPath[filepath.Clean(retainedPath(e.ID))] = id
		}
	}
	regMu.Unlock()

	rd, err := os.ReadDir(base)
	if err != nil {
		return
	}
	type cand struct {
		name string
		full string
		kind string
		size int64
	}
	var cands []cand
	for _, de := range rd {
		nm := de.Name()
		if strings.HasPrefix(nm, ".") {
			continue
		}
		full := filepath.Join(base, nm)
		if shareBlocked(full) {
			continue
		}
		st, err := os.Lstat(full)
		if err != nil {
			continue
		}
		if st.Mode()&os.ModeSymlink != 0 {
			continue
		}
		clean := filepath.Clean(full)
		if _, known := byPath[clean]; known {
			continue
		}
		if coveredTop[nm] {
			continue
		}
		var kind string
		var size int64
		if st.IsDir() {
			n, total := dirSizeAbs(full)
			if n == 0 {
				continue
			}
			kind, size = "dir", total
		} else if st.Mode().IsRegular() {
			if st.Size() == 0 {
				continue
			}
			kind, size = "file", st.Size()
		} else {
			continue
		}
		cands = append(cands, cand{name: nm, full: full, kind: kind, size: size})
		if liveCount+len(cands) >= shareMax {
			break
		}
	}
	// Prune check outside the lock (disk I/O).
	pruneSet := map[string]bool{}
	for _, m := range managed {
		if _, err := os.Lstat(m.hp); err != nil {
			pruneSet[m.id] = true
		}
	}
	if len(cands) == 0 && len(pruneSet) == 0 {
		return
	}
	regMu.Lock()
	var toAdd []*shareEntry
	for _, c := range cands {
		if len(shares)+len(toAdd) >= shareMax {
			break
		}
		if _, known := byPath[filepath.Clean(c.full)]; known {
			continue
		}
		toAdd = append(toAdd, &shareEntry{
			Name: c.name, Kind: c.kind, Size: c.size,
			Owner: "host", OwnerID: "host", Source: "host",
			HostPath: c.full, Created: nowUnix(),
		})
	}
	var prune []string
	for id := range pruneSet {
		if e, ok := shares[id]; ok && e.HostPath != "" && isInsideTempLocked(e.HostPath, base) {
			// Re-check under lock to avoid racing a just-completed upload.
			prune = append(prune, id)
		}
	}
	// Final Lstat re-check for prune candidates while holding lock is cheap
	// for the (usually empty) prune list; skip when nothing to prune.
	if len(prune) > 0 {
		kept := prune[:0]
		for _, id := range prune {
			if e, ok := shares[id]; ok {
				if _, err := os.Lstat(e.HostPath); err == nil {
					continue
				}
				kept = append(kept, id)
			}
		}
		prune = kept
	}
	for _, e := range toAdd {
		e.ID = randHex(12)
		// Ensure fresh-id uniqueness within this batch + existing map.
		for {
			if _, dup := shares[e.ID]; !dup {
				dupBatch := false
				for _, b := range toAdd {
					if b != e && b.ID == e.ID {
						dupBatch = true
						break
					}
				}
				if !dupBatch {
					break
				}
			}
			e.ID = randHex(12)
		}
		shares[e.ID] = e
		shareOrder = append(shareOrder, e.ID)
	}
	for _, id := range prune {
		if e, ok := shares[id]; ok {
			delete(shares, id)
			for i, sid := range shareOrder {
				if sid == id {
					shareOrder = append(shareOrder[:i], shareOrder[i+1:]...)
					break
				}
			}
			if e.SessionID != "" {
				delete(sessToShare, strings.ToLower(e.SessionID))
			}
		}
	}
	// Invalidate cached stats after mutating (separate mutex — do it while
	// still holding regMu is safe, but batch it here for clarity).
	addedIDs := make([]string, 0, len(toAdd)+len(prune))
	for _, e := range toAdd {
		addedIDs = append(addedIDs, e.ID)
	}
	addedIDs = append(addedIDs, prune...)
	regMu.Unlock()
	for _, id := range addedIDs {
		invalidateShareStat(id)
	}
}

// isInsideTempLocked is isInsideTemp without re-resolving on every call;
// caller holds regMu (no disk I/O here beyond the EvalSymlinks already done
// in isInsideTemp — kept separate to avoid lock-order surprises).
func isInsideTempLocked(absPath, base string) bool {
	if base == "" || absPath == "" {
		return false
	}
	cleanBase := filepath.Clean(base)
	cleanP := filepath.Clean(absPath)
	if cleanP == cleanBase {
		return true
	}
	sep := string(os.PathSeparator)
	return strings.HasPrefix(cleanP, cleanBase+sep)
}

// ---------------------------------------------------------------------------
// GET /api/shares (public)
// ---------------------------------------------------------------------------

func handleAPIShares(w http.ResponseWriter, r *http.Request) {
	// Live-sync managed <temp> files first: manual copies appear, external
	// deletes disappear — no restart needed.
	syncTempLive()
	// Snapshot under lock, stat outside it: shareAvailable/shareMtime do
	// disk I/O that must never block share/unshare/presence.
	regMu.Lock()
	snap := make([]*shareEntry, 0, len(shareOrder))
	for _, id := range shareOrder {
		if e, ok := shares[id]; ok {
			cp := *e
			snap = append(snap, &cp)
		}
	}
	regMu.Unlock()
	out := make([]map[string]interface{}, 0, len(snap))
	for _, e := range snap {
		out = append(out, shareJSON(e))
	}
	sendJSON(w, r, 200, map[string]interface{}{"shares": out})
}

// ---------------------------------------------------------------------------
// POST /api/share (owner-only): index a host-disk file or dir
// ---------------------------------------------------------------------------

func dirSizeAbs(root string) (files int, total int64) {
	stack := []string{root}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		rd, err := os.ReadDir(cur)
		if err != nil {
			continue
		}
		for _, de := range rd {
			full := filepath.Join(cur, de.Name())
			// Never follow symlinks (cycle-safe); count real regular files.
			st, err := os.Lstat(full)
			if err != nil {
				continue
			}
			if st.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if st.IsDir() {
				stack = append(stack, full)
				continue
			}
			if st.Mode().IsRegular() {
				files++
				total += st.Size()
			}
		}
	}
	return files, total
}

func handleAPIShare(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	data := readJSONBody(r, jsonSmallMax)
	raw := strings.TrimSpace(jStr(data, "path"))
	if raw == "" {
		shareFail(w, r, 400, "share_bad_path")
		return
	}
	cleaned := filepath.Clean(raw)
	if shareBlocked(cleaned) {
		shareFail(w, r, 403, "share_blocked")
		return
	}
	if !filepath.IsAbs(cleaned) {
		shareFail(w, r, 400, "share_need_abs")
		return
	}
	// Resolve symlinks, then re-check the target (no escape to /etc/...).
	target := cleaned
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		target = resolved
	}
	if shareBlocked(target) {
		shareFail(w, r, 403, "share_blocked")
		return
	}
	st, err := os.Stat(target)
	if err != nil {
		shareFail(w, r, 404, "share_not_found")
		return
	}
	kind := ""
	var size int64
	if st.IsDir() {
		n, total := dirSizeAbs(target)
		if n == 0 {
			shareFail(w, r, 400, "share_empty")
			return
		}
		kind = "dir"
		size = total
	} else if st.Mode().IsRegular() {
		if st.Size() == 0 {
			shareFail(w, r, 400, "share_empty")
			return
		}
		kind = "file"
		size = st.Size()
	} else {
		shareFail(w, r, 400, "share_not_regular")
		return
	}
	regMu.Lock()
	if len(shares) >= shareMax {
		regMu.Unlock()
		shareFail(w, r, 429, "share_limit")
		return
	}
	regMu.Unlock()
	id := newShareID()
	e := &shareEntry{
		ID: id, Name: filepath.Base(cleaned), Kind: kind, Size: size,
		Owner: "host", OwnerID: "host", Source: "host",
		HostPath: target, Created: nowUnix(),
	}
	regMu.Lock()
	shares[id] = e
	shareOrder = append(shareOrder, id)
	regMu.Unlock()
	invalidateShareStat(id)
	writeLog(clientIP(r), "share", kind+" "+target)
	sendJSON(w, r, 200, map[string]interface{}{"ok": true, "id": id})
}

// ---------------------------------------------------------------------------
// POST /api/share/guest (public): offer a file + get an upload session
// ---------------------------------------------------------------------------

func handleAPIShareGuest(w http.ResponseWriter, r *http.Request) {
	data := readJSONBody(r, jsonSmallMax)
	// Folder uploads announce per-file rel paths ("dir/sub/file"); keep
	// the structure via safeRelPath (same traversal/depth/length rules as
	// host shares). Guest bytes still stage under UUID dirs and retain
	// under share-ID paths — Name is display/comparison only.
	name := safeRelPath(strings.TrimSpace(jStr(data, "name")))
	if name == "" || strings.HasPrefix(name, ".") {
		shareFail(w, r, 400, "share_bad_path")
		return
	}
	size := jInt(data, "size", 0)
	if size <= 0 {
		shareFail(w, r, 400, "share_bad_path")
		return
	}
	if size > maxBytes() {
		shareFail(w, r, 413, "share_too_big")
		return
	}
	kind := strings.ToLower(strings.TrimSpace(jStr(data, "kind")))
	if kind == "" {
		kind = "file"
	}
	if kind != "file" {
		shareFail(w, r, 400, "share_bad_path")
		return
	}
	// Optional attribution for presence/relay. Empty owner_id means the
	// entry never goes presence-gated (always available once complete)
	// but live relay during upload requires a real owner_id.
	ownerID := strings.TrimSpace(jStr(data, "owner_id"))
	owner := strings.TrimSpace(jStr(data, "owner"))
	if len(owner) > 64 {
		owner = owner[:64]
	}
	// Piece framing for the V2 chunked engine (the web client only speaks
	// /upload_piece). Accept an explicit piece_len/noverify so turbo mode
	// keeps its 8MB lanes; default to reliable 2MB.
	plen := int64(pieceDefault)
	if v := jInt(data, "piece_len", 0); v >= int64(pieceMin) && v <= int64(pieceMax) {
		plen = v
	}
	noVerify := jBool(data, "noverify") || jBool(data, "noVerify")
	if jBool(data, "turbo") {
		noVerify = true
	}
	if noVerify && jInt(data, "piece_len", 0) <= 0 {
		// Turbo default (8MB) clamped to the server max.
		tp := int64(8 * 1024 * 1024)
		if tp > int64(pieceMax) {
			tp = int64(pieceMax)
		}
		if tp >= int64(pieceMin) {
			plen = tp
		}
	}
	needN := int((size + plen - 1) / plen)
	if needN <= 0 || needN > maxSessionPieces {
		shareFail(w, r, 413, "share_too_big")
		return
	}
	regMu.Lock()
	overLimit := len(shares) >= shareMax
	regMu.Unlock()
	if overLimit {
		shareFail(w, r, 429, "share_limit")
		return
	}
	shareID := newShareID()
	sessID := newGuestSessionID()
	// V2 open session for the /upload_piece engine (the web client never
	// speaks /upload_chunk). Bitmap all-zero, hashes claimed per-piece.
	sdir := sessDir(sessID)
	if sdir == "" {
		shareFail(w, r, 500, "share_bad_path")
		return
	}
	if err := os.MkdirAll(sdir, 0755); err != nil {
		shareFail(w, r, 500, "share_bad_path")
		return
	}
	saveMeta(sessID, &sessionMeta{V: 2, Name: name, Size: size,
		PieceLen: plen, Hashes: make([]*string, needN),
		Bitmap: strings.Repeat("0", needN), Ranges: [][2]int64{},
		Created: nowUnix(), NoVerify: noVerify})
	if f, err := os.OpenFile(filepath.Join(sdir, "data.part"),
		os.O_WRONLY|os.O_CREATE, 0644); err == nil {
		_ = f.Truncate(size)
		f.Close()
	}
	e := &shareEntry{
		ID: shareID, Name: name, Kind: "file", Size: size,
		Owner: owner, OwnerID: ownerID, Source: "guest",
		SessionID: sessID, Completed: false, Created: nowUnix(),
	}
	regMu.Lock()
	shares[shareID] = e
	shareOrder = append(shareOrder, shareID)
	sessToShare[strings.ToLower(sessID)] = shareID
	regMu.Unlock()
	writeLog(clientIP(r), "share_guest", name+" ("+strconv.FormatInt(size, 10)+"b)")
	sendJSON(w, r, 200, map[string]interface{}{
		"ok": true, "id": shareID, "session": sessID,
		"piece_len": plen, "noverify": noVerify})
}

// upgradeGuestV1ToV2 migrates a legacy V1 guest-share session to V2 in
// place (the web client only speaks /upload_piece since v1.6). V1 bytes
// are sequential [0,received), so fully covered pieces are marked '1'
// and the sparse file is truncated to the full size. Returns the new
// meta, or nil when not a migratable guest V1.
func upgradeGuestV1ToV2(uid string) *sessionMeta {
	uid = strings.ToLower(uid)
	regMu.Lock()
	_, isGuest := sessToShare[uid]
	regMu.Unlock()
	if !isGuest {
		return nil
	}
	lk := sessLock(uid)
	lk.Lock()
	defer lk.Unlock()
	m := loadMeta(uid)
	if m == nil || m.V == 2 {
		return m
	}
	if m.Size <= 0 {
		return nil
	}
	plen := int64(pieceDefault)
	needN := int((m.Size + plen - 1) / plen)
	if needN <= 0 || needN > maxSessionPieces {
		return nil
	}
	var received int64
	if st, err := os.Stat(filepath.Join(sessDir(uid), "data.part")); err == nil {
		received = st.Size()
	}
	if received < 0 {
		received = 0
	}
	if received > m.Size {
		received = m.Size
	}
	bm := make([]byte, needN)
	for i := 0; i < needN; i++ {
		ps, pe := int64(i)*plen, int64(i)*plen+plen
		if pe > m.Size {
			pe = m.Size
		}
		if pe <= received {
			bm[i] = '1'
		} else {
			_ = ps
			bm[i] = '0'
		}
	}
	nm := &sessionMeta{V: 2, Name: m.Name, Size: m.Size,
		PieceLen: plen, Hashes: make([]*string, needN),
		Bitmap: string(bm), Created: m.Created, NoVerify: false,
		Extract: m.Extract}
	if received > 0 {
		nm.Ranges = [][2]int64{{0, received}}
	} else {
		nm.Ranges = [][2]int64{}
	}
	saveMeta(uid, nm)
	if f, err := os.OpenFile(filepath.Join(sessDir(uid), "data.part"),
		os.O_WRONLY|os.O_CREATE, 0644); err == nil {
		_ = f.Truncate(m.Size)
		f.Close()
	}
	touchSessDir(uid)
	return nm
}

// guestComplete saves a finished guest upload as a REAL managed file.
// Called from handleUploadComplete when the session belongs to a share.
func guestComplete(w http.ResponseWriter, r *http.Request, uid, shareID string,
	data map[string]interface{}) {
	uid = strings.ToLower(uid)
	// Migrate legacy V1 guest sessions so old announces can still finish.
	if m0 := loadMeta(uid); m0 != nil && m0.V != 2 {
		_ = upgradeGuestV1ToV2(uid)
	}
	m := loadMeta(uid)
	regMu.Lock()
	e, ok := shares[shareID]
	regMu.Unlock()
	if !ok || e.Source != "guest" {
		fail(w, r, 404, "up_session_gone")
		return
	}
	if m == nil {
		// Idempotent retry after a completed save.
		if e.HostPath != "" {
			if st, err := os.Stat(e.HostPath); err == nil && st.Size() == e.Size {
				sendJSON(w, r, 200, map[string]interface{}{
					"ok": true, "saved": []string{e.Name}, "share": shareID})
				return
			}
		}
		// Legacy retry: hidden .shares/<id> exists — heal it by migrating
		// to a real managed file with the original name.
		if st, err := os.Stat(retainedPath(shareID)); err == nil && st.Size() == e.Size {
			if dest, ok := migrateLegacyRetained(e); ok {
				_ = dest
				sendJSON(w, r, 200, map[string]interface{}{
					"ok": true, "saved": []string{e.Name}, "share": shareID})
				return
			}
		}
		fail(w, r, 404, "up_session_gone")
		return
	}
	lk := sessLock(uid)
	lk.Lock()
	m = loadMeta(uid)
	if m == nil {
		lk.Unlock()
		fail(w, r, 404, "up_session_gone")
		return
	}
	var received int64
	if m.V == 2 {
		if missing := missingPieces(m); len(missing) > 0 {
			lk.Unlock()
			sendJSON(w, r, 400, map[string]interface{}{
				"error":   tr(reqLang(r), "up_incomplete_n", len(missing)),
				"missing": missing})
			return
		}
		received = m.Size
	} else {
		received = sessReceived(uid)
		if received != m.Size {
			lk.Unlock()
			sendJSON(w, r, 400, map[string]interface{}{
				"error": tr(reqLang(r), "up_incomplete_bytes",
					strconv.FormatInt(received, 10), strconv.FormatInt(m.Size, 10))})
			return
		}
	}
	src := filepath.Join(sessDir(uid), "data.part")
	lk.Unlock()
	// Save as a REAL managed file (<temp>/<original-name>) with full data:
	// same temp filesystem, so a rename is atomic; fall back to copy.
	// Collision-safe via uniquePath + O_EXCL reservation (like legacy uploads).
	relName := safeRelPath(e.Name)
	if relName == "" {
		relName = safeRelPath(m.Name)
	}
	if relName == "" {
		sendJSON(w, r, 500, map[string]interface{}{
			"error": tr(reqLang(r), "up_save_fail", m.Name)})
		return
	}
	base := tempBase()
	uploadLock.Lock()
	dest := ""
	var rerr error
	for tries := 0; tries < 10; tries++ {
		cand := uniquePath(base, relName)
		_ = os.MkdirAll(filepath.Dir(cand), 0755)
		rsv, cerr := os.OpenFile(cand, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if cerr != nil {
			rerr = cerr
			if os.IsExist(cerr) {
				continue
			}
			break
		}
		rsv.Close()
		_ = os.Remove(cand)
		rerr = os.Rename(src, cand)
		if rerr == nil {
			dest = cand
		}
		break
	}
	uploadLock.Unlock()
	if rerr != nil {
		// EXDEV fallback: copy then remove staging.
		if dest == "" {
			uploadLock.Lock()
			dest = uniquePath(base, relName)
			uploadLock.Unlock()
		}
		_ = os.MkdirAll(filepath.Dir(dest), 0755)
		if in, err := os.Open(src); err == nil {
			out, cerr := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
			if cerr == nil {
				_, rerr = io.Copy(out, in)
				out.Close()
			} else {
				rerr = cerr
			}
			in.Close()
		} else {
			rerr = err
		}
	}
	if rerr != nil || dest == "" {
		sendJSON(w, r, 500, map[string]interface{}{
			"error": tr(reqLang(r), "up_save_fail", m.Name)})
		return
	}
	_ = os.RemoveAll(sessDir(uid))
	forgetSessLock(uid)
	// Drop any relay-cached prefix for this share (incomplete downloads may
	// have appended to the legacy hidden path); the managed file is the truth.
	_ = os.Remove(retainedPath(shareID))
	regMu.Lock()
	if live, ok := shares[shareID]; ok {
		live.Completed = true
		live.HostPath = dest
		live.Size = received
		// Keep the original rel name (folder structure preserved).
		if live.Name == "" {
			live.Name = relName
		}
	}
	regMu.Unlock()
	invalidateShareStat(shareID)
	invalidateListCaches()
	savedRel := relName
	if rel, err := filepath.Rel(base, dest); err == nil {
		savedRel = filepath.ToSlash(rel)
	}
	writeLog(clientIP(r), "share_guest_done",
		savedRel+" ("+strconv.FormatInt(received, 10)+"b)")
	sendJSON(w, r, 200, map[string]interface{}{
		"ok": true, "saved": []string{savedRel}, "share": shareID})
}

// migrateLegacyRetained heals a legacy hidden .shares/<id> file by moving it
// to a real managed path (<temp>/<original-name>). Returns dest + true on
// success. Used by idempotent retries and boot migration.
func migrateLegacyRetained(e *shareEntry) (string, bool) {
	if e == nil {
		return "", false
	}
	legacy := retainedPath(e.ID)
	st, err := os.Lstat(legacy)
	if err != nil || !st.Mode().IsRegular() || st.Size() <= 0 {
		return "", false
	}
	relName := safeRelPath(e.Name)
	if relName == "" {
		short := e.ID
		if len(short) > 8 {
			short = short[:8]
		}
		relName = "restored-" + short
	}
	base := tempBase()
	if base == "" {
		return "", false
	}
	uploadLock.Lock()
	defer uploadLock.Unlock()
	dest := uniquePath(base, relName)
	_ = os.MkdirAll(filepath.Dir(dest), 0755)
	if err := os.Rename(legacy, dest); err != nil {
		// EXDEV: copy then remove.
		in, err := os.Open(legacy)
		if err != nil {
			return "", false
		}
		defer in.Close()
		out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return "", false
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			_ = os.Remove(dest)
			return "", false
		}
		out.Close()
		_ = os.Remove(legacy)
	}
	regMu.Lock()
	if live, ok := shares[e.ID]; ok {
		live.HostPath = dest
		live.Completed = true
		if live.Name == "" {
			live.Name = relName
		}
	}
	regMu.Unlock()
	invalidateShareStat(e.ID)
	return dest, true
}

// ---------------------------------------------------------------------------
// POST /api/unshare (public): remove entry + its temp data
// ---------------------------------------------------------------------------

func handleAPIUnshare(w http.ResponseWriter, r *http.Request) {
	data := readJSONBody(r, jsonSmallMax)
	id := strings.TrimSpace(jStr(data, "id"))
	if id == "" {
		shareFail(w, r, 400, "share_bad_path")
		return
	}
	regMu.Lock()
	e, ok := shares[id]
	if ok {
		delete(shares, id)
		for i, sid := range shareOrder {
			if sid == id {
				shareOrder = append(shareOrder[:i], shareOrder[i+1:]...)
				break
			}
		}
		if e.SessionID != "" {
			delete(sessToShare, strings.ToLower(e.SessionID))
		}
	}
	regMu.Unlock()
	if !ok {
		shareFail(w, r, 404, "share_not_found")
		return
	}
	// Managed model: present => downloadable, unshare => delete real bytes.
	// - Guest with HostPath (new model): delete the real managed file.
	// - Host with HostPath INSIDE temp: delete the real file/dir.
	// - Host with HostPath OUTSIDE temp: index-only, keep the disk file.
	// - Legacy guest without HostPath: delete hidden .shares/<id>.
	if e.HostPath != "" && isInsideTemp(e.HostPath) {
		if e.Kind == "dir" {
			_ = os.RemoveAll(e.HostPath)
		} else {
			_ = os.Remove(e.HostPath)
			// Prune newly-emptied folder parents (folder uploads).
			if base := tempBase(); base != "" {
				if rel, err := filepath.Rel(base, filepath.Clean(e.HostPath)); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
					pruneEmptyParents(filepath.ToSlash(rel))
				}
			}
		}
		if e.SessionID != "" {
			_ = os.RemoveAll(sessDir(strings.ToLower(e.SessionID)))
			forgetSessLock(strings.ToLower(e.SessionID))
		}
	} else if e.Source == "guest" {
		if e.HostPath != "" {
			_ = os.Remove(e.HostPath)
		}
		_ = os.Remove(retainedPath(id))
		if e.SessionID != "" {
			_ = os.RemoveAll(sessDir(strings.ToLower(e.SessionID)))
			forgetSessLock(strings.ToLower(e.SessionID))
		}
	}
	invalidateShareStat(id)
	invalidateListCaches()
	writeLog(clientIP(r), "unshare", e.Name)
	sendJSON(w, r, 200, map[string]interface{}{"ok": true})
}

// ---------------------------------------------------------------------------
// GET /r/<id> (+HEAD, Range, public)
// ---------------------------------------------------------------------------

func handleRegistryGet(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/r/")
	if id == "" || strings.Contains(id, "/") {
		shareFail(w, r, 404, "share_not_found")
		return
	}
	regMu.Lock()
	e, ok := shares[id]
	regMu.Unlock()
	if !ok {
		shareFail(w, r, 404, "share_not_found")
		return
	}
	switch e.Source {
	case "host":
		if e.Kind == "dir" {
			serveHostDirZip(w, r, e)
			return
		}
		serveHostFile(w, r, e)
	default:
		serveGuestEntry(w, r, e)
	}
}

func serveHostFile(w http.ResponseWriter, r *http.Request, e *shareEntry) {
	st, err := os.Stat(e.HostPath)
	if err != nil || !st.Mode().IsRegular() {
		shareFail(w, r, 404, "share_not_found")
		return
	}
	size := st.Size()
	ctype := mimeByName(e.Name)
	disp := "attachment; filename*=UTF-8''" + percentEncode(filepath.Base(e.Name))
	etag := "\"" + strconv.FormatInt(size, 10) + "-" +
		strconv.FormatInt(st.ModTime().Unix(), 10) + "\""
	lastMod := st.ModTime().UTC().Format(http.TimeFormat)
	setFileHeaders := func() {
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Content-Disposition", disp)
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", lastMod)
	}
	if etagMatches(r, etag) || modifiedSinceAllowsNotModified(r, lastMod) {
		notModified(w, etag, lastMod)
		return
	}
	if r.Method == "HEAD" {
		setFileHeaders()
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.WriteHeader(200)
		return
	}
	rawRange := r.Header.Get("Range")
	start, end, partial := parseRange(rawRange, size)
	if partial && !ifRangeAllowsPartial(r, etag, lastMod) {
		partial = false
		start, end = 0, size-1
	}
	if strings.TrimSpace(rawRange) != "" && !partial && r.Header.Get("If-Range") == "" {
		w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(size, 10))
		fail(w, r, 416, "up_range_invalid")
		return
	}
	f, oerr := os.Open(e.HostPath)
	if oerr != nil {
		shareFail(w, r, 404, "share_not_found")
		return
	}
	defer f.Close()
	setFileHeaders()
	if !acquireDownloadSlot(r) {
		return
	}
	defer releaseDownloadSlot()
	t0 := nowNano()
	var sent int64
	if partial {
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.Header().Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+
			strconv.FormatInt(end, 10)+"/"+strconv.FormatInt(size, 10))
		w.WriteHeader(206)
		_, _ = f.Seek(start, io.SeekStart)
		sent = streamCopy(w, f, end-start+1)
	} else {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.WriteHeader(200)
		sent = streamCopy(w, f, size)
	}
	dt := float64(nowNano()-t0) / 1e9
	if dt < 0.001 {
		dt = 0.001
	}
	writeLog(clientIP(r), "download", e.Name+" ("+strconv.FormatInt(sent, 10)+"b)")
}

// absEntry is one regular file under an absolute host dir.
type absEntry struct {
	rel   string // slash-separated, relative to root
	abs   string
	size  int64
	mtime int64
}

func walkAbsDir(root string, limit int) ([]absEntry, bool) {
	if ce, tr, ok := getCachedWalkAbs(root, limit); ok {
		return ce, tr
	}
	entries, truncated := walkAbsDirUncached(root, limit)
	setCachedWalkAbs(root, limit, entries, truncated)
	return entries, truncated
}

func walkAbsDirUncached(root string, limit int) ([]absEntry, bool) {
	out := []absEntry{}
	type item struct{ dir, rel string }
	stack := []item{{dir: root}}
	for len(stack) > 0 {
		if len(out) >= limit {
			return out, true
		}
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		rd, err := os.ReadDir(cur.dir)
		if err != nil {
			continue
		}
		// Sort entries by name without an auxiliary map (vet-clean + less alloc).
		sort.Slice(rd, func(i, j int) bool { return rd[i].Name() < rd[j].Name() })
		for _, de := range rd {
			nm := de.Name()
			if strings.HasPrefix(nm, ".") {
				continue
			}
			full := filepath.Join(cur.dir, nm)
			st, err := os.Lstat(full)
			if err != nil || st.Mode()&os.ModeSymlink != 0 {
				continue
			}
			rel := nm
			if cur.rel != "" {
				rel = cur.rel + "/" + nm
			}
			if st.IsDir() {
				stack = append(stack, item{dir: full, rel: rel})
				continue
			}
			if !st.Mode().IsRegular() {
				continue
			}
			if len(out) >= limit {
				return out, true
			}
			out = append(out, absEntry{rel: rel, abs: full,
				size: st.Size(), mtime: st.ModTime().Unix()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	return out, false
}

// walkAbsDirWithEmpty lists files + empty dirs in ONE walk (host-dir zip
// fast path: replaces walkAbsDir + collectEmptyDirsAbs double-walk).
func walkAbsDirWithEmpty(root string, limit int) ([]absEntry, []zipDirEntry, bool) {
	out := []absEntry{}
	allDirs := map[string]zipDirEntry{}
	ancestors := map[string]bool{}
	type item struct{ dir, rel string }
	stack := []item{{dir: root}}
	for len(stack) > 0 {
		if len(out) >= limit {
			return out, nil, true
		}
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		rd, err := os.ReadDir(cur.dir)
		if err != nil {
			continue
		}
		sort.Slice(rd, func(i, j int) bool { return rd[i].Name() < rd[j].Name() })
		for _, de := range rd {
			nm := de.Name()
			if strings.HasPrefix(nm, ".") {
				continue
			}
			full := filepath.Join(cur.dir, nm)
			st, err := os.Lstat(full)
			if err != nil || st.Mode()&os.ModeSymlink != 0 {
				continue
			}
			rel := nm
			if cur.rel != "" {
				rel = cur.rel + "/" + nm
			}
			if st.IsDir() {
				allDirs[rel] = zipDirEntry{name: rel + "/", mtime: st.ModTime()}
				stack = append(stack, item{dir: full, rel: rel})
				continue
			}
			if !st.Mode().IsRegular() {
				continue
			}
			if len(out) >= limit {
				return out, nil, true
			}
			out = append(out, absEntry{rel: rel, abs: full,
				size: st.Size(), mtime: st.ModTime().Unix()})
			name := rel
			for {
				i := strings.LastIndex(name, "/")
				if i < 0 {
					break
				}
				name = name[:i]
				ancestors[name] = true
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	empty := []zipDirEntry{}
	for rel, d := range allDirs {
		if !ancestors[rel] {
			empty = append(empty, d)
		}
	}
	sort.Slice(empty, func(i, j int) bool { return empty[i].name < empty[j].name })
	return out, empty, false
}

// absStoreZipLength mirrors storeZipLength for absolute-dir entries.
func absStoreZipLength(entries []absEntry, dirs []zipDirEntry) (int64, bool) {
	if int64(len(entries)+len(dirs)) >= 65535 {
		return 0, false
	}
	var total int64
	add := func(n int64) bool {
		total += n
		return total < 1<<32
	}
	for _, e := range entries {
		if e.rel == "" || len(e.rel) > 0xffff || e.size < 0 ||
			uint64(e.size) >= 0xffffffff {
			return 0, false
		}
		if !add(30 + 9 + int64(len(e.rel)) + e.size + 16) {
			return 0, false
		}
		if !add(46 + 9 + int64(len(e.rel))) {
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

func collectEmptyDirsAbs(root string, entries []absEntry) []zipDirEntry {
	ancestors := map[string]bool{}
	for _, e := range entries {
		name := e.rel
		for {
			i := strings.LastIndex(name, "/")
			if i < 0 {
				break
			}
			name = name[:i]
			ancestors[name] = true
		}
	}
	out := []zipDirEntry{}
	type item struct{ disk, sub string }
	stack := []item{{disk: root}}
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
			st, err := os.Lstat(filepath.Join(cur.disk, nm))
			if err != nil || st.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if !st.IsDir() {
				continue
			}
			sub := nm
			if cur.sub != "" {
				sub = cur.sub + "/" + nm
			}
			stack = append(stack, item{disk: filepath.Join(cur.disk, nm), sub: sub})
			if !ancestors[sub] {
				out = append(out, zipDirEntry{name: sub + "/", mtime: st.ModTime()})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// serveHostDirZip streams a live zip of an absolute host dir.
// ZIP-only policy: STORE (uncompressed) always — every stock OS/phone opens
// it, no CPU spent on deflate, exact Content-Length when under zip32 limits.
func serveHostDirZip(w http.ResponseWriter, r *http.Request, e *shareEntry) {
	root := e.HostPath
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		shareFail(w, r, 404, "share_not_found")
		return
	}
	entries, dirs, truncated := walkAbsDirWithEmpty(root, maxZipFiles+1)
	if len(entries) == 0 {
		shareFail(w, r, 404, "zip_empty")
		return
	}
	if truncated || len(entries) > maxZipFiles {
		shareFail(w, r, 413, "zip_too_big")
		return
	}
	var total int64
	for _, en := range entries {
		total += en.size
		if total > maxZipBytes {
			shareFail(w, r, 413, "zip_too_big")
			return
		}
	}
	zipName := filepath.Base(root) + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+
		asciiFallbackName(zipName)+"\"; filename*=UTF-8''"+percentEncode(zipName))
	w.Header().Set("Accept-Ranges", "none")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if wire, ok := absStoreZipLength(entries, dirs); ok {
		w.Header().Set("Content-Length", strconv.FormatInt(wire, 10))
	}
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
	written := 0
	for _, d := range dirs {
		fh := &zip.FileHeader{Name: d.name, Method: zip.Store}
		fh.SetModTime(d.mtime)
		if _, err := zw.CreateHeader(fh); err != nil {
			continue
		}
	}
	buf := getCopyBuf()
	defer putCopyBuf(buf)
	for _, en := range entries {
		lst, err := os.Lstat(en.abs)
		if err != nil || !lst.Mode().IsRegular() {
			continue
		}
		f, err := os.Open(en.abs)
		if err != nil {
			continue
		}
		fh := &zip.FileHeader{Name: en.rel, Method: zip.Store}
		fh.SetModTime(time.Unix(en.mtime, 0))
		fw, err := zw.CreateHeader(fh)
		if err != nil {
			f.Close()
			break
		}
		if _, err := io.CopyBuffer(fw, f, buf); err != nil {
			f.Close()
			break
		}
		f.Close()
		written++
	}
	if cerr := zw.Close(); cerr != nil {
		writeLog(clientIP(r), "download_zip_fail", e.Name)
		return
	}
	writeLog(clientIP(r), "download_zip",
		e.Name+" ("+strconv.FormatInt(int64(written), 10)+" files)")
}

// ---------------------------------------------------------------------------
// guest serving (+ relay fetch for the missing tail)
// ---------------------------------------------------------------------------

type ignoreErrWriter struct{ w io.Writer }

func (x ignoreErrWriter) Write(p []byte) (int, error) {
	_, _ = x.w.Write(p)
	return len(p), nil
}

func serveGuestEntry(w http.ResponseWriter, r *http.Request, e *shareEntry) {
	rp := guestRealPath(e)
	st, serr := os.Stat(rp)
	var received int64
	if serr == nil {
		received = st.Size()
	}
	complete := e.Completed && serr == nil && received >= e.Size && e.Size > 0
	if !complete {
		// Fully cached sub-range? Serve it without bothering the owner.
		size := e.Size
		rawRange := strings.TrimSpace(r.Header.Get("Range"))
		if rawRange != "" {
			s, en, partial := parseRange(rawRange, size)
			_ = s
			if partial && en < received {
				serveRetainedRange(w, r, e, rp, size, s, en, true)
				return
			}
		}
		if !presenceAlive(e.OwnerID) {
			// Owner gone: never report "unavailable" when we hold bytes.
			// Serve whatever prefix is on disk (resumable); only 404 when
			// nothing was ever retained.
			if serr == nil && received > 0 {
				if rawRange == "" {
					serveRetainedRange(w, r, e, rp, received, 0, received-1, false)
					return
				}
				shareFail(w, r, 416, "share_unavailable")
				return
			}
			shareFail(w, r, 404, "share_unavailable")
			return
		}
		serveGuestRelay(w, r, e, rp, received)
		return
	}
	// Managed copy serves even after the owner leaves.
	size := e.Size
	etag := guestETag(e)
	var lastMod string
	if st2, err := os.Stat(rp); err == nil {
		lastMod = st2.ModTime().UTC().Format(http.TimeFormat)
	}
	if etagMatches(r, etag) || (lastMod != "" && modifiedSinceAllowsNotModified(r, lastMod)) {
		notModified(w, etag, lastMod)
		return
	}
	if r.Method == "HEAD" {
		serveRetainedHeaders(w, e, rp, size, 200, 0, size-1, false)
		w.WriteHeader(200)
		return
	}
	rawRange := r.Header.Get("Range")
	start, end, partial := parseRange(rawRange, size)
	if partial && !ifRangeAllowsPartial(r, etag, lastMod) {
		partial = false
		start, end = 0, size-1
	}
	if strings.TrimSpace(rawRange) != "" && !partial && r.Header.Get("If-Range") == "" {
		w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(size, 10))
		fail(w, r, 416, "up_range_invalid")
		return
	}
	if !partial {
		serveRetainedRange(w, r, e, rp, size, 0, size-1, false)
		return
	}
	serveRetainedRange(w, r, e, rp, size, start, end, true)
}

func guestETag(e *shareEntry) string {
	return "\"" + e.ID + "-" + strconv.FormatInt(e.Size, 10) + "\""
}

func serveRetainedHeaders(w http.ResponseWriter, e *shareEntry, rp string,
	size int64, status int, start, end int64, partial bool) {
	w.Header().Set("Content-Type", mimeByName(e.Name))
	w.Header().Set("Content-Disposition",
		"attachment; filename*=UTF-8''"+percentEncode(filepath.Base(e.Name)))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("ETag", guestETag(e))
	if st, err := os.Stat(rp); err == nil {
		w.Header().Set("Last-Modified", st.ModTime().UTC().Format(http.TimeFormat))
	}
	if partial {
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.Header().Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+
			strconv.FormatInt(end, 10)+"/"+strconv.FormatInt(size, 10))
	} else {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
}

func serveRetainedRange(w http.ResponseWriter, r *http.Request, e *shareEntry,
	rp string, size, start, end int64, partial bool) {
	f, err := os.Open(rp)
	if err != nil {
		shareFail(w, r, 404, "share_unavailable")
		return
	}
	defer f.Close()
	// Revalidation on the retained path too (cheap: headers only).
	etag := guestETag(e)
	var lastMod string
	if st, serr := os.Stat(rp); serr == nil {
		lastMod = st.ModTime().UTC().Format(http.TimeFormat)
	}
	if r.Method != "HEAD" && (etagMatches(r, etag) || (lastMod != "" && modifiedSinceAllowsNotModified(r, lastMod))) {
		notModified(w, etag, lastMod)
		return
	}
	serveRetainedHeaders(w, e, rp, size, 200, start, end, partial)
	if partial {
		w.WriteHeader(206)
	} else {
		w.WriteHeader(200)
	}
	if r.Method == "HEAD" {
		return
	}
	if !acquireDownloadSlot(r) {
		return
	}
	defer releaseDownloadSlot()
	_, _ = f.Seek(start, io.SeekStart)
	sent := streamCopy(w, f, end-start+1)
	writeLog(clientIP(r), "download", e.Name+" ("+strconv.FormatInt(sent, 10)+"b)")
}

// serveGuestRelay streams the cached prefix from disk, then the remainder
// relayed live from the owner. Headers go out only after the owner answers;
// a silent owner fails 504 with relay_timeout (nothing written yet).
func serveGuestRelay(w http.ResponseWriter, r *http.Request, e *shareEntry,
	rp string, received int64) {
	size := e.Size
	if r.Method == "HEAD" {
		serveRetainedHeaders(w, e, rp, size, 200, 0, size-1, false)
		w.WriteHeader(200)
		return
	}
	rawRange := r.Header.Get("Range")
	start, end, partial := parseRange(rawRange, size)
	if strings.TrimSpace(rawRange) != "" && !partial {
		w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(size, 10))
		fail(w, r, 416, "up_range_invalid")
		return
	}
	if !partial {
		start, end = 0, size-1
	}
	jobOffset := received
	if start > jobOffset {
		jobOffset = start
	}
	jobLen := end - jobOffset + 1
	if jobLen <= 0 {
		serveRetainedRange(w, r, e, rp, size, start, end, partial)
		return
	}
	job := relayEnqueue(e.OwnerID, e.ID, jobOffset, jobLen)
	defer relayRemove(job.Token)
	wait := time.Duration(relayWaitSec) * time.Second
	select {
	case <-job.claimed:
	case <-time.After(wait):
		sendJSON(w, r, 504, map[string]interface{}{
			"error": tr(reqLang(r), "relay_timeout"), "code": "relay_timeout"})
		return
	case <-r.Context().Done():
		return
	}
	// Owner is streaming into job.pipeR: headers first, then cached bytes
	// (if any of the requested range is local), then the relayed tail.
	// Relayed bytes that extend the contiguous prefix are appended to the
	// retained file so later requests serve more from disk.
	if !acquireDownloadSlot(r) {
		return
	}
	defer releaseDownloadSlot()
	serveRetainedHeaders(w, e, rp, size, 200, start, end, partial)
	if partial {
		w.WriteHeader(206)
	} else {
		w.WriteHeader(200)
	}
	if start < received {
		cachedEnd := end
		if cachedEnd >= received {
			cachedEnd = received - 1
		}
		if cachedEnd >= start {
			if f, err := os.Open(rp); err == nil {
				_, _ = f.Seek(start, io.SeekStart)
				buf := getCopyBuf()
				_, _ = io.CopyBuffer(w, io.LimitReader(f, cachedEnd-start+1), buf)
				putCopyBuf(buf)
				f.Close()
			}
		}
	}
	var src io.Reader = job.pipeR
	if jobOffset == received {
		if f, err := os.OpenFile(rp, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644); err == nil {
			defer f.Close()
			src = io.TeeReader(job.pipeR, ignoreErrWriter{f})
		}
	}
	buf := getCopyBuf()
	sent, _ := io.CopyBuffer(w, io.LimitReader(src, jobLen), buf)
	putCopyBuf(buf)
	_ = sent
	writeLog(clientIP(r), "download_relay", e.Name+" (offset "+
		strconv.FormatInt(jobOffset, 10)+")")
}

// ---------------------------------------------------------------------------
// relay tables: owners long-poll, pieces stream into waiting /r/ responses
// ---------------------------------------------------------------------------

type relayJob struct {
	Token   string
	ShareID string
	OwnerID string
	Offset  int64
	Length  int64
	pipeR   *io.PipeReader
	pipeW   *io.PipeWriter
	claimed chan struct{}
	created float64
}

var (
	relayMu      sync.Mutex
	relayJobs    = map[string]*relayJob{}
	relayPending = map[string][]*relayJob{}
	relayWaiters = map[string][]chan struct{}{}
)

// relayPollWaitSec bounds GET /api/relay/need long-polls (~25s per contract).
const relayPollWaitSec = 25

func relayEnqueue(ownerID, shareID string, offset, length int64) *relayJob {
	pr, pw := io.Pipe()
	job := &relayJob{
		Token: randHex(16), ShareID: shareID, OwnerID: ownerID,
		Offset: offset, Length: length,
		pipeR: pr, pipeW: pw, claimed: make(chan struct{}),
		created: nowUnix(),
	}
	relayMu.Lock()
	relayJobs[job.Token] = job
	relayPending[ownerID] = append(relayPending[ownerID], job)
	for _, ch := range relayWaiters[ownerID] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	relayWaiters[ownerID] = nil
	relayMu.Unlock()
	return job
}

func relayRemove(token string) {
	relayMu.Lock()
	job, ok := relayJobs[token]
	if ok {
		delete(relayJobs, token)
		if lst := relayPending[job.OwnerID]; len(lst) > 0 {
			kept := lst[:0]
			for _, j := range lst {
				if j.Token != token {
					kept = append(kept, j)
				}
			}
			if len(kept) == 0 {
				delete(relayPending, job.OwnerID)
			} else {
				relayPending[job.OwnerID] = kept
			}
		}
	}
	relayMu.Unlock()
	if ok {
		_ = job.pipeR.Close()
		_ = job.pipeW.Close()
	}
}

func relayJobJSON(j *relayJob) map[string]interface{} {
	return map[string]interface{}{
		"token": j.Token, "share": j.ShareID,
		"offset": j.Offset, "len": j.Length,
	}
}

// GET /api/relay/need?owner_id=X (public, long-poll ~25s)
func handleAPIRelayNeed(w http.ResponseWriter, r *http.Request) {
	ownerID := strings.TrimSpace(r.URL.Query().Get("owner_id"))
	if ownerID == "" {
		shareFail(w, r, 400, "relay_bad_owner")
		return
	}
	relayMu.Lock()
	pending := append([]*relayJob{}, relayPending[ownerID]...)
	if len(pending) > 0 {
		out := make([]map[string]interface{}, 0, len(pending))
		for _, j := range pending {
			out = append(out, relayJobJSON(j))
		}
		relayMu.Unlock()
		sendJSON(w, r, 200, map[string]interface{}{"jobs": out})
		return
	}
	waiter := make(chan struct{}, 1)
	relayWaiters[ownerID] = append(relayWaiters[ownerID], waiter)
	relayMu.Unlock()
	select {
	case <-waiter:
	case <-time.After(relayPollWaitSec * time.Second):
	case <-r.Context().Done():
	}
	relayMu.Lock()
	pending = append([]*relayJob{}, relayPending[ownerID]...)
	out := make([]map[string]interface{}, 0, len(pending))
	for _, j := range pending {
		out = append(out, relayJobJSON(j))
	}
	// Drop our waiter registration (a notify may have fired already).
	if lst := relayWaiters[ownerID]; len(lst) > 0 {
		kept := lst[:0]
		for _, ch := range lst {
			if ch != waiter {
				kept = append(kept, ch)
			}
		}
		if len(kept) == 0 {
			delete(relayWaiters, ownerID)
		} else {
			relayWaiters[ownerID] = kept
		}
	}
	relayMu.Unlock()
	sendJSON(w, r, 200, map[string]interface{}{"jobs": out})
}

// POST /api/relay/piece?token=T with RAW BYTES body (public)
func handleAPIRelayPiece(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		shareFail(w, r, 404, "relay_bad_token")
		return
	}
	relayMu.Lock()
	job, ok := relayJobs[token]
	if ok {
		// Claim exactly once; drop from the pending list so later polls
		// don't offer it again.
		select {
		case <-job.claimed:
		default:
			close(job.claimed)
		}
		if lst := relayPending[job.OwnerID]; len(lst) > 0 {
			kept := lst[:0]
			for _, j := range lst {
				if j.Token != token {
					kept = append(kept, j)
				}
			}
			if len(kept) == 0 {
				delete(relayPending, job.OwnerID)
			} else {
				relayPending[job.OwnerID] = kept
			}
		}
	}
	relayMu.Unlock()
	if !ok {
		shareFail(w, r, 404, "relay_bad_token")
		return
	}
	// Stream exactly Length bytes into the waiting /r/ response (never
	// buffer a huge tail in memory); extra body bytes are ignored.
	var copyErr error
	if r.Body != nil {
		defer r.Body.Close()
		buf := getCopyBuf()
		var n int64
		n, copyErr = io.CopyBuffer(job.pipeW, io.LimitReader(r.Body, job.Length), buf)
		putCopyBuf(buf)
		if copyErr == nil && n < job.Length {
			copyErr = io.ErrUnexpectedEOF
		}
	}
	if copyErr != nil {
		_ = job.pipeW.CloseWithError(copyErr)
	} else {
		_ = job.pipeW.Close()
	}
	sendJSON(w, r, 200, map[string]interface{}{"ok": true})
}

// ---------------------------------------------------------------------------
// GET /api/browse?dir=<abs> (owner-only)
// ---------------------------------------------------------------------------

func handleAPIBrowse(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("dir"))
	if raw == "" {
		shareFail(w, r, 400, "share_bad_path")
		return
	}
	cleaned := filepath.Clean(raw)
	if shareBlocked(cleaned) {
		shareFail(w, r, 403, "share_blocked")
		return
	}
	if !filepath.IsAbs(cleaned) {
		shareFail(w, r, 400, "share_need_abs")
		return
	}
	// Resolve symlinks like the old direct browse did.
	dir := cleaned
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		dir = resolved
	}
	if shareBlocked(dir) {
		shareFail(w, r, 403, "share_blocked")
		return
	}
	st, err := os.Stat(dir)
	if err != nil {
		shareFail(w, r, 404, "share_not_found")
		return
	}
	if !st.IsDir() {
		shareFail(w, r, 400, "share_not_regular")
		return
	}
	rd, err := os.ReadDir(dir)
	if err != nil {
		shareFail(w, r, 404, "share_not_found")
		return
	}
	type dirJSON2 struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	type fileJSON2 struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		Size  int64  `json:"size"`
		Mtime int64  `json:"mtime"`
	}
	dirs := []dirJSON2{}
	files := []fileJSON2{}
	for _, de := range rd {
		nm := de.Name()
		if strings.HasPrefix(nm, ".") {
			continue // hide dotfiles/dotdirs
		}
		// Fast path: non-symlinks use DirEntry info (1 syscall saved).
		// Symlink-to-dir/file still resolved via Stat (follow).
		if de.Type()&os.ModeSymlink == 0 {
			if de.IsDir() {
				dirs = append(dirs, dirJSON2{Name: nm, Path: filepath.Join(dir, nm)})
				continue
			}
			if info, err := de.Info(); err == nil && info.Mode().IsRegular() {
				files = append(files, fileJSON2{Name: nm, Path: filepath.Join(dir, nm),
					Size: info.Size(), Mtime: info.ModTime().Unix()})
			}
			continue
		}
		full := filepath.Join(dir, nm)
		fst, err := os.Stat(full) // follows symlinks
		if err != nil {
			continue
		}
		if fst.IsDir() {
			dirs = append(dirs, dirJSON2{Name: nm, Path: full})
			continue
		}
		if fst.Mode().IsRegular() {
			files = append(files, fileJSON2{Name: nm, Path: full,
				Size: fst.Size(), Mtime: fst.ModTime().Unix()})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	parent := filepath.Dir(dir)
	if parent == dir {
		parent = ""
	}
	home, _ := os.UserHomeDir()
	sendJSON(w, r, 200, map[string]interface{}{
		"dir": dir, "parent": parent, "home": home,
		"dirs": dirs, "files": files,
	})
}

// ---------------------------------------------------------------------------
// POST /api/presence (public heartbeat)
// ---------------------------------------------------------------------------

func handleAPIPresence(w http.ResponseWriter, r *http.Request) {
	data := readJSONBody(r, jsonSmallMax)
	ownerID := strings.TrimSpace(jStr(data, "owner_id"))
	if ownerID == "" {
		shareFail(w, r, 400, "share_bad_path")
		return
	}
	name := strings.TrimSpace(jStr(data, "name"))
	regMu.Lock()
	presence[ownerID] = &presenceEntry{Name: name, LastSeen: nowUnix()}
	regMu.Unlock()
	// Throttled: heartbeats arrive every few seconds per guest — a full
	// uploads+shares walk on each one is a disk storm. At most 1/min.
	if shouldSweepNow() {
		SweepTempOrphans()
	}
	sendJSON(w, r, 200, map[string]interface{}{"ok": true})
}

// ---------------------------------------------------------------------------
// GET /api/temp (public) + POST /api/temp/clean (owner-only)
// ---------------------------------------------------------------------------

func handleAPITemp(w http.ResponseWriter, r *http.Request) {
	base := tempBase()
	if sz, ok := getCachedTempSize(base); ok {
		sendJSON(w, r, 200, map[string]interface{}{
			"path": base, "size": sz, "warn_bytes": tempMaxBytes})
		return
	}
	sz := tempDirSize(base)
	setCachedTempSize(base, sz)
	sendJSON(w, r, 200, map[string]interface{}{
		"path": base, "size": sz, "warn_bytes": tempMaxBytes})
}

func handleAPITempClean(w http.ResponseWriter, r *http.Request) {
	if !adminOnly(w, r) {
		return
	}
	base := tempBase()
	before := tempDirSize(base)
	rd, err := os.ReadDir(base)
	if err != nil {
		shareFail(w, r, 500, "temp_bad_path")
		return
	}
	for _, de := range rd {
		// Never delete the TLS identity: wiping cert.pem/key.pem breaks
		// HTTPS until restart and looks like "hidden files reappearing".
		if de.Name() == ".beam-tls" {
			continue
		}
		_ = os.RemoveAll(filepath.Join(base, de.Name()))
	}
	after := tempDirSize(base)
	freed := before - after
	if freed < 0 {
		freed = 0
	}
	invalidateListCaches()
	writeLog(clientIP(r), "temp_clean", strconv.FormatInt(freed, 10)+"b")
	sendJSON(w, r, 200, map[string]interface{}{"ok": true, "freed": freed})
}

// ---------------------------------------------------------------------------
// GET /api/folder.zip?prefix=Docs — one-click whole-folder download
// ---------------------------------------------------------------------------

// handleAPIFolderZip streams every completed managed guest file under a
// virtual folder prefix (e.g. "Docs" matches "Docs/a.pdf" +
// "Docs/sub/b.jpg") as ONE STORE zip in a single click. Host dir shares
// already stream via GET /r/<id>; this endpoint covers the guest case
// where a folder is N registry entries sharing a rel-path prefix.
// Caps mirror the host zip path (maxZipFiles / maxZipBytes); empty or
// over-limit prefixes return clean JSON instead of a corrupt archive.
func handleAPIFolderZip(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("prefix")
	if raw == "" {
		raw = r.URL.Query().Get("dir")
	}
	prefix := safeRelPath(strings.TrimSpace(raw))
	if prefix == "" {
		shareFail(w, r, 400, "share_bad_path")
		return
	}
	type item struct {
		rel  string
		path string
		size int64
	}
	regMu.Lock()
	snap := make([]*shareEntry, 0, len(shareOrder))
	for _, id := range shareOrder {
		if e, ok := shares[id]; ok {
			cp := *e
			snap = append(snap, &cp)
		}
	}
	regMu.Unlock()
	var list []item
	var total int64
	for _, e := range snap {
		if e.Source != "guest" || !e.Completed {
			continue
		}
		nm := e.Name
		if nm != prefix && !strings.HasPrefix(nm, prefix+"/") {
			continue
		}
		rel := strings.TrimPrefix(nm, prefix+"/")
		if nm == prefix {
			rel = filepath.Base(nm)
		}
		if rel == "" || strings.HasPrefix(rel, ".") || strings.Contains(rel, "..") {
			continue
		}
		rp := guestRealPath(e)
		st, err := os.Stat(rp)
		if err != nil || st.Size() <= 0 {
			continue
		}
		sz := st.Size()
		total += sz
		if total > maxZipBytes {
			shareFail(w, r, 413, "share_too_big")
			return
		}
		list = append(list, item{rel: rel, path: rp, size: sz})
		if len(list) > maxZipFiles {
			shareFail(w, r, 413, "share_too_big")
			return
		}
	}
	if len(list) == 0 {
		shareFail(w, r, 404, "share_not_found")
		return
	}
	sort.Slice(list, func(i, j int) bool { return list[i].rel < list[j].rel })
	_, base := splitParent(prefix)
	if base == "" {
		base = prefix
	}
	zipName := base + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+
		asciiFallbackName(zipName)+"\"; filename*=UTF-8''"+percentEncode(zipName))
	w.Header().Set("Accept-Ranges", "none")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Exact Content-Length when the archive fits zip32 (same math as host).
	var wire int64
	fits := true
	if int64(len(list)) < 65535 {
		var t int64
		add := func(n int64) bool {
			t += n
			return t < 1<<32
		}
		for _, it := range list {
			if it.rel == "" || len(it.rel) > 0xffff || it.size < 0 ||
				uint64(it.size) >= 0xffffffff {
				fits = false
				break
			}
			if !add(30 + 9 + int64(len(it.rel)) + it.size + 16) {
				fits = false
				break
			}
			if !add(46 + 9 + int64(len(it.rel))) {
				fits = false
				break
			}
		}
		if fits && add(22) {
			wire = t
			w.Header().Set("Content-Length", strconv.FormatInt(wire, 10))
		}
	}
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
	buf := getCopyBuf()
	defer putCopyBuf(buf)
	written := 0
	for _, it := range list {
		f, err := os.Open(it.path)
		if err != nil {
			continue
		}
		fh := &zip.FileHeader{Name: it.rel, Method: zip.Store}
		if st, err := f.Stat(); err == nil {
			fh.SetModTime(st.ModTime())
		}
		fw, err := zw.CreateHeader(fh)
		if err != nil {
			f.Close()
			break
		}
		if _, err := io.CopyBuffer(fw, f, buf); err != nil {
			f.Close()
			break
		}
		f.Close()
		written++
	}
	if cerr := zw.Close(); cerr != nil {
		writeLog(clientIP(r), "download_zip_fail", prefix+" ("+truncateRunes(cerr.Error(), 120)+")")
		return
	}
	writeLog(clientIP(r), "download_zip", prefix+" ("+strconv.FormatInt(int64(written), 10)+" files, folder.zip)")
}

// ---------------------------------------------------------------------------
// boot restore: re-register temp files orphaned by a restart
// ---------------------------------------------------------------------------

func registryFull() bool {
	regMu.Lock()
	defer regMu.Unlock()
	return len(shares) >= shareMax
}

// RestoreTempShares re-registers on-disk temp content after a restart (the
// registry itself is memory-only). Must run on boot BEFORE SweepTempOrphans
// so adopted files count as live and are never swept.
//
//  1. Loose non-dot files/dirs in the temp root (same skip rules as the
//     share/list walkers: no dot names, no symlinks, no empty files) come
//     back as host shares with fresh ids and ORIGINAL names (old /r/ links
//     for these expire, but names/sizes/mtimes survive).
//  2. Legacy hidden guest bytes (.shares/<id> with no live entry) are
//     MIGRATED to real managed files (<temp>/restored-<id8>) and registered
//     with full data — no more ghost "restored-xxx" entries pointing at
//     hidden bytes. New uploads never create .shares files.
//
// Never fails the boot: any unreadable dir yields zero restores.
// Returns (loose, adopted) counts.
func RestoreTempShares() (loose, adopted int) {
	base := tempBase()
	if base == "" {
		return 0, 0
	}
	// 1) Loose files/dirs (os.ReadDir is already name-sorted: deterministic).
	if rd, err := os.ReadDir(base); err == nil {
		for _, de := range rd {
			nm := de.Name()
			if strings.HasPrefix(nm, ".") {
				continue
			}
			full := filepath.Join(base, nm)
			if shareBlocked(full) {
				continue
			}
			st, err := os.Lstat(full)
			if err != nil {
				continue
			}
			if st.Mode()&os.ModeSymlink != 0 {
				continue
			}
			var kind string
			var size int64
			if st.IsDir() {
				n, total := dirSizeAbs(full)
				if n == 0 {
					continue
				}
				kind, size = "dir", total
			} else if st.Mode().IsRegular() {
				if st.Size() == 0 {
					continue
				}
				kind, size = "file", st.Size()
			} else {
				continue
			}
			if registryFull() {
				break
			}
			id := newShareID()
			e := &shareEntry{
				ID: id, Name: nm, Kind: kind, Size: size,
				Owner: "host", OwnerID: "host", Source: "host",
				HostPath: full, Created: nowUnix(),
			}
			regMu.Lock()
			shares[id] = e
			shareOrder = append(shareOrder, id)
			regMu.Unlock()
			invalidateShareStat(id)
			loose++
		}
	}
	// 2) Legacy hidden guest bytes: migrate to REAL managed files so they
	// are downloadable with full data and deletable like everything else.
	sd := sharesDir()
	rd, err := os.ReadDir(sd)
	if err != nil {
		return loose, adopted
	}
	regMu.Lock()
	live := make(map[string]bool, len(shares))
	for id := range shares {
		live[id] = true
	}
	regMu.Unlock()
	for _, de := range rd {
		if de.IsDir() {
			continue
		}
		id := de.Name()
		// Skip sidecars / temp junk.
		if strings.HasSuffix(id, ".json") || strings.HasSuffix(id, ".tmp") {
			continue
		}
		if live[id] {
			continue
		}
		full := filepath.Join(sd, id)
		st, err := os.Lstat(full)
		if err != nil {
			continue
		}
		if st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
			continue
		}
		if st.Size() == 0 {
			continue
		}
		if registryFull() {
			break
		}
		short := id
		if len(short) > 8 {
			short = short[:8]
		}
		managedName := "restored-" + short
		// Move to a real managed file (unique when colliding).
		uploadLock.Lock()
		dest := uniquePath(base, managedName)
		uploadLock.Unlock()
		if err := os.Rename(full, dest); err != nil {
			// EXDEV: copy then remove.
			if in, err := os.Open(full); err == nil {
				func() {
					defer in.Close()
					uploadLock.Lock()
					defer uploadLock.Unlock()
					out, cerr := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
					if cerr != nil {
						return
					}
					if _, cerr := io.Copy(out, in); cerr != nil {
						out.Close()
						_ = os.Remove(dest)
						return
					}
					out.Close()
					_ = os.Remove(full)
				}()
				if _, serr := os.Stat(dest); serr != nil {
					continue
				}
			} else {
				continue
			}
		}
		dst, err := os.Stat(dest)
		if err != nil || dst.Size() <= 0 {
			continue
		}
		useID := newShareID()
		e := &shareEntry{
			ID: useID, Name: managedName, Kind: "file",
			Size: dst.Size(), Owner: "guest", OwnerID: "",
			Source: "guest", Completed: true, Created: float64(dst.ModTime().UnixNano()) / 1e9,
			HostPath: dest,
		}
		regMu.Lock()
		shares[useID] = e
		shareOrder = append(shareOrder, useID)
		regMu.Unlock()
		invalidateShareStat(useID)
		live[useID] = true
		adopted++
	}
	return loose, adopted
}

// ---------------------------------------------------------------------------
// orphan sweep: startup + every presence sweep
// ---------------------------------------------------------------------------

// SweepTempOrphans deletes temp session data older than 24h with no live
// share/session. Legacy .uploads staging goes through SweepUploads;
// legacy hidden guest files (.shares/<id>) with no live registry entry are
// removed too (new uploads never land there — boot migrates orphans to real
// managed files). Manual clean (POST /api/temp/clean) is the only other
// deletion path — tempMaxBytes never auto-deletes.
func SweepTempOrphans() {
	SweepUploads()
	base := tempBase()
	if base == "" {
		return
	}
	const maxAge = 24 * 3600.0
	now := nowUnix()
	sd := sharesDir()
	rd, err := os.ReadDir(sd)
	if err != nil {
		return
	}
	regMu.Lock()
	live := make(map[string]bool, len(shares))
	for id := range shares {
		live[id] = true
	}
	// Managed real files (HostPath) are never swept here — only legacy
	// hidden paths without a live entry.
	managed := map[string]bool{}
	for _, e := range shares {
		if e.HostPath != "" {
			managed[filepath.Clean(e.HostPath)] = true
		}
	}
	regMu.Unlock()
	for _, de := range rd {
		if de.IsDir() {
			continue
		}
		id := de.Name()
		if strings.HasSuffix(id, ".json") || strings.HasSuffix(id, ".tmp") {
			continue
		}
		if live[id] {
			continue
		}
		full := filepath.Join(sd, id)
		if managed[filepath.Clean(full)] {
			continue
		}
		st, err := os.Stat(full)
		if err != nil {
			continue
		}
		if now-float64(st.ModTime().UnixNano())/1e9 < maxAge {
			continue
		}
		_ = os.Remove(full)
	}
}
