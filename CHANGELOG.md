# Changelog

All notable changes to Beam are documented here.
The format is inspired by [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

### Added

- `README_AR.md` — full Arabic translation of the README with simplified install
  (Linux / Windows / Mac / Android), explicit warning to rerun `./install.sh`
  after every folder copy, `Beam` binary vs `Beam.desktop` icon explanation,
  and `~/Downloads/Beam` vs `Beam-Temp` folder explanation.
- `HELP.md` — central help file (Arabic + English) with a Troubleshooting table:
  port 2004 busy, firewall, Allow Launching, browser not opening, AppImage
  needs libfuse2, hotspot needs pkexec/Admin, Wi-Fi drop during upload,
  Mac Gatekeeper and Windows SmartScreen.
- `docs/screenshots/README.md` — placeholder guide for the 3 required
  screenshots (homepage + QR, mobile, file tree) until real images are added.
- `LICENSE` — MIT license (Ahmed Faseh, 2026).

### Changed

- `README.md` — new "Download per OS" table at the top pointing to
  `dist/portables/` and `dist/mobile/*.apk` plus Add to Home Screen for
  iPhone, links to `README_AR.md` and `HELP.md`, `Beam.prev` rollback-only
  clarification, and the Host-copies-to-`~/Downloads/Beam` line.

## [1.6.0] — 2026

Current version (see `VERSION`).

- Version badge on the home page.
- Portable builds per OS under `dist/portables/1.6.0/`.
- Android APK at `dist/mobile/Beam-1.6.0-android.apk` (see `mobile/SETUP.md`).
- Host share folder `~/Downloads/Beam`; unified registry rooted at `~/Downloads/Beam-Temp`.
- Session-only settings, in-memory log (last 500 lines), 5-hour idle auto-shutdown.
