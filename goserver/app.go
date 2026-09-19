package beamcore

import (
	"regexp"
	"sync"
	"time"
)

// Global runtime state.
// NOTE: Beam keeps zero side files next to the user: no config.json, no
// logs/ directory. Settings live in memory for the session (the owner's
// browser keeps a copy in localStorage and re-applies it), the access log
// is an in-memory ring, and shared files live in ~/Downloads/Beam.
var (
	BaseDir   string
	SharedDir string
	// TempDir is the one and only writable location: ~/Downloads/Beam-Temp
	// (visible, no dot prefix). All chunked-upload staging (.uploads/),
	// retained guest bytes (.shares/) and the TLS cert (.beam-tls/) live
	// under it. SharedDir is kept only as a deprecated alias that always
	// points at the same path (legacy helpers still read it).
	TempDir string
	// AppVersion is injected at build time via:
	//   go build -ldflags "-X fileshare.AppVersion=$VER"
	// Fallback matches VERSION file so dev runs (go run) still report right.
	AppVersion = "1.6.0"
	ServerPort = 2004

	// IsPhoneBuild is true when the engine runs inside the Android app
	// (goserver/beamapp). The web UI uses it to hide the desktop hotspot
	// form (custom SSID/password are impossible with Android Local-Only
	// Hotspot — the OS picks the credentials) and show the phone flow.
	IsPhoneBuild = false

	// defaultPort is the fixed, bookmarkable Beam port (http://<ip>:2004).
	defaultPort = 2004

	// IdleTimeout closes the server after this long without any HTTP
	// activity (any request except /health resets the timer).
	// Zero disables the auto shutdown. Overridable via --idle-timeout.
	IdleTimeout = 5 * time.Hour

	CfgMu sync.RWMutex
	Cfg   Config

	NSMu sync.RWMutex
	Net  = NetState{SSID: "Beam", LanMode: true, Security: "wpa"}

	uploadLock sync.Mutex // naming + final save + hashcache (like _UPLOAD_LOCK)

	sessMu    sync.Mutex
	sessLocks = map[string]*sync.Mutex{}

	netcapMu      sync.Mutex
	netcapAt      float64
	netcapAvail   bool
	netcapCapable bool

	uuidRe = regexp.MustCompile(`^[0-9a-zA-Z_-]{8,64}$`)
	hexRe  = regexp.MustCompile(`^[0-9a-fA-F]+$`)
)

// NetState mirrors fileshare.py NET_STATE.
type NetState struct {
	SSID           string
	LanMode        bool
	HotspotRunning bool
	Security       string
	WifiPassword   string
}
