#!/bin/bash
# Beam .rpm builder (Fedora/openSUSE/COPR — also works on Debian with the
# `rpm` package installed: sudo apt install rpm).
# Usage: packaging/fedora/build-rpm.sh   (output: dist/rpm/*.rpm)
# Needs: rpmbuild in PATH + Go 1.21+ (spec rebuilds from source: vet+test+build).
set -u
export PATH="$HOME/go/bin:/usr/local/go/bin:/opt/go/bin:$PATH"
APP_DIR="$(cd "$(dirname "$0")/../.." && pwd)" || exit 1
cd "$APP_DIR" || exit 1
command -v rpmbuild >/dev/null 2>&1 || { echo "rpmbuild missing (Fedora: sudo dnf install rpm-build golang / Debian: sudo apt install rpm)"; exit 1; }
command -v go >/dev/null 2>&1 || { echo "go missing (needed: the spec rebuilds from source)"; exit 1; }
VER="$(tr -d ' \t\r\n' < VERSION 2>/dev/null)"
[ -z "$VER" ] && { echo "VERSION missing"; exit 1; }

./packaging/fedora/make-tarball.sh "$VER" || exit 1
TOP="$(mktemp -d)/rpm" || exit 1
mkdir -p "$TOP"/{BUILD,RPMS,SOURCES,SPECS,SRPMS}
cp "/tmp/beam-$VER.tar.gz" "$TOP/SOURCES/" || exit 1
cp packaging/fedora/beam.spec "$TOP/SPECS/" || exit 1

# NOTE: --nodeps skips rpm-db dependency checks (we build on Debian for
# Fedora; Go comes from a tarball, not an rpm). The spec still documents
# the real Fedora requirements for COPR/maintainer builds.
if rpmbuild --nodeps --define "_topdir $TOP" --define "ver $VER" -bb "$TOP/SPECS/beam.spec"; then
  mkdir -p dist/rpm
  rm -f dist/rpm/beam-fileshare_"$VER"-*.rpm
  find "$TOP/RPMS" -name '*.rpm' -exec cp {} dist/rpm/ \; || exit 1
  rm -rf "$(dirname "$TOP")"
  echo "--- contents ---"
  rpm -qlp dist/rpm/*.rpm
  echo "Built: $(ls dist/rpm/*.rpm) ($(du -h dist/rpm/*.rpm | cut -f1))"
else
  echo "rpmbuild failed"
  exit 1
fi
