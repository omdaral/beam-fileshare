# 01 - Goals

> Decision reference: if you're torn between two options, pick the one that achieves these goals.

## 1. The Problem
- The company needs to move files between computers, phones, and PCs quickly.
- Internet is slow or forbidden, and apps like SHAREit have ads and aren't safe.
- Most solutions require installation on every device or technical expertise.

## 2. Core Goal (one sentence)
**One company computer opens a private network, and any device that joins it sends and receives files from the browser in one click, with no internet and no installation.**

## 3. Detailed Goals
1. One-click launch (double-click) with no commands or complex setup.
2. Single executable: `Beam.exe` for Windows and `Beam` for Linux, with no Python and no `pip install`.
3. Client installs nothing: browser only (Chrome / Safari / Edge).
4. Works 100% offline inside the company.
5. Two-way send and receive, multiple files, with a progress bar.
6. Supports 8-10 devices at the same time efficiently.
7. Arabic-first (RTL) and simple for a non-technical employee.

## 4. Out of Scope (we won't do this)
- ❌ Chat or calls between devices.
- ❌ Cloud sync or internet upload.
- ❌ User accounts and complex login.
- ❌ iOS native app (IPA) at this stage — Android APK exists as an optional wrapper (see `mobile/SETUP.md`), iPhone uses Safari → Add to Home Screen.
- ❌ Advanced military-grade encryption — WPA2 password is enough for now (security = Wi-Fi password, no entry codes).

## 5. Success Metrics (how we measure)
| Metric | Success |
|---------|--------|
| Time from double-click to network appearing | Under 30 seconds |
| New client from joining Wi-Fi to first download | Under a minute + QR |
| 100MB file transfer | No disconnect, with clear progress |
| Simultaneous devices | 8 devices without lag |
| Library installs on client device | Zero |
| Working without internet | Fully working |

## 6. Users
- **Manager / host device owner:** Turns the network on and off, sees who is connected, sees the file log.
- **Employee:** Joins quickly, uploads a work file, downloads a file, and leaves.

## 7. Decision Rule
Any new proposal must pass 3 questions:
1. Does it preserve "no install"? If not -> rejected.
2. Does it work offline? If not -> rejected.
3. Does it work on Windows and Linux? If not -> needs a documented alternative or is rejected.
