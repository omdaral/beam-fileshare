# Project Constitution - Hotspot File Sharing App

> This folder is the **single source of truth** for the project. Every decision or change must come back here first.
> Don't get lost: if something isn't written here, discuss and document it before implementing it.

## Project in Brief
A desktop app (Windows + Linux) that creates a company-private Hotspot network,
and any phone or computer that joins it can **send and receive** files from the browser
without installing anything, without internet, and without libraries.

## Constitution Map

| No. | File | Answers the question |
|-----|-------|-----------------|
| 01 | `01-Goals.md` | Why are we building the app? What is success? What is out of scope? |
| 02 | `02-User-Journey.md` | How does the user move step by step? |
| 03 | `03-Identity-Design.md` | What does the app look like, its identity, and design rules? |
| 04 | `04-Architecture.md` | How will we build it technically? With which language and components? |

## Golden Rules (constitution above the constitution)

1. **One click runs it:** The regular user is not technical. Running the app = double-click only.
2. **Zero install for clients:** Joining phones and computers use only the browser.
3. **Zero libraries on the user's machine:** The final `exe` / `binary` is a single file with everything inside.
4. **Offline first:** The app must work with no internet at all.
5. **Send + receive:** Not a view-only app. Upload and download both ways.
6. **Windows + Linux:** Every feature must work on both systems or have a documented alternative.
7. **Any change = constitution update:** No code may contradict these files. If we must diverge, update the file first.

## Project Status
- [x] Idea and agreement
- [x] Project constitution (this folder)
- [x] Phase 1: Go server + web page (`goserver/` + passing `go test` tests)
- [x] Phase 2: Windows + Linux Hotspot module + QR (`goserver/hotspot*.go` + `/api/status` + dual Wi-Fi/link QR)
- [x] Phase 3: Browser-based management instead of desktop window (localhost manager + remote code + Arabic messages)
- [x] Phase 4: Build (`build.sh`/`build.bat` + `Beam.sh`/`Beam.bat`/`install.sh`;
  static binary for each system — no Python, no Go, no libraries on the user's machine.
  After copying the folder anywhere, run `./install.sh` in the new location)
- [x] Full Python → Go replacement: all Python files deleted (`fileshare.py`/`hotspot.py`/`desktop.py`/tests/`vendor/`)
- [x] Web simplification: direct entry with no code (security = Wi-Fi password) + atomic upload + safe delete (v1.0.2)
- [x] Large files (no size limit by default): parallel chunked upload + fingerprints + resume + auto refresh + LAN/Hotspot/Open (v1.1.0, single fast+reliable mode since v1.6.x)
- [x] Operation clarity: v1.2.1 version stamp + in-browser Admin section + single icon
- [x] Landing page + owner settings window (tabbed Modal) + default visitor language (v1.3.0)
- [x] Folder sharing: collapsible tree + lightweight search + zip download + recursive delete + over-limit zip queue with reason (v1.4.0)
- [x] Always-zipped folders + single fast+reliable transfer mode + isolated background extraction (v1.5.0 modes unified in v1.6.x; `noverify` remains as a legacy API flag only)
- [ ] In-company testing (checklist in `TEST_CHECKLIST.md` — real trial + `go -C goserver test ./...` on the build machine)

## Unified Terms
- **Host computer:** The machine running the app and hosting the Hotspot.
- **Client:** Any phone or computer joining the network from the browser.
- **LAN Mode:** Fallback mode if Hotspot fails; we run on the existing Wi-Fi.
