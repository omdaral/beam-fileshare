# Beam — Packaging and Distribution (packaging/)

> One static binary per platform (`./build-all.sh`), wrapped here per OS.
> Beam writes no sidecar files (no config and no logs): settings are for the session only
> (+ a copy in browser localStorage), log in memory, and files in
> `~/Downloads/Beam` — any package can be reinstalled without touching user files.

## Map

| Target | Files here | Build | Status |
|---|---|---|---|
| Debian/Ubuntu `.deb` | `debian/build-deb.sh` + `common/` + `icons/` | `./packaging/debian/build-deb.sh amd64` | ✅ builds and is checked (lintian) here |
| AppImage | `appimage/build-appimage.sh` | needs `mksquashfs` (Debian: `sudo apt install squashfs-tools`) | ✅ x86_64 tested by running, aarch64 built only |
| Fedora `.rpm` | `fedora/build-rpm.sh` (spec + clean tarball) | `./packaging/fedora/build-rpm.sh` (needs `rpmbuild`: Fedora `rpm-build` / Debian `rpm`) → `dist/rpm/` | ✅ builds here and is checked (`rpm -qlp`) |
| Arch | `arch/PKGBUILD` + `arch/.SRCINFO` | on Arch: `updpkgsums && makepkg -si` | files ready, build on Arch |
| Flatpak/Flathub | `flatpak/com.beam.beam.{yaml,desktop,metainfo.xml,beam-wrapper.sh}` | needs `flatpak-builder` (not installed here) | manifest + metainfo validated |
| macOS | `Beam.command` (root) + darwin binary from `dist/` | zip ready | documented in README (LAN mode) |

## Full build
```bash
./build-all.sh                  # six binaries + versions.json
./packaging/build-portables.sh  # six portable ZIPs in dist/portables/<ver>/
./packaging/build-packages.sh   # deb (amd64/arm64) + appimage (if mksquashfs exists)
python3 packaging/manifest.py $(cat VERSION)  # dist/MANIFEST.json + SHA256SUMS index
./publish.sh                    # everything: 9 steps (capabilities + icons + build + packages + index + launchers + install + verify + report)
```

## Honest technical notes
1. **FUSE**: running AppImage needs FUSE2 on the user machine (Ubuntu 22.04+ has no
    FUSE2 by default) — documented alternative for the user: `sudo apt install libfuse2`
    or extract the package manually. The build itself needs no FUSE (runtime + mksquashfs).
2. **Identity before any public release (TODO)**: temporary `beam-fileshare@localhost` in
    (deb control/changelog) and `TODO` in (spec/PKGBUILD/metainfo/app-id) —
    put your name, email, project site, and LICENSE file before: Debian mentors / COPR /
    AUR / Flathub. Note: **Flathub app-id cannot change after first acceptance**.
3. **Arch**: update `source=` to the real tarball URL + `updpkgsums` then
    `makepkg --printsrcinfo > .SRCINFO` before uploading to AUR.
4. **Flathub checklist**: set identity + LICENSE + switch manifest source to
    git tag + add screenshots with links + build with flatpak-builder + try running
    (`flatpak-builder --run ... beam`) + read `flatpak-builder-lint`.
5. **Fedora/COPR**: build the tarball here (`make-tarball.sh`), copy it with the spec
    to a Fedora machine, `rpmbuild -bb`, then upload the SRPM to COPR.
6. Empty folders are lost inside folder zips (documented limitation) — unrelated to packages.
7. `packaging/tools/` (appimagetool/runtime) are local build tools not committed
    logically — add them to `.gitignore` if you start git (folder is not git currently).

## Output folders (dist/)
- `dist/debian/*.deb`, `dist/appimage/*.AppImage` — from `build-packages.sh`.
- `dist/<os>/<arch>/<ver>/` + `versions.json` + `SHA256SUMS` — from `build-all.sh`.
