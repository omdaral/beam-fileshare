# Changelog

All notable changes to Beam are documented here.
The format is inspired by [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

### Added

- `DOWNLOAD.md` — full per-OS download matrix (6 portables + APK + deb/AppImage/rpm) with direct `releases/latest/download/` links, arch detection help, SHA256 verification, and a 404 troubleshooting section (repo must be Public + tag pushed).
- `get-beam.sh` — one-click downloader: detects OS/arch via `uname`, downloads the matching bundle (pinned `v1.6.0` with `latest/` fallback), extracts it, and prints next steps. `bash -n` clean.
- `README.md` / `README_AR.md` — "Download (ready to use)" sections now point to GitHub Releases instead of gitignored local `dist/` paths, plus the one-click script command.
- `HELP.md` — new section ٠ "Downloading the right file" (script + manual + 404 cause + SHA check); AppImage alternative and Android APK rows now link to Releases.
- `TEST_CHECKLIST.md` — download checks: all DOWNLOAD.md assets resolve + `get-beam.sh --no-extract` smoke test.
- `README_AR.md` — full Arabic translation of the README with simplified install
  (Linux / Windows / Mac / Android), explicit warning to rerun `./install.sh`
  after every folder copy, `Beam` binary vs `Beam.desktop` icon explanation,
  and `~/Downloads/Beam` vs `Beam-Temp` folder explanation.
- `HELP.md` (earlier) — central help file (Arabic + English) with a Troubleshooting table:
  port 2004 busy, firewall, Allow Launching, browser not opening, AppImage
  needs libfuse2, hotspot needs pkexec/Admin, Wi-Fi drop during upload,
  Mac Gatekeeper and Windows SmartScreen.
- `docs/screenshots/README.md` — placeholder guide for the 3 required
  screenshots (homepage + QR, mobile, file tree) until real images are added.
- `LICENSE` — MIT license (Ahmed Faseh, 2026).

### Fixed

- `.github/workflows/release.yml` — Release assets narrowed to uniquely named
  deliverables (`*.tar.gz` / `*.zip` / `*.deb` / `*.AppImage` / `*.rpm` /
  `*.apk` + `MANIFEST.json` / `SHA256SUMS` / `versions.json`); raw
  `dist/linux|windows|darwin/<arch>/<ver>/Beam[.exe]` trees stay as run
  Artifacts only (their shared `Beam` / `Beam.exe` basenames would collide as
  flat Release assets).
- `.github/workflows/release.yml` — Pillow install now tries
  `--break-system-packages` first (PEP 668 on ubuntu 24.04 runners) with a
  fallback to plain `pip3 install`.
- `.github/workflows/release.yml` — AppImage runtime fetch uses `curl -fSL`
  plus `test -s` guards so HTTP errors fail fast instead of producing a
  mystery failure later in `build-appimage.sh`.

### Changed

- `README.md` — "Download (ready to use)" table now points to GitHub Releases
  (`releases/latest/download/`) + one-click `get-beam.sh` command, links to
  `README_AR.md` / `HELP.md` / `DOWNLOAD.md`, `Beam.prev` rollback-only
  clarification, and the Host-copies-to-`~/Downloads/Beam` line.

## [1.6.0] — 2026

Current version (see `VERSION`).

- Version badge on the home page.
- Portable builds per OS under `dist/portables/1.6.0/`.
- Android APK at `dist/mobile/Beam-1.6.0-android.apk` (see `mobile/SETUP.md`).
- Host share folder `~/Downloads/Beam`; unified registry rooted at `~/Downloads/Beam-Temp`.
- Session-only settings, in-memory log (last 500 lines), 5-hour idle auto-shutdown.
