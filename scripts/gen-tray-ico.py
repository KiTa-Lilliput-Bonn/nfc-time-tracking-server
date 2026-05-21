#!/usr/bin/env python3
"""
Erzeugt cmd/server/tray.ico aus derselben Kreis-Geometrie wie web/public/app-icon.svg
(Stroke #42b883, r≈14, Strichstärke 2 im 32×32-Raster).
"""
from __future__ import annotations

import math
import struct
from pathlib import Path

# RGB wie in app-icon.svg (#42b883)
R, G, B = 0x42, 0xB8, 0x83


def ring_rgba(x: float, y: float, size: int) -> tuple[int, int, int, int]:
    """True-Scalable Kreisring; Design an SVG viewBox 32×32 angelehnt."""
    cx = (size - 1) * 0.5
    cy = (size - 1) * 0.5
    d = math.hypot(x - cx, y - cy)
    scale = size / 32.0
    r_inner = 13.0 * scale
    r_outer = 15.0 * scale
    if r_inner <= d <= r_outer:
        return R, G, B, 255
    return 0, 0, 0, 0


def dib_section(size: int) -> bytes:
    w = h = size
    biSize = 40
    biWidth = w
    biHeight = h * 2
    biPlanes = 1
    biBitCount = 32
    biCompression = 0
    biSizeImage = w * h * 4
    header = struct.pack(
        "<IIIHHIIIIII",
        biSize,
        biWidth,
        biHeight,
        biPlanes,
        biBitCount,
        biCompression,
        biSizeImage,
        0,
        0,
        0,
        0,
    )
    # BMP-Zeilen bottom-up; Pixelzentrum (x+0.5, y+0.5)
    rows = []
    for row in range(h):
        y = h - 1 - row
        line = bytearray()
        for x in range(w):
            r, g, b, a = ring_rgba(x + 0.5, y + 0.5, size)
            line.extend([b, g, r, a])
        rows.append(bytes(line))
    xor = b"".join(rows)
    stride = ((w + 31) // 32) * 4
    and_mask = b"\x00" * (stride * h)
    return header + xor + and_mask


def build_ico(sizes: tuple[int, ...]) -> bytes:
    icondir = struct.pack("<HHH", 0, 1, len(sizes))
    offset = 6 + 16 * len(sizes)
    entries: list[bytes] = []
    images: list[bytes] = []
    for sz in sizes:
        dib = dib_section(sz)
        entries.append(struct.pack("<BBBBHHII", sz, sz, 0, 0, 1, 32, len(dib), offset))
        images.append(dib)
        offset += len(dib)
    return icondir + b"".join(entries) + b"".join(images)


def main() -> None:
    root = Path(__file__).resolve().parents[1]
    out = root / "cmd" / "server" / "tray.ico"
    data = build_ico((16, 32, 48))
    out.write_bytes(data)
    print(f"wrote {out} ({len(data)} bytes)")


if __name__ == "__main__":
    main()
