# Beam — Packaging Notes (PACKAGING NOTES)
Version: `1.6.0` — Current scope: Go server + hotspot + web page + browser management.

## 1) What does packaging build?
- Input: `goserver/` (stdlib only — zero dependencies).
- Output:
  - Linux: `Beam` next to `Beam.sh` (launcher) and `Beam.desktop` (icon).
  - Windows: `Beam.exe` next to `Beam.bat`.
- Binary is fully static (`CGO_ENABLED=0`) — **no glibc, no DLL, no Python, and no pip on the user machine**.
- Web page is embedded in the binary (`go:embed goserver/web/index.html`).
- At runtime: **no sidecar files** — no `config.json`, no `logs/`, and no `Shared/` next to the executable.
  The only files: `~/Downloads/Beam` (well-known share folder) + `/tmp/beam-server.log` log for launchers only.

## 2) Compatibility
- Linux: any modern amd64 distro (static — no specific glibc required). Build with `./build.sh`.
- Windows: build the exe on any system with `GOOS=windows` (or `build.bat` on Windows). Hotspot there uses WinRT first, then netsh.
- Running on the target machine needs no Go, no Python, no pip, and no Admin (Admin only the first time for hotspot/firewall).

## 3) Known limits
1. **Windows SmartScreen:** unsigned exe → `More info` → `Run anyway` the first time (normal for internal tools).
2. **Reserved port:** port is fixed at `2004` — if busy the program prints an Arabic message and suggests an alternate port (exit=2) — fix: `Beam --port 2005`.
3. **Folder move:** `.desktop` paths are absolute by spec nature — after every copy run `./install.sh`
    in the new location to regenerate the correct paths and auto-validate the syntax.

## 4) Quick commands
```bash
./build.sh                        # check + tests + platform matrix in dist/<os>/<arch>/<version>/
./install.sh                      # after every folder copy: regenerates icon paths and checks them
./Beam.sh --no-browser       # run from terminal without opening the browser
curl -s http://127.0.0.1:2004/health      # verify: {"ok": true}
go -C goserver test ./...                 # all tests
```
```bat
REM Windows:
build.bat
Beam.exe
```
