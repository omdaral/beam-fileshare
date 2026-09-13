package beamcore

import (
	"os/exec"
	"runtime"
)

// OpenBrowser opens the Beam web UI in the default browser.
// It never blocks and never fails the server: errors are ignored
// (headless machines simply keep running without a browser).
func OpenBrowser(url string) {
	var cmds [][]string
	switch runtime.GOOS {
	case "windows":
		cmds = [][]string{
			{"rundll32", "url.dll,FileProtocolHandler", url},
			{"cmd", "/c", "start", "", url},
		}
	case "darwin":
		cmds = [][]string{{"open", url}}
	default: // linux and the rest
		cmds = [][]string{
			{"xdg-open", url},
			{"gio", "open", url},
			{"sensible-browser", url},
		}
	}
	for _, c := range cmds {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		cmd := exec.Command(c[0], c[1:]...)
		_ = cmd.Start()
		// Best effort: release the process without waiting.
		if cmd.Process != nil {
			_ = cmd.Process.Release()
		}
		return
	}
}
