#!/bin/sh
# Rebuild screenshots with the system Chrome. Crops the empty tail off each shot.
set -e
root=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
out="$root/screenshots"
mkdir -p "$out"
shot() {
  w=$1
  h=$2
  name=$3
  s=$4
  dir=/tmp/chrome-$s-$name
  rm -rf "$dir"
  timeout 12 google-chrome --headless=new --no-sandbox --disable-gpu --hide-scrollbars \
    --force-device-scale-factor=2 --window-size="$w,$h" \
    --default-background-color=FFFFFFFF \
    --user-data-dir="$dir" \
    --screenshot="$out/$s-$name.png" \
    "file://$root/index.html?s=$s" >/dev/null 2>&1 || true
}
for s in a-item a-shop a-rules a-review b-item b-shop b-rules b-review; do
  shot 390 1100 390 "$s" &
done
wait
for s in a-item a-shop a-rules a-review b-item b-shop b-rules b-review; do
  shot 1440 1100 1440 "$s" &
done
wait
python3 - "$out" << 'PY'
import sys
from pathlib import Path
from PIL import Image
out = Path(sys.argv[1])
for path in sorted(out.glob("*.png")):
    im = Image.open(path).convert("RGB")
    px = im.load()
    last = 0
    for y in range(im.height - 1, -1, -1):
        hit = False
        for x in range(0, im.width, 3):
            r, g, b = px[x, y]
            if r < 250 or g < 250 or b < 250:
                hit = True
                break
        if hit:
            last = y
            break
    bottom = min(im.height, last + 72)
    cropped = im.crop((0, 0, im.width, bottom))
    cropped.save(path)
    print(f"{path.name}: {cropped.size[0]}x{cropped.size[1]}")
PY
