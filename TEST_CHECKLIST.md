# Beam — Mandatory Test Checklist (TEST CHECKLIST)
> Reference: `docs/04-Architecture.md` section 7 + success metrics in `docs/01-Goals.md`.
> Printable: copy it on paper and mark ✅ / ❌ for each item. Environment: LAN or Hotspot, default port `2004`.

## 0) General setup (before every item)
- [ ] Download check: every asset in DOWNLOAD.md resolves (HTTP 200) — 6 portables + APK + deb/AppImage/rpm + SHA256SUMS + MANIFEST.json
- [ ] `bash -n get-beam.sh` passes, and `bash get-beam.sh --no-extract --dir /tmp/beam-dl-test` downloads the correct file for this machine
- [ ] `bash get-beam.sh --install --dir /tmp/beam-install-test` installs into the dir, runs install.sh, and leaves no archive/temp behind (`ls /tmp/beam-dl-*` empty afterwards)
- [ ] Server is running: `./Beam --port 2004` (or `go -C goserver run . --port 2004`) and prints `http://IP:2004` links
- [ ] At startup: prints **device entry link** + shows a system notification with it (Linux)
- [ ] Beam page on the host machine: entry hero in large type at the top of the page + working copy button + ⚙️ Settings button visible in the header
- [ ] ⚙️ button opens a settings dialog with tabs (Network / General / Devices & Log / Device) + closes with Esc and outside click
- [ ] Access is direct with no code (security = Wi-Fi password only)
- [ ] `~/Downloads/Beam` folder is created automatically, and the log is viewed from the web page (Devices & Log tab — memory only, no files)
- [ ] At startup: browser opens automatically on the UI + fixed-size circular ⏻ button showing «إيقاف السيرفر» ("Stop server") in the header (for the owner; one click stops and closes the tab)
- [ ] After 5 hours idle (or `--idle-timeout 1m` for testing) the server shuts itself down
- [ ] Go tests green: `go -C goserver test ./...`
- [ ] One-click publish: `./publish.sh` ends with «تم النشر بنجاح» ("Published successfully") and the new server responds with Beam branding (check `/tmp/beam-server.log` on failure)

## 0-B) Single page + conditional owner zone (mandatory)
```bash
BASE=http://127.0.0.1:2004
# Same page for everyone on all addresses:
for p in / /index.html /admin /guest; do curl -s -o /dev/null -w "$p:%{http_code} " $BASE$p; done; echo
curl -s $BASE/ | grep -c "ownerZone\|Beam"
# Expected: 200 200 200 200 then a count > 0
# From a phone on the network (guest):
# - Same page opens (hero + files + QR) with no ⚙️ button and no settings dialog — and any admin API returns 403
# - If the owner opens via a LAN address they see the guest copy (no ⚙️) — fix: use the tab auto-opened on their machine
# - Clear-log button (Devices & Log tab) fully empties the in-memory log (two-click confirm)
# - Connected-devices list refreshes every 3 seconds in the owner dialog
# - Default language: change it from the "General" tab and save → a new visitor (clean browser) opens with it, and guests can freely switch their own language from the header
# - Folders: upload a folder (button + drag & drop) → collapsible tree → instant search → zip download that extracts cleanly → delete single file → delete whole folder (two clicks) → prune empties
# - Bar cap: upload a large folder (50+ files) → one grouped row with counter and total bar + ≤5 individual bars + finished ones disappear leaving the last 5 → cancel a batch mid-flight and confirm it stops
# - Packages: deb builds and lintian is clean (except maintainer identity) + AppImage builds and runs (health) + manifest/metainfo validated — acceptance gate
# - Always-zipped folder: upload a folder → single zip session → tree appears after completion → single zip file stays as-is with no extraction
# - Modes: upload the same file in the single fast+reliable mode → cut mid-transfer → resume re-picks the same file and continues from the missing chunks → download and compare byte-identical
# - Over-cap folder: shows as a zip row with the reason and does not open as a tree — acceptance gate
# - Scan Wi-Fi QR and link QR with a phone: instant connect/open — acceptance gate
# - Branding: Beam logo visible + light paper theme by default + toggle switches to dark and saves the choice
# - EN/AR language button flips direction and all strings (check the files screen and buttons)
# - QR inside a white frame even in dark mode + Arabic font renders correctly with no internet
```
- [ ] Guest sees no admin button in the page HTML — Result: ✅ / ❌

### Core smoke commands — work from any device on the same network
```bash
BASE=http://127.0.0.1:2004   # change IP to the printed one, and PORT per --port

# 1) Health
curl -s $BASE/health
# Expected: {"ok": true}

# 2) File list (direct with no login)
curl -s $BASE/files
# Expected: {"files": [...]}

# 3) Upload a test file (chunked protocol v1 — standard tools only)
echo "smoke-test" > /tmp/smoke.txt
UUID=$(openssl rand -hex 16)
NAME=smoke.txt; SIZE=$(stat -c%s /tmp/smoke.txt)
curl -s -H 'Content-Type: application/json' -d "{\"uuid\":\"$UUID\",\"name\":\"$NAME\",\"size\":$SIZE}" $BASE/upload_init
curl -s --data-binary @/tmp/smoke.txt "$BASE/upload_chunk?id=$UUID&offset=0"
curl -s -H 'Content-Type: application/json' -d "{\"id\":\"$UUID\"}" $BASE/upload_complete
# Expected: complete: 200 {'ok': true, 'saved': [...]}
# Note: legacy compat path POST /upload (multipart up to 128MB) still works for old tools:
# curl -s -F "files=@/tmp/smoke.txt" $BASE/upload
# Expected: {"ok": true, "saved": [...]}

# 4) Download it and compare
curl -s "$BASE/download?file=smoke.txt" -o /tmp/smoke_dl.txt
diff /tmp/smoke.txt /tmp/smoke_dl.txt && echo SAME || echo DIFFERENT
# Expected: SAME

# 5) Delete + confirm it is gone
curl -s -X POST -H 'Content-Type: application/json' -d '{"file":"smoke.txt"}' $BASE/delete
curl -s "$BASE/download?file=smoke.txt"
# Expected: {"ok": true} then {"error": "الملف غير موجود"}

# Chunked-upload helper for testing (v1: init/chunk/complete with 4MB chunks)
# Usage: bigup $BASE FILE [UUID] [STOP_AFTER_BYTES]
bigup() {
  local BASE=$1 FILE=$2 UUID=${3:-$(openssl rand -hex 16)} STOP=${4:-}
  local NAME=$(basename "$FILE") SIZE=$(stat -c%s "$FILE")
  curl -s -H 'Content-Type: application/json' \
    -d "{\"uuid\":\"$UUID\",\"name\":\"$NAME\",\"size\":$SIZE}" $BASE/upload_init > /dev/null
  local OFF=0 SENT=0 CH=4194304
  while [ "$OFF" -lt "$SIZE" ]; do
    tail -c +$((OFF+1)) "$FILE" 2>/dev/null | head -c $CH | \
      curl -s --data-binary @- "$BASE/upload_chunk?id=$UUID&offset=$OFF" > /tmp/bigup_resp.json
    OFF=$(grep -o '"offset":[0-9]*' /tmp/bigup_resp.json | grep -o '[0-9]*' | tail -1)
    [ -z "$OFF" ] && { echo "UPLOAD FAIL"; cat /tmp/bigup_resp.json; return 1; }
    SENT=$OFF
    echo "offset: $OFF"
    if [ -n "$STOP" ] && [ "$SENT" -ge "$STOP" ]; then echo "SIMULATED-DROP at $OFF"; return 0; fi
  done
  curl -s -H 'Content-Type: application/json' -d "{\"id\":\"$UUID\"}" $BASE/upload_complete
}

# 6) Concurrent upload of the same name (conflict) — must yield two files, no loss
echo a > /tmp/t1.txt; echo b > /tmp/t2.txt
mkdir -p /tmp/r1 /tmp/r2; cp /tmp/t1.txt /tmp/r1/race.txt; cp /tmp/t2.txt /tmp/r2/race.txt
U1=$(openssl rand -hex 16); U2=$(openssl rand -hex 16)
( bigup $BASE /tmp/r1/race.txt $U1 > /dev/null 2>&1; echo " <- race1 done" ) &
( bigup $BASE /tmp/r2/race.txt $U2 > /dev/null 2>&1; echo " <- race2 done" ) &
wait
curl -s $BASE/files | grep -o "race[^\"]*"
# Expected: race.txt and race (1).txt — no visible .part files
```

---

## 1) Double-click on a clean machine (no Python/Go)
> Success metric: from double-click to network/link display < 30 seconds. Client library installs = zero.
- [ ] Copy the whole folder (icon `Beam.desktop` + program `Beam`) to a clean machine with no Python/Go
- [ ] Double-click → `Beam شغال` ("Beam running") lines + IP:Port links appear within < 30 seconds
- [ ] Open `http://127.0.0.1:2004/health` from the same machine
```bash
curl -s http://127.0.0.1:2004/health
# Expected: {"ok": true}
```
- [ ] Result: ✅ / ❌ — Measured time: ____ seconds — Notes: ____

## 2) Android + iPhone: upload and download
> Success metric: new client from joining Wi-Fi to first download < one minute.
- [ ] Android: went to `http://IP:2004` directly (no code) → uploaded an image → downloaded it → opened OK
- [ ] iPhone (Safari): same steps → upload + download OK
```bash
# From a helper laptop on the same network (simulating a mobile client):
BASE=http://192.168.1.5:2004   # change IP
echo hi > /tmp/m.txt && bigup $BASE /tmp/m.txt
curl -s "$BASE/download?file=m.txt" -o /tmp/m_dl.txt && diff /tmp/m.txt /tmp/m_dl.txt && echo SAME
```
(Paste the `bigup` function from the smoke section above first.)
- [ ] Result: ✅ / ❌ — Notes: ____

## 3) Large file round-trip + drop and resume (500MB)
> Success metric: large transfer without loading RAM + resume from stopping point + speed counter and remaining time.
> Chunk protocol v2: parallel 2MB chunks (×3) + SHA-256 fingerprint per chunk + self-check (corrupt chunk is retried alone).
- [ ] Generate a 500MB file and upload it, drop mid-upload (or use STOP_AFTER), re-upload with the same UUID → resumes → download and compare SHA256
```bash
BASE=http://127.0.0.1:2004
head -c 524282004 /dev/urandom > /tmp/big500.bin
sha256sum /tmp/big500.bin | tee /tmp/big.sha
UUID=$(openssl rand -hex 16)
bigup $BASE /tmp/big500.bin $UUID 100000000   # upload 100MB then simulated drop
bigup $BASE /tmp/big500.bin $UUID             # resume from stopping point → complete
curl -s "$BASE/download?file=big500.bin" -o /tmp/big500_dl.bin
sha256sum -c <(sed 's|/tmp/big500.bin|/tmp/big500_dl.bin|' /tmp/big.sha) && echo ROUNDTRIP_OK
# Expected: ROUNDTRIP_OK + no .part files in the listing
```
- [ ] From the browser: re-select the same file after a page refresh mid-upload → resumes (automatic find) and does not start from zero
- [ ] From another browser/device: select the same file → existing progress is adopted (resume with a new identity)
- [ ] Did the speed and remaining-time counter show during upload? Yes / No — Measured speed: ____
- [ ] Single download button per file (single fast+reliable mode): progress + speed + pause/resume + fingerprint check? Yes / No — Huge file (>800MB) falls back to direct download automatically? Yes / No
- [ ] Folder zip button: preflight error (missing/empty/over-limit folder) shows readable text and starts no download? Yes / No — Healthy folder downloads as `Name.zip` and opens? Yes / No
- [ ] Folder zip STORE (default): `GET /download_zip?dir=Docs&method=store` returns exact `Content-Length` == body length, opens as a valid zip, includes empty subdirs, entries use STORE? Yes / No — `method=deflate` is 400 zip_store_only (ZIP-only)? Yes / No — `method=bogus` is a clean 400? Yes / No
- [ ] Upload-extract staging: upload a `.zip` with `extract:true` → tree lands without overwriting same names (`Name (1)`…), source zip removed on success / kept on corrupt zip, no `.extract-*` leftovers? Yes / No
- [ ] Does the log line include duration and speed? (Log from the web page — Devices & Log tab — example literal: `file (123b, 1.2s, 3.4 م.ب/ث)`)
- [ ] Result: ✅ / ❌ — Upload time: ____ — Download time: ____

## 4) Wi-Fi drop during upload → clear message
> Expected: clear Arabic message in the UI (`انقطع الاتصال... حاول تاني` — "Connection lost... try again") + no partial file shown as complete.
- [ ] Start uploading a large file (100MB) then disconnect the client Wi-Fi mid-upload
- [ ] Confirm the browser message is clear (screenshot attached?)
- [ ] Reconnect and re-upload → succeeds
```bash
# After reconnecting — verify list integrity and no corrupt file:
BASE=http://127.0.0.1:2004
curl -s $BASE/files
# After reconnecting — verify list integrity (and the log from the web page):
# Expected: healthy JSON list + exactly one completed upload line for the re-uploaded file
```
- [ ] Result: ✅ / ❌ — Visible message text: ____

## 5) Stop/start 5 times with no machine reboot
> Expected: every time it starts on the same port and prints the same links, with no stuck `Address already in use`.
- [ ] Repeat 5 times: `Ctrl+C` then `./Beam --port 2004` (or close/open the exe)
```bash
for i in 1 2 3 4 5; do
  echo "=== cycle $i ==="
  ./Beam --port 2004 & SRV=$!
  sleep 3
  curl -s http://127.0.0.1:2004/health || echo "FAIL cycle $i"
  kill $SRV 2>/dev/null; wait $SRV 2>/dev/null
  sleep 1
done
# Expected: {"ok": true} five times, no FAIL
```
- [ ] Successful cycles: __ / 5 — Result: ✅ / ❌

## 6) 8 simultaneous devices with no hangs
> Success metric: 8 devices uploading/downloading together with no hangs.
- [ ] Connect 8 clients (phones + laptops) on the same network at the same time
- [ ] Each device: open the link directly + upload a small file + download a file + `/files`
```bash
# Simulate 8 concurrent clients from one machine:
BASE=http://127.0.0.1:2004
for i in 1 2 3 4 5 6 7 8; do
  ( echo "client$i-data" > /tmp/c$i.txt \
    && bigup $BASE /tmp/c$i.txt $(openssl rand -hex 16) > /dev/null 2>&1 \
    && curl -s $BASE/files | head -c 200; echo " <- client$i done" ) &
done; wait
# Expected: 8 "done" lines with no errors, and the server still answers /health
curl -s $BASE/health
```
- [ ] Successful devices: __ / 8 — Result: ✅ / ❌ — Notes (hangs/slowness): ____

---

## 7) Android background survival (no split-screen needed)
> Success metric: the on-phone server keeps answering /health with the app fully
> backgrounded, on any Android/OEM. Regression test for "server dies unless split-screen".
- [ ] Fresh install → open Beam → tap "▶ تشغيل السيرفر" → a system dialog asks for
>   background permission → allow → التشخيص card shows "خدمة الخلفية: تعمل ✅"
>   and "إعفاء البطارية: مُعفى ✅", and a persistent "Beam يعمل" notice is visible
- [ ] Press Home (app fully hidden, NO split-screen) → from a second device,
>   `curl http://PHONE_IP:2004/health` every 30s for 5 minutes → always `{"ok":true}`
- [ ] Lock the screen for 5 minutes → unlock → status still "يعمل الآن", no restart needed
- [ ] Start a 200MB upload from another device → minimize Beam mid-upload → completes, byte-identical
- [ ] Force-stop the app (Settings → Force stop) → reopen Beam → server auto-resumes
>   ("جارٍ الاستئناف…") with no tap, notice returns
- [ ] Deny path: on a test device deny the battery dialog → status area shows the
>   warning "اسمح بتشغيل Beam في الخلفية" and التشخيص shows "إعفاء البطارية: مقيّد ⛔"
- [ ] Result: ✅ / ❌ — Device + Android version: ____ — Notes: ____

---

## Final sign-off table
| Item | ✅/❌ | Notes |
|---|---|---|
| 1) Clean machine |  |  |
| 2) Android + iPhone |  |  |
| 3) 100MB round-trip |  |  |
| 4) Drop during upload |  |  |
| 5) Stop/start ×5 |  |  |
| 7) Android background |  |  |
| 6) 8 devices |  |  |

Tester: ____ — Date: ____ — Version (`VERSION`): ____ — OS: ____
