# 02 - User Journey

> Every screen or button must serve one of these journeys. No extra buttons.

## Personas
- **Ahmed (host device owner):** An IT employee or manager with a Windows or Linux laptop; wants zero hassle.
- **Mona (employee):** Has an Android or iPhone; wants to send a PDF or receive work photos and leave.

---

## Journey 1: Ahmed turns on the network (first time)
1. Double-clicks `Beam` (first run asks for Admin/sudo permission — a message in Arabic explains why) — the browser opens automatically to the UI.
2. From Settings chooses **network mode**: Wi-Fi (LAN) or Hotspot — with network name and password (or an open network on Linux).
3. Presses the big Start button -> the app starts the selected mode + server.
4. He sees:
   - Network name and password in large type (or an open-network warning if he chose one — literal text: "شبكة مفتوحة")
   - Wi-Fi QR Code + QR for the files page link
   - Status: literal text "شغال - متصل N أجهزة" ("Running - N devices connected")

**Rule:** Fewer than 3 clicks from open to running. No intermediate screens.

## Journey 2: Mona joins and sends a file (client - mobile)
1. Opens Wi-Fi, finds the `Beam` network, enters the password (or scans the QR).
2. Scans the second QR or types in the browser: `http://192.168.137.1:2004` (port is fixed at 2004).
3. Lands **directly on file exchange** — no entry code; the network password is the only security.
4. After entry she sees:
   - Large **"ارفع ملفات"** ("Upload files") button + drag-and-drop area
   - List of available files for download with size and a download button per file
5. Selects a file (no size limit by default) and presses upload -> sees a % progress bar -> message "تم الرفع بنجاح" ("Upload completed successfully").
   - If the connection drops: re-select the same file and it resumes where it stopped (even after refreshing the page).
   - The list auto-refreshes every 5 seconds — no refresh button.
6. Downloads a file Ahmed shared -> it saves to her phone.

**Rule:** Mona installs nothing. Everything from the browser. If she needs to install -> design failure.

## Journey 3: Desktop employee sends and receives
Exactly the same as Mona's journey, but from a desktop browser (Chrome/Edge). Can drag a full folder or large files.

## Journey 4: Ahmed monitors and shuts down
- Sees connected device count and names (IP).
- Sees a log: e.g. "منى رفعت تقرير.pdf - 2MB - 10:15" ("Mona uploaded report.pdf - 2MB - 10:15") / "كريم نزل صور.zip" ("Karim downloaded photos.zip").
- Can delete an abusive file from the `Downloads/Beam` folder (or from the page itself).
- Presses the round ⏻ **"إيقاف السيرفر"** ("Stop server") button at the top of the page (one click) -> server stops and the tab closes.
- If he forgets: the server shuts itself down automatically after 5 hours without activity.

---

## Failure scenarios (must handle)
| Problem | User feeling | Required fix in the app |
|---------|--------------|---------------------------|
| Wi-Fi card doesn't support Hotspot | "The app isn't working" | Clear message in Arabic + alternative button "شغل وضع LAN" ("Run LAN mode") that works on existing Wi-Fi |
| Forgot the password | "I can't get in" | "إظهار الباسورد" ("Show password") button + QR always visible |
| Disconnect during 500MB upload | "The file is lost" | Resume the upload or at least a clear message + no corruption of old files |
| Stranger joined the network | Security risk | Entry code for files page + ability to kick IP + change password |
| Port blocked by firewall | Page won't open | App opens the port automatically on first run or explains in one step |

## Binding rules for implementation
1. Every journey must complete without verbal explanation - the UI explains itself.
2. Every error message must be in Arabic in solution form: "[what happened]... [do this]".
3. QR is the main bridge - must be large, clear, and printable.
