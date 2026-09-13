#!/bin/bash
# Build the source tarball the .spec expects: /tmp/beam-<ver>.tar.gz
# Layout inside: beam-<ver>/{goserver,packaging,VERSION,go.mod?} (go.mod lives in goserver/)
set -u
APP_DIR="$(cd "$(dirname "$0")/../.." && pwd)" || exit 1
VER="${1:-$(tr -d ' \t\r\n' < "$APP_DIR/VERSION")}"
[ -z "$VER" ] && { echo "version?"; exit 1; }
TMP="$(mktemp -d)" || exit 1
mkdir -p "$TMP/beam-$VER"
cp -r "$APP_DIR/goserver" "$APP_DIR/packaging" "$APP_DIR/VERSION" "$APP_DIR/icon.png" "$TMP/beam-$VER/"
# Drop build/runtime residue from the staging copy (never ship binaries,
# logs, or the bulky AppImage tooling — the spec rebuilds from source).
rm -f "$TMP/beam-$VER/goserver/fileshare" "$TMP/beam-$VER/goserver/fileshare.exe"
rm -rf "$TMP/beam-$VER/packaging/tools" "$TMP/beam-$VER/dist"
find "$TMP/beam-$VER" \( -name .uploads -o -name '*.log' \) -exec rm -rf {} + 2>/dev/null || true
tar -czf "/tmp/beam-$VER.tar.gz" -C "$TMP" "beam-$VER"
rm -rf "$TMP"
echo "tarball: /tmp/beam-$VER.tar.gz ($(du -h "/tmp/beam-$VER.tar.gz" | cut -f1))"
echo "next: rpmbuild -bb packaging/fedora/beam.spec --define 'ver $VER'  (SOURCES=/tmp/beam-$VER.tar.gz)"
