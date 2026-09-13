# Desktop Window — Removed (DEPRECATED)

> The tkinter window (`desktop.py`) was removed permanently as part of the full replacement with Go.
> Replacement: **network and device management section in the browser** — shown automatically on the host machine
> (`localhost` is manager), and any admin request from elsewhere is rejected (`403` — no remote access at all).

User journey (≤ 3 clicks per `docs/02`): double-click the icon ← **start networking** from the browser ← share the QR.

Current reference: `README.md` + admin source in `goserver/auth.go` + `goserver/status.go` + `goserver/net_admin.go` + `goserver/config_admin.go` and its UI in `goserver/web/index.html`.
