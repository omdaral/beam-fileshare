#!/bin/bash
# Regenerate packaging icons (16..256px) from icon.png via Pillow.
# Run once; outputs to packaging/icons/beam_<size>.png (also used by deb/rpm/flatpak).
set -u
cd "$(dirname "$0")/.." || exit 1
mkdir -p packaging/icons
python3 -c "
from PIL import Image
import os
os.makedirs('packaging/icons', exist_ok=True)
src = Image.open('icon.png').convert('RGBA')
for s in [16, 22, 24, 32, 48, 64, 128, 256]:
    src.resize((s, s), Image.LANCZOS).save('packaging/icons/beam_%d.png' % s)
    print(s, 'ok')
"
