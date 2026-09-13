#!/usr/bin/env python3
"""Regenerate dist/MANIFEST.json + dist/SHA256SUMS over every deliverable.

Covers binaries, portable bundles and system packages (skips the manifest
files themselves). Run at the end of publish.sh. stdlib only.
Usage: manifest.py <version>
"""
import datetime
import hashlib
import json
import os
import sys

APP = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DIST = os.path.join(APP, "dist")
SKIP = {"MANIFEST.json", "SHA256SUMS"}


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def main():
    ver = sys.argv[1]
    entries = []
    for root, _dirs, files in os.walk(DIST):
        for name in sorted(files):
            if name in SKIP:
                continue
            full = os.path.join(root, name)
            rel = os.path.relpath(full, DIST)
            entries.append({
                "file": rel,
                "bytes": os.path.getsize(full),
                "sha256": sha256(full),
            })
    entries.sort(key=lambda e: e["file"])
    updated = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    # Reproducible builds: honor SOURCE_DATE_EPOCH when set.
    try:
        epoch = int(os.environ.get("SOURCE_DATE_EPOCH", ""))
        updated = datetime.datetime.fromtimestamp(epoch, datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    except Exception:
        pass
    manifest = {
        "app": "Beam",
        "version": ver,
        "updated": updated,
        "artifacts": entries,
    }
    with open(os.path.join(DIST, "MANIFEST.json"), "w", encoding="utf-8") as f:
        json.dump(manifest, f, indent=2, ensure_ascii=False)
        f.write("\n")
    with open(os.path.join(DIST, "SHA256SUMS"), "w", encoding="utf-8") as f:
        for e in entries:
            f.write("%s  %s\n" % (e["sha256"], e["file"]))
    print("manifest: %d artifacts" % len(entries))
    return 0


if __name__ == "__main__":
    sys.exit(main())
