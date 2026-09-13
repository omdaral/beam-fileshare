#!/usr/bin/env python3
"""Apply Beam's native layer onto the generated Capacitor android/ project.

Idempotent: safe to run on every build. Fails loudly on unexpected layouts.
1. Copies BeamServer.java + Hotspot.java (on-phone Go server + LOHS hotspot).
2. Registers both plugins in MainActivity.
3. Wires mobile/beam.aar into app/build.gradle.
4. Patches AndroidManifest (foreground service + wifi permissions).
5. Generates Beam launcher icons (legacy mipmaps + adaptive).
Usage: python3 scripts/apply-android-src.py  (from mobile/)
"""
import os
import shutil
import sys
import xml.etree.ElementTree as ET

HERE = os.path.dirname(os.path.abspath(__file__))
MOBILE = os.path.dirname(HERE)
APP = os.path.dirname(MOBILE)
AND = os.path.join(MOBILE, "android")
NS = "http://schemas.android.com/apk/res/android"
ET.register_namespace("android", NS)

MAIN = os.path.join(AND, "app", "src", "main", "java", "app", "beam", "share", "MainActivity.java")
MANIFEST = os.path.join(AND, "app", "src", "main", "AndroidManifest.xml")
GRADLE = os.path.join(AND, "app", "build.gradle")


def need(cond, msg):
    if not cond:
        raise SystemExit("apply-android-src: " + msg)


MAINACTIVITY = """package app.beam.share;

import android.os.Bundle;

import com.getcapacitor.BridgeActivity;

public class MainActivity extends BridgeActivity {
    @Override
    public void onCreate(Bundle savedInstanceState) {
        registerPlugin(BeamServer.class);
        registerPlugin(Hotspot.class);
        super.onCreate(savedInstanceState);
    }
}
"""


def step_plugins():
    dst = os.path.join(AND, "app", "src", "main", "java", "app", "beam", "share")
    need(os.path.isdir(dst), "android project missing (run: npx cap add android)")
    for name in ("BeamServer.java", "Hotspot.java"):
        shutil.copyfile(os.path.join(MOBILE, "src", "plugins", name),
                        os.path.join(dst, name))
    with open(MAIN, "w", encoding="utf-8") as f:
        f.write(MAINACTIVITY)
    print("plugins installed + registered ✅")


def step_aar():
    aar = os.path.join(MOBILE, "beam.aar")
    need(os.path.isfile(aar), "mobile/beam.aar missing (gomobile bind first)")
    with open(GRADLE, encoding="utf-8") as f:
        g = f.read()
    line = '    implementation(files("../../beam.aar"))\n'
    if "beam.aar" not in g:
        anchor = "    implementation fileTree(include: ['*.jar'], dir: 'libs')\n"
        need(anchor in g, "build.gradle layout changed")
        g = g.replace(anchor, anchor + line)
        with open(GRADLE, "w", encoding="utf-8") as f:
            f.write(g)
    print("beam.aar wired ✅")


def step_manifest():
    need(os.path.isfile(MANIFEST), "AndroidManifest.xml missing")
    tree = ET.parse(MANIFEST)
    root = tree.getroot()
    app = root.find("application")
    need(app is not None, "<application> missing")

    perms = [
        "android.permission.FOREGROUND_SERVICE",
        "android.permission.FOREGROUND_SERVICE_DATA_SYNC",
        "android.permission.WAKE_LOCK",
        "android.permission.POST_NOTIFICATIONS",
        "android.permission.ACCESS_FINE_LOCATION",
        "android.permission.CHANGE_WIFI_MULTICAST_STATE",
    ]
    have = {p.get("{%s}name" % NS) for p in root.findall("uses-permission")}
    for perm in perms:
        if perm not in have:
            el = ET.Element("uses-permission")
            el.set("{%s}name" % NS, perm)
            if perm == "android.permission.ACCESS_FINE_LOCATION":
                el.set("{%s}maxSdkVersion" % NS, "32")
            root.insert(list(root).index(app), el)
    if "android.permission.NEARBY_WIFI_DEVICES" not in have:
        el = ET.Element("uses-permission")
        el.set("{%s}name" % NS, "android.permission.NEARBY_WIFI_DEVICES")
        el.set("{%s}usesPermissionFlags" % NS, "neverForLocation")
        root.insert(list(root).index(app), el)

    def has(tag, attr):
        return any(c.get("{%s}name" % NS) == attr for c in app.findall(tag))

    svc = "io.capawesome.capacitorjs.plugins.foregroundservice.AndroidForegroundService"
    rcv = "io.capawesome.capacitorjs.plugins.foregroundservice.NotificationActionBroadcastReceiver"
    if not has("service", svc):
        s = ET.SubElement(app, "service")
        s.set("{%s}name" % NS, svc)
        s.set("{%s}exported" % NS, "false")
        s.set("{%s}foregroundServiceType" % NS, "dataSync")
    if not has("receiver", rcv):
        r = ET.SubElement(app, "receiver")
        r.set("{%s}name" % NS, rcv)
        r.set("{%s}exported" % NS, "false")
    tree.write(MANIFEST, encoding="utf-8", xml_declaration=True)
    print("manifest patched (service + wifi perms) ✅")


def step_icons():
    from PIL import Image, ImageDraw
    paper = Image.open(os.path.join(APP, "icon.png")).convert("RGBA")

    def mark_layer(size, alpha=255):
        k = size / 48.0
        layer = Image.new("RGBA", (size, size), (0, 0, 0, 0))
        d = ImageDraw.Draw(layer)
        ink = (25, 23, 20, alpha)
        for (x1, y1), (x2, y2), col in (
                ((10, 36), (28, 8), (25, 23, 20, alpha)),
                ((22, 38), (36, 16), (25, 23, 20, int(alpha * 0.45)))):
            a = (k * x1, k * y1)
            b = (k * x2, k * y2)
            w = k * 5.5
            d.line([a, b], fill=col, width=max(1, int(round(w))))
            for (x, y) in (a, b):
                d.ellipse([x - w / 2, y - w / 2, x + w / 2, y + w / 2], fill=col)
        r = k * 5
        d.ellipse([k * 37 - r, k * 37 - r, k * 37 + r, k * 37 + r],
                  fill=(217, 72, 31, alpha))
        return layer

    res = os.path.join(AND, "app", "src", "main", "res")
    # Adaptive foreground (mark only, transparent; bg color below).
    fg = Image.new("RGBA", (432, 432), (0, 0, 0, 0))
    fg.alpha_composite(mark_layer(432).resize((260, 260), Image.LANCZOS),
                       (86, 86))
    fg.save(os.path.join(res, "drawable", "ic_launcher_foreground.png"))
    # Legacy mipmaps (full identity art) + per-density adaptive foregrounds.
    for dpi, size in (("mdpi", 48), ("hdpi", 72), ("xhdpi", 96),
                      ("xxhdpi", 144), ("xxxhdpi", 192)):
        d = os.path.join(res, "mipmap-" + dpi)
        os.makedirs(d, exist_ok=True)
        paper.resize((size, size), Image.LANCZOS).save(
            os.path.join(d, "ic_launcher.png"))
        paper.resize((size, size), Image.LANCZOS).save(
            os.path.join(d, "ic_launcher_round.png"))
        fg.resize((size, size), Image.LANCZOS).save(
            os.path.join(d, "ic_launcher_foreground.png"))
    with open(os.path.join(res, "values", "ic_launcher_background.xml"),
              "w", encoding="utf-8") as f:
        f.write('<?xml version="1.0" encoding="utf-8"?>\n'
                '<resources>\n'
                '    <color name="ic_launcher_background">#F7F5F0</color>\n'
                '</resources>\n')
    print("launcher icons generated ✅")


def main():
    step_plugins()
    step_aar()
    step_manifest()
    step_icons()
    return 0


if __name__ == "__main__":
    sys.exit(main())
