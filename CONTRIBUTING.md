# Contributing to Beam

Thanks for your interest in contributing! Beam is a small, offline-first file-sharing app (Go + vanilla web UI + optional Capacitor Android wrapper). We keep the process lightweight.

## Quick start for contributors

```bash
# 1. Run the server (needs Go once)
go -C goserver run ./cmd/beam --no-browser

# 2. Run all tests
go -C goserver test ./...

# 3. Full local build (all 6 platform binaries + portables)
./build-all.sh
```

- Source of truth for behavior: `goserver/` + `goserver/web/`
- Version lives in one place: `VERSION` (tag must be `v<VERSION>`, enforced by CI)
- Android wrapper source: `mobile/src/`, `mobile/*.json`, `mobile/*.sh`, `mobile/scripts/` (everything else under `mobile/` is regenerable)
- Packaging scripts: `packaging/`

## What to read first

- `README.md` (user guide) + `README_AR.md` (Arabic)
- `HELP.md` (troubleshooting) + `DOWNLOAD.md` (release matrix)
- `docs/` (project constitution: goals, journeys, architecture)
- `api_spec.md` (developer API)

## Pull request rules

1. **One change per PR** — keep it focused and explain *why*, not just *what*.
2. **Tests:** `go -C goserver vet ./...` and `go -C goserver test ./...` must pass. Add/adjust tests when you change behavior.
3. **Docs:** update `README.md` / `HELP.md` / `CHANGELOG.md` (`[Unreleased]`) when user-visible behavior changes. Keep `README_AR.md` in sync for user-facing changes.
4. **No binaries:** never commit `dist/`, `Beam`, `*.apk`, `*.deb`, `*.AppImage`, `mobile/android/`, `mobile/node_modules/`, or `*.desktop` files. They are gitignored build outputs released via GitHub Releases only.
5. **No secrets:** never commit `.env`, keys, certs, or personal paths. Use placeholders (`YourStrongPass123`, `192.168.1.5` examples only).
6. **Style:** Go stdlib first, small diffs, no new dependencies without discussion (open an issue first).

## Reporting bugs

Use the **Bug report** issue template. Include: OS/arch, Beam version (`VERSION` + tag), exact command, browser, what you expected vs. what happened, and the relevant lines from the in-app log (Devices & Log tab).

## Feature requests

Use the **Feature request** template. New ideas must pass the 3 constitution questions in `docs/01-Goals.md`: preserves "no install"? works offline? works on Windows + Linux (or has a documented alternative)?

## Release process (maintainers)

- Bump `VERSION`, update `CHANGELOG.md`, commit, tag `v<VERSION>`, `git push origin main v<VERSION>`
- The `Release` workflow builds everything, runs the smoke test, and publishes to GitHub Releases with `SHA256SUMS` + `MANIFEST.json`
- See `DOWNLOAD.md` §5 if the Releases page shows 404 (repo must be Public + tag pushed)
