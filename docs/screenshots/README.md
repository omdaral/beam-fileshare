# Screenshots (placeholders — real images coming later)

> No fake images are committed. This file only describes the 3 required
> screenshots. Add the real PNG files later with the exact names below.

## Required images (3)

| # | File | Content | How to capture |
|---|---|---|---|
| 1 | `01-homepage-qr.png` | Desktop homepage showing the big device login link + copy button + network status card (Wi-Fi QR + link QR) + version badge `v1.6.0` | Run `./Beam` on Linux, open the page, light theme, browser width ~1280px, crop to the top cards including both QR codes |
| 2 | `02-mobile.png` | Mobile view of the same page (portrait) with the login link + QR visible | Same server, open `http://<ip>:2004` on a phone browser (or desktop devtools mobile emulation ~390px wide), screenshot portrait |
| 3 | `03-file-tree.png` | File tree view with a few folders/files, search box, per-folder download/delete buttons, and one resumable upload session if possible | Copy 2–3 sample folders into `~/Downloads/Beam`, start an upload, screenshot the tree + sessions panel |

## Rules

- Real screenshots only — do not generate or commit placeholder/fake images.
- PNG preferred, max width 1280px, hide private IPs if needed (or use `192.168.x.x` examples).
- Keep both themes? Light theme is enough; QR codes must stay black-on-white.
- Reference from docs like: `![homepage + QR](docs/screenshots/01-homepage-qr.png)`.

## Status

- [ ] `01-homepage-qr.png`
- [ ] `02-mobile.png`
- [ ] `03-file-tree.png`
