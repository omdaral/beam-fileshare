# 03 - Identity & Design

> Impression: bold, colorful landing page — full-screen gradient hero and full-bleed sections, with calm, refined content.

## 1. Name and Identity: Beam
- **Name:** Beam — a direct beam between devices (4 universal Latin letters, pronounced as written).
- **Logo:** Embedded SVG — 3 slanted light beams + dot (device) in badge color. Favicon data-URI version.
- **Colors (light — default):**
  - Paper `#F7F5F0` + white `#FFFFFF` + ink `#191714` + warm gray `#6E675C`
  - International badge `#D9481F` (frank red-orange) + purple `#8B5CF6` + cyan `#00B8D4` (for hero and accents) + success `#1E7B44` + danger `#B3261E`
- **Colors (dark — toggled):** `#141210` + `#1E1A15` + light ink `#F2EDE3` + badge `#FF6B4A`.
- **Fonts (offline):** Cairo for Arabic (embedded 400/700/900) + system grotesk stack for Latin + monospace for links and numbers. Headings at 900.
- **Languages:** Arabic (RTL) + English (LTR) — header button for everyone (local choice saved), and the owner sets `default_lang` for new visitors from the General tab (literal label: "عام", announced in `/api/status`). One pure `STR` dictionary (no code), `data-i18n` keys.
- **Rules:** Gradients allowed in hero and accents only (remaining sections solid), linear SVG icons instead of emoji everywhere, hairline borders, one soft shadow, respect `prefers-reduced-motion`.

## 2. Landing Page `/` (everyone — owner sees a ⚙️ button)
Full-width sections (inner container 1100px, mobile-first):
1. **Hero `#join`:** live badge + huge title + entry link in mono + copy + stats (network/devices/mode) — for everyone.
2. **3-step bar:** literal steps "اتصل → افتح/امسح → ارفع ونزّل" ("Connect → Open/Scan → Upload & Download").
3. **Network `#share`:** info panel + QR panel (always black on white) + copy.
4. **Owner window `#ownerZone`** (Modal — ⚙️ button in header visible to localhost only, and APIs reject others):
   Network tab (LAN/Hotspot + SSID/password/open + start/stop + capability) + General tab (port/limit/default visitor language) + Devices & Log tab (3-second live + refresh/clear) + Device tab (stop server).
5. **Files `#filesSec`:** upload (files/folder with contents) + collapsible tree + instant local search + zip download for folders + over-cap folder queued for zip with reason + help section `#help` + footer.

## 3. Voice and Tone (AR + EN)
- Simple Arabic and direct English, verb form, same name for button and result.
- Error in solution form + bilingual server messages (`msg` per `X-Lang`, with `Accept-Language` as fallback).
- Empty states are calls to action. No technical jargon.

## 4. Binding Rules
- Every touch button at least 48px. No popups or ads.
- QR at least 200x200px with white frame, scannable by phone (acceptance gate).
- No account or email sign-up. No button that doesn't serve a user journey (02-User-Journey.md).

## 5. Assets (embedded in binary — offline)
- `goserver/web/cairo-{400,700,900}.woff2` served at `/vendor/*.woff2`.
- `goserver/web/qrcode-vendor.js` served at `/vendor/qrcode.js`.
- Logo SVG embedded in page + favicon data-URI.
