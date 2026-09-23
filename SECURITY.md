# Security Policy

## Supported versions

| Version | Supported          |
| ------- | ------------------ |
| 1.6.x   | :white_check_mark: |
| < 1.6   | :x:                |

We support the latest `v1.6.x` release published on the [Releases page](https://github.com/omdaral/beam-fileshare/releases/latest). Older builds are not patched — please upgrade with `get-beam.sh` or from `DOWNLOAD.md`.

## Security model (please read)

Beam is a **local-network app**. There are no accounts and no cloud:

- **The only access control is the Wi-Fi / hotspot password.** Anyone who joins your network can open `http://<ip>:2004`, upload, download, and delete.
- Default transport is plain HTTP on port `2004` (no certificate warnings). For encrypted LAN traffic run with `--tls` (`BEAM_TLS=1`) and verify the printed SHA-256 fingerprint.
- Settings are session-only, the log is in-memory (last 500 lines), shares live in `~/Downloads/Beam`. There is no `config.json` or log file to leak.
- The Android wrapper uses cleartext LAN + `allowNavigation` for private ranges by design (see `mobile/capacitor.config.json`).

Choose your network carefully. Do not run Beam on a network you do not trust.

## Reporting a vulnerability

**Do not open a public issue for security reports.**

- Open a [private security advisory](https://github.com/omdaral/beam-fileshare/security/advisories/new) or contact the maintainer via the profile email.
- Include: Beam version, OS/arch, exact command/flags, network mode (LAN/hotspot/TLS?), steps to reproduce, and impact.
- We aim to acknowledge within 72 hours, fix in a patch release, and credit reporters in `CHANGELOG.md` (unless you prefer to stay anonymous).

## Out of scope

- Social-engineering someone's Wi-Fi password, physical access to an unlocked host, or an attacker already on your LAN sniffing plain HTTP (use `--tls` if this matters to you).
- Third-party dependencies' own advisories — please report those upstream, then link them to us if Beam is affected.
