#!/usr/bin/env python3
"""Regenerate every Beam icon from the identity geometry (icon.svg).

Outputs (Pillow only, no external rasterizer):
  icon.png / icon64.png ................. desktop launchers (root)
  packaging/icons/beam.ico .............. Windows bundles
  packaging/icons/beam.icns ............. macOS bundles
  packaging/icons/beam_<16..256>.png .... deb/rpm/flatpak/appimage
Fidelity notes: endpoint discs replicate SVG round caps (PIL lines are
butt-capped), and everything renders at 4x then downscales (anti-aliasing).
Usage: packaging/render-icons.py   (run from the repo root)
"""
import os
import subprocess
import sys

from PIL import Image, ImageDraw

APP = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PAPER = (247, 245, 240, 255)
INK = (25, 23, 20, 255)
INK_SOFT = (25, 23, 20, 115)  # SVG opacity .45
ACC = (217, 72, 31, 255)
SS = 4


def beam_layer(size, p1, p2, color):
    k = size / 48.0
    layer = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    d = ImageDraw.Draw(layer)
    w = k * 5.5
    a = (k * p1[0], k * p1[1])
    b = (k * p2[0], k * p2[1])
    d.line([a, b], fill=color, width=max(1, int(round(w))))
    for (x, y) in (a, b):  # round caps
        d.ellipse([x - w / 2, y - w / 2, x + w / 2, y + w / 2], fill=color)
    return layer


def render(size):
    big = size * SS
    k = big / 48.0
    im = Image.new("RGBA", (big, big), (0, 0, 0, 0))
    d = ImageDraw.Draw(im)
    d.rounded_rectangle([k * 1, k * 1, k * 47, k * 47], radius=k * 11, fill=PAPER)
    im = Image.alpha_composite(im, beam_layer(big, (10, 36), (28, 8), INK))
    im = Image.alpha_composite(im, beam_layer(big, (22, 38), (36, 16), INK_SOFT))
    d = ImageDraw.Draw(im)
    r = k * 5
    d.ellipse([k * 37 - r, k * 37 - r, k * 37 + r, k * 37 + r], fill=ACC)
    return im.resize((size, size), Image.LANCZOS)


def main():
    os.chdir(APP)
    render(256).save("icon.png")
    render(64).save("icon64.png")
    master = render(1024)
    master.save("packaging/icons/beam.ico",
                sizes=[(16, 16), (24, 24), (32, 32), (48, 48),
                       (64, 64), (128, 128), (256, 256)])
    master.save("packaging/icons/beam.icns")
    r = subprocess.run(["./packaging/icons.sh"], capture_output=True, text=True)
    if r.returncode != 0:
        print(r.stdout + r.stderr)
        return 1
    print("icons regenerated: root + ico/icns + beam_16..256 ✅")
    return 0


if __name__ == "__main__":
    sys.exit(main())
