package beamcore

// Central tunables: single source of truth (no magic numbers scattered).
//
// Defaults preserve the exact v1.6.x behavior (zero behavior change).
// Every value can be overridden via BEAM_* env (parsed at startup via
// ApplyEnvOverrides) — e.g. BEAM_PORT=2004, BEAM_PIECE_MAX=16777216,
// BEAM_IDLE_TIMEOUT=5h, BEAM_CHUNK_MAX_V2=67108864.

import (
	"os"
	"strconv"
	"time"
)

const (
	uploadsDirname = ".uploads"
	hashcacheName  = ".hashcache.json"
)

// Byte-size tunables (vars so tests + env can adjust).
var (
	uploadTTLSec = 24 * 3600
	copyBufSize  = 1024 * 1024

	pieceMin     = 256 * 1024
	pieceMax     = 16 * 1024 * 1024
	// Single fast+reliable mode: 4MB verified pieces, 4 lanes. No more
	// reliable-vs-turbo choice in the UI — speed comes from parallelism,
	// safety from always-on per-piece checksums.
	pieceDefault = 4 * 1024 * 1024

	// Per-request caps (previously hardcoded 64M/128M in uploads_http.go).
	chunkMaxV2     = 64 * 1024 * 1024
	multipartMax   = 128 * 1024 * 1024
	multipartSlack = 1024

	// JSON body caps (previously bare 1<<20 / 10<<20).
	jsonSmallMax = int64(1 << 20)
	jsonBigMax   = int64(10 << 20)

	hashCacheMax = 500
	uniqueTries  = 10000

	// maxSessionPieces caps the V2 piece count per session (giant-session
	// guard: init with needN beyond this is rejected with 413).
	maxSessionPieces = 200000
)

// Folder-sharing caps (vars so tests can lower them).
var (
	maxRelDepth  = 10
	maxRelLen    = 512
	maxTreeFiles = 1000
	maxListFiles = 5000
	maxZipFiles  = 2000
	maxZipBytes  = int64(4) * 1024 * 1024 * 1024
)

// Registry-based sharing tunables (vars so tests + env can adjust).
var (
	// shareMax caps the live registry entries (POST /api/share guest+host).
	// One entry per file: a whole folder uploads as N entries, so the
	// default leaves headroom for full-folder one-click sharing
	// (still env-tunable via BEAM_SHARE_MAX).
	shareMax = 2000
	// relayWaitSec bounds a waiting /r/ download for owner relay bytes.
	relayWaitSec = 30
	// presenceTTLsec marks a guest owner absent after this many seconds
	// without a /api/presence heartbeat. Long enough that a hidden tab,
	// a sleeping phone, or a short network drop does NOT flip files to
	// "unavailable" — completed files are served from disk anyway.
	presenceTTLsec = 180
	// tempMaxBytes is a WARN-ONLY threshold for the temp dir, reported by
	// GET /api/temp as "warn_bytes". No auto-delete ever happens —
	// cleanup is manual via POST /api/temp/clean (product decision).
	tempMaxBytes = int64(2) * 1024 * 1024 * 1024
)

// Network / timing tunables.
var (
	netcapTTLSec = 30.0
	lanIPTTLSec  = 8.0

	// TLS tunables (optional self-signed HTTPS for LAN/hotspot).
	// Default is plain HTTP; enable with --tls / BEAM_TLS=1.
	tlsEnabledDefault = false

	httpReadHeaderTimeout = 30 * time.Second
	// httpReadTimeout is retained for header reads / compat only: the
	// server sets ReadTimeout 0 so slow request bodies (big uploads over
	// slow LAN) are never killed mid-body; headers stay bounded by
	// ReadHeaderTimeout above.
	httpReadTimeout       = 30 * time.Second
	httpIdleTimeout       = 120 * time.Second
	httpMaxHeaderBytes    = 1 << 20

	idleTickInterval = 15 * time.Second

	memLogCap = 500
)

// LimitsSnapshot exposes the tunables the web UI needs so it never
// hardcodes a copy (piece sizes, caps, poll intervals).
func LimitsSnapshot() map[string]interface{} {
	return map[string]interface{}{
		"port":            ServerPort,
		"piece_min":       pieceMin,
		"piece_max":       pieceMax,
		"piece_default":   pieceDefault,
		"chunk_max_v2":    chunkMaxV2,
		"multipart_max":   multipartMax,
		"max_file_mb":     cfgSnapshot().MaxFileMB,
		"max_rel_depth":   maxRelDepth,
		"max_zip_files":   maxZipFiles,
		"max_zip_bytes":   maxZipBytes,
		"share_max":       shareMax,
		"relay_wait_sec":  relayWaitSec,
		"presence_ttl":    presenceTTLsec,
		"download_conc":   downloadMaxConc,
		"temp_warn_bytes": tempMaxBytes,
		"search_max":      200,
		"poll_status_ms":  10000,
		"poll_clients_ms": 5000,
		"poll_files_ms":   5000,
	}
}

func envInt(name string, cur int) int {
	s := os.Getenv(name)
	if s == "" {
		return cur
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return cur
	}
	return v
}

func envInt64(name string, cur int64) int64 {
	s := os.Getenv(name)
	if s == "" {
		return cur
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return cur
	}
	return v
}

func envFloat(name string, cur float64) float64 {
	s := os.Getenv(name)
	if s == "" {
		return cur
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return cur
	}
	return v
}

func envDur(name string, cur time.Duration) time.Duration {
	s := os.Getenv(name)
	if s == "" {
		return cur
	}
	if v, err := time.ParseDuration(s); err == nil {
		return v
	}
	if secs, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Duration(secs) * time.Second
	}
	return cur
}

// ApplyEnvOverrides reads BEAM_* env into every tunable (idempotent).
// Invalid values fall back to current (never crash boot on a typo).
func ApplyEnvOverrides() {
	ServerPort = envInt("BEAM_PORT", ServerPort)
	if ServerPort < 1 || ServerPort > 65535 {
		ServerPort = defaultPort
	}
	defaultPort = ServerPort
	IdleTimeout = envDur("BEAM_IDLE_TIMEOUT", IdleTimeout)

	copyBufSize = envInt("BEAM_COPY_BUF", copyBufSize)
	if copyBufSize < 32*1024 {
		copyBufSize = 32 * 1024
	}
	if copyBufSize > 8*1024*1024 {
		copyBufSize = 8 * 1024 * 1024
	}
	pieceMin = envInt("BEAM_PIECE_MIN", pieceMin)
	pieceMax = envInt("BEAM_PIECE_MAX", pieceMax)
	pieceDefault = envInt("BEAM_PIECE_DEFAULT", pieceDefault)
	if pieceMin < 64*1024 {
		pieceMin = 64 * 1024
	}
	if pieceMax < pieceMin {
		pieceMax = pieceMin
	}
	if pieceDefault < pieceMin || pieceDefault > pieceMax {
		pieceDefault = 4 * 1024 * 1024
	}
	chunkMaxV2 = envInt("BEAM_CHUNK_MAX_V2", chunkMaxV2)
	multipartMax = envInt("BEAM_MULTIPART_MAX", multipartMax)
	uploadTTLSec = envInt("BEAM_UPLOAD_TTL_SEC", uploadTTLSec)
	hashCacheMax = envInt("BEAM_HASHCACHE_MAX", hashCacheMax)
	uniqueTries = envInt("BEAM_UNIQUE_TRIES", uniqueTries)
	if uniqueTries < 100 {
		uniqueTries = 100
	}

	maxRelDepth = envInt("BEAM_MAX_REL_DEPTH", maxRelDepth)
	maxRelLen = envInt("BEAM_MAX_REL_LEN", maxRelLen)
	maxTreeFiles = envInt("BEAM_MAX_TREE_FILES", maxTreeFiles)
	maxListFiles = envInt("BEAM_MAX_LIST_FILES", maxListFiles)
	maxZipFiles = envInt("BEAM_MAX_ZIP_FILES", maxZipFiles)
	maxZipBytes = envInt64("BEAM_MAX_ZIP_BYTES", maxZipBytes)

	shareMax = envInt("BEAM_SHARE_MAX", shareMax)
	if shareMax < 1 {
		shareMax = 1
	}
	relayWaitSec = envInt("BEAM_RELAY_WAIT_SEC", relayWaitSec)
	if relayWaitSec < 1 {
		relayWaitSec = 1
	}
	presenceTTLsec = envInt("BEAM_PRESENCE_TTL_SEC", presenceTTLsec)
	if presenceTTLsec < 1 {
		presenceTTLsec = 1
	}
	tempMaxBytes = envInt64("BEAM_TEMP_MAX_BYTES", tempMaxBytes)
	if tempMaxBytes < 0 {
		tempMaxBytes = 0
	}

	netcapTTLSec = envFloat("BEAM_NETCAP_TTL", netcapTTLSec)
	lanIPTTLSec = envFloat("BEAM_LANIP_TTL", lanIPTTLSec)

	httpReadHeaderTimeout = envDur("BEAM_HTTP_READHEADER_TIMEOUT", httpReadHeaderTimeout)
	httpReadTimeout = envDur("BEAM_HTTP_READ_TIMEOUT", httpReadTimeout)
	httpIdleTimeout = envDur("BEAM_HTTP_IDLE_TIMEOUT", httpIdleTimeout)
	idleTickInterval = envDur("BEAM_IDLE_TICK", idleTickInterval)
	memLogCap = envInt("BEAM_MEMLOG_CAP", memLogCap)
	if memLogCap < 50 {
		memLogCap = 50
	}
	if v := envInt("BEAM_DOWNLOAD_CONC", downloadMaxConc); v != downloadMaxConc {
		setDownloadSemSize(v)
	} else if downloadMaxConc < 8 || downloadMaxConc > 256 {
		setDownloadSemSize(downloadMaxConc)
	}
}
