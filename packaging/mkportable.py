#!/usr/bin/env python3
"""Assemble portable Beam bundles (zip/tar.gz) from the dist/ tree.

Usage: mkportable.py <version> <outdir>
Reads dist/<os>/<arch>/<ver>/Beam[.exe] + root launchers/icons, writes
validated bundles + prints "<file> <bytes>" lines for the manifest.
stdlib only (no `zip` binary needed).
"""
import hashlib
import os
import sys
import tarfile
import zipfile

APP = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

README_WIN = """Beam — مشاركة الملفات بين أجهزتك (بدون إنترنت وبدون تثبيت)
1. دبل كليك على Beam.bat (السيرفر يفتح المتصفح تلقائياً على البورت 2004).
2. شارك رابط الدخول الظاهر أعلى الصفحة مع أي جهاز على نفس الشبكة.
3. ملفاتك في Downloads\\Beam داخل مجلد المستخدم.
Beam — share files between your devices (offline, no install).
Double-click Beam.bat, then share the join link with any device on the same network.
"""

README_MAC = """Beam — مشاركة الملفات بين أجهزتك (بدون إنترنت وبدون تثبيت)
1. أول مرة: كليك يمين على Beam.command ← Open (تجاوز Gatekeeper مرة واحدة).
2. بعدها دبل كليك — المتصفح يفتح تلقائياً (البورت 2004، وضع LAN فقط على ماك).
3. ملفاتك في ~/Downloads/Beam.
"""

README_LINUX = """Beam — مشاركة الملفات بين أجهزتك (بدون إنترنت وبدون تثبيت)
1. نفّذ ./install.sh مرة واحدة (يولّد أيقونة Beam بالمسارات الصحيحة).
2. دبل كليك على أيقونة Beam (المتصفح يفتح تلقائياً، البورت 2004).
3. ملفاتك في ~/Downloads/Beam.
"""


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def bundle_zip(outpath, members):
    """members: list of (arcname, realpath, is_exec)."""
    # Reproducible: fixed date (1980-01-01) unless SOURCE_DATE_EPOCH set.
    import datetime
    try:
        epoch = int(os.environ.get("SOURCE_DATE_EPOCH", ""))
        dt = datetime.datetime.fromtimestamp(epoch, datetime.timezone.utc).timetuple()[:6]
    except Exception:
        dt = (1980, 1, 1, 0, 0, 0)
    with zipfile.ZipFile(outpath, "w", zipfile.ZIP_DEFLATED) as z:
        for arc, real, exe in members:
            zi = zipfile.ZipInfo(arc, date_time=dt)
            zi.external_attr = (0o755 if exe else 0o644) << 16
            with open(real, "rb") as f:
                z.writestr(zi, f.read())
    # structural validation
    with zipfile.ZipFile(outpath) as z:
        names = set(z.namelist())
        for arc, real, exe in members:
            if arc not in names:
                raise SystemExit("bundle missing member: " + arc)
            if exe and (z.getinfo(arc).external_attr >> 16) & 0o111 == 0:
                raise SystemExit("exec bit lost: " + arc)


def bundle_tar(outpath, members):
    import time
    try:
        mtime = int(os.environ.get("SOURCE_DATE_EPOCH", "0")) or 0
    except Exception:
        mtime = 0
    with tarfile.open(outpath, "w:gz", compresslevel=9) as t:
        for arc, real, exe in members:
            ti = t.gettarinfo(real, arc)
            ti.mode = 0o755 if exe else 0o644
            ti.uid = ti.gid = 0
            ti.uname = ti.gname = ""
            if mtime:
                ti.mtime = mtime
            with open(real, "rb") as f:
                t.addfile(ti, f)
    with tarfile.open(outpath) as t:
        names = set(t.getnames())
        for arc, real, exe in members:
            if arc not in names:
                raise SystemExit("bundle missing member: " + arc)


def main():
    ver, outdir = sys.argv[1], sys.argv[2]
    os.makedirs(outdir, exist_ok=True)
    B = lambda *p: os.path.join(APP, *p)  # noqa: E731
    results = []

    def need(path):
        if not os.path.isfile(path):
            raise SystemExit("missing input (run ./build-all.sh first): " + path)
        return path

    # README staging
    readmes = {"win": README_WIN, "mac": README_MAC, "linux": README_LINUX}
    staged = {}
    for k, text in readmes.items():
        p = os.path.join(outdir, ".README-" + k + ".txt")
        with open(p, "w", encoding="utf-8") as f:
            f.write(text)
        staged[k] = p

    jobs = [
        ("windows", "amd64", "Beam-%s-windows-amd64.zip" % ver, "zip", [
            ("Beam.exe", need(B("dist", "windows", "amd64", ver, "Beam.exe")), False),
            ("Beam.bat", need(B("Beam.bat")), False),
            ("beam.ico", need(B("packaging", "icons", "beam.ico")), False),
            ("VERSION", need(B("VERSION")), False),
            ("README.txt", staged["win"], False),
        ]),
        ("windows", "arm64", "Beam-%s-windows-arm64.zip" % ver, "zip", [
            ("Beam.exe", need(B("dist", "windows", "arm64", ver, "Beam.exe")), False),
            ("Beam.bat", need(B("Beam.bat")), False),
            ("beam.ico", need(B("packaging", "icons", "beam.ico")), False),
            ("VERSION", need(B("VERSION")), False),
            ("README.txt", staged["win"], False),
        ]),
        ("darwin", "amd64", "Beam-%s-macos-amd64.zip" % ver, "zip", [
            ("Beam", need(B("dist", "darwin", "amd64", ver, "Beam")), True),
            ("Beam.command", need(B("Beam.command")), True),
            ("beam.icns", need(B("packaging", "icons", "beam.icns")), False),
            ("VERSION", need(B("VERSION")), False),
            ("README.txt", staged["mac"], False),
        ]),
        ("darwin", "arm64", "Beam-%s-macos-arm64.zip" % ver, "zip", [
            ("Beam", need(B("dist", "darwin", "arm64", ver, "Beam")), True),
            ("Beam.command", need(B("Beam.command")), True),
            ("beam.icns", need(B("packaging", "icons", "beam.icns")), False),
            ("VERSION", need(B("VERSION")), False),
            ("README.txt", staged["mac"], False),
        ]),
        ("linux", "amd64", "Beam-%s-linux-amd64.tar.gz" % ver, "tar", [
            ("Beam", need(B("dist", "linux", "amd64", ver, "Beam")), True),
            ("Beam.sh", need(B("Beam.sh")), True),
            ("install.sh", need(B("install.sh")), True),
            ("icon.png", need(B("icon.png")), False),
            ("VERSION", need(B("VERSION")), False),
            ("README.txt", staged["linux"], False),
        ]),
        ("linux", "arm64", "Beam-%s-linux-arm64.tar.gz" % ver, "tar", [
            ("Beam", need(B("dist", "linux", "arm64", ver, "Beam")), True),
            ("Beam.sh", need(B("Beam.sh")), True),
            ("install.sh", need(B("install.sh")), True),
            ("icon.png", need(B("icon.png")), False),
            ("VERSION", need(B("VERSION")), False),
            ("README.txt", staged["linux"], False),
        ]),
    ]

    for _os, _arch, fname, kind, members in jobs:
        out = os.path.join(outdir, fname)
        if kind == "zip":
            bundle_zip(out, members)
        else:
            bundle_tar(out, members)
        size = os.path.getsize(out)
        print("%s %d %s" % (fname, size, sha256(out)))
        results.append(fname)

    for p in staged.values():
        os.remove(p)
    return 0


if __name__ == "__main__":
    sys.exit(main())
