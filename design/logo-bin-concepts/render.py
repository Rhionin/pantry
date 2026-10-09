#!/usr/bin/env python3
"""Render the bin logo concepts in this folder.

Review artifacts only. Nothing here is wired into the app.
"""

from pathlib import Path

import cairosvg
from PIL import Image, ImageDraw, ImageFont

HERE = Path(__file__).resolve().parent
SHELF_SVG_PATH = HERE.parents[1] / "internal" / "brand" / "concepts" / "shelf.svg"

TILE = "#1C7C4E"
RIM = "#14281C"
CREAM = "#F6F1E6"
AMBER = "#F0B429"
GLACIER = "#2C6E97"
PUNCH = "#E15A2D"
INK = "#1C1915"
MUTED = "#5E584E"
RULE = "#E4DFD4"
WHITE = "#FFFFFF"

INTER_BOLD = "/usr/share/fonts/truetype/macos/Inter-Bold.ttf"
INTER_SEMI = "/usr/share/fonts/truetype/macos/Inter-SemiBold.ttf"
INTER_REG = "/usr/share/fonts/truetype/macos/Inter-Regular.ttf"


def svg(body, title, desc):
    return f"""<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512">
  <title>{title}</title>
  <desc>{desc}</desc>
  <rect width="512" height="512" rx="118" fill="{TILE}"/>
  {body}
</svg>
"""


def fill_path(x, top, w, bottom, radius):
    """Amber sitting in a rounded well. The curve matches the well's bottom."""
    straight = bottom - radius - top
    inner = w - 2 * radius
    return (
        f'<path fill="{AMBER}" d="M{x} {top}h{w}v{straight}'
        f"c0 {radius * 0.552:.1f}-{radius * 0.448:.1f} {radius}-{radius} {radius}"
        f"h-{inner}"
        f"c-{radius * 0.552:.1f} 0-{radius}-{radius * 0.448:.1f}-{radius}-{radius}z\"/>"
    )


# One tub. The flat top of the amber is the stock line.
LEVEL = svg(
    f"""<rect x="72" y="88" width="368" height="336" rx="78" fill="{RIM}"/>
  <rect x="128" y="144" width="256" height="224" rx="40" fill="{CREAM}"/>
  {fill_path(128, 268, 256, 368, 40)}""",
    "Level",
    "One pantry bin. The flat top of the amber is how full it is.",
)

# A tall measuring cup. Wider than the first pass so the fill survives at 16px.
# The brim is the silhouette; the cream neck is the shortfall.
MEASURE = svg(
    f"""<rect x="140" y="112" width="232" height="316" rx="64" fill="{RIM}"/>
  <rect x="96" y="60" width="320" height="92" rx="42" fill="{RIM}"/>
  <rect x="184" y="156" width="144" height="228" rx="36" fill="{CREAM}"/>
  {fill_path(184, 260, 144, 384, 36)}""",
    "Measure",
    "A tall bin used as a gauge. The amber stops short of the brim.",
)


def tub(x, ratio):
    y, h, rx, inset = 118, 276, 46, 16
    bw = 112
    inner_top = y + inset
    inner_bot = y + h - inset
    inner_h = inner_bot - inner_top
    fill_h = round(inner_h * ratio)
    radius = 22
    return (
        f'<rect x="{x}" y="{y}" width="{bw}" height="{h}" rx="{rx}" fill="{CREAM}"/>\n  '
        + fill_path(x + inset, inner_bot - fill_h, bw - 2 * inset, inner_bot, radius)
    )


# Three amounts, side by side. No dark rim: a rim at this size would erase the levels.
ROW = svg(
    tub(52, 0.32) + "\n  " + tub(200, 0.58) + "\n  " + tub(348, 0.84),
    "Row",
    "Three bins, three amounts. A vertical stack would redo the existing Stack mark, so these sit in a row.",
)

# The Shelf plank is copied exactly. The bin stands on it.
PLANK = svg(
    f"""<rect x="136" y="68" width="240" height="250" rx="60" fill="{RIM}"/>
  <rect x="180" y="112" width="152" height="178" rx="34" fill="{CREAM}"/>
  {fill_path(180, 196, 152, 290, 34)}
  <rect x="76" y="302" width="360" height="100" rx="50" fill="{CREAM}"/>""",
    "Plank",
    "The current Shelf plank, with a bin where the parcel sat.",
)

MEMBERS = svg(
    f"""<rect x="60" y="112" width="392" height="288" rx="76" fill="{RIM}"/>
  <rect x="108" y="160" width="296" height="192" rx="40" fill="{CREAM}"/>
  <rect x="128" y="232" width="80" height="112" rx="24" fill="{AMBER}"/>
  <rect x="216" y="232" width="80" height="112" rx="24" fill="{GLACIER}"/>
  <rect x="304" y="232" width="80" height="112" rx="24" fill="{PUNCH}"/>""",
    "Members",
    "One bin, and a swatch for each product standing in it.",
)

MONOGRAM = svg(
    f"""<rect x="84" y="64" width="124" height="384" rx="56" fill="{CREAM}"/>
  <rect x="156" y="64" width="272" height="252" rx="76" fill="{CREAM}"/>
  <rect x="236" y="116" width="140" height="140" rx="48" fill="{TILE}"/>
  {fill_path(236, 180, 140, 256, 48)}""",
    "Monogram",
    "A P whose counter is the bin.",
)

# Favicon notes are filled after looking at the 16px and 32px rasters.
CONCEPTS = [
    {
        "slug": "level",
        "name": "Level",
        "idea": "One bin. The amber stops where the stock stops.",
        "verdict": "Holds at 16 px. The rim, the empty well, and the amber stay separate.",
        "svg": LEVEL,
    },
    {
        "slug": "measure",
        "name": "Measure",
        "idea": "The bin is a gauge, filled short of the brim.",
        "verdict": "Holds at 16 px. The brim, the empty neck, and the amber are all still there.",
        "svg": MEASURE,
    },
    {
        "slug": "row",
        "name": "Row",
        "idea": "Three bins, three amounts, one household.",
        "verdict": "Holds at 32 px. At 16 px the gaps collapse, so the three heights barely separate.",
        "svg": ROW,
    },
    {
        "slug": "plank",
        "name": "Plank",
        "idea": "The Shelf plank stays. A bin takes the parcel’s place.",
        "verdict": "Holds at 32 px. At 16 px it is close to Shelf. The pale well above the amber is the difference.",
        "svg": PLANK,
    },
    {
        "slug": "members",
        "name": "Members",
        "idea": "The contents are a swatch for each product in the group.",
        "verdict": "Holds at 32 px. At 16 px each swatch is about two pixels wide.",
        "svg": MEMBERS,
    },
    {
        "slug": "monogram",
        "name": "Monogram",
        "idea": "The counter of the P is the bin.",
        "verdict": "The P holds at 16 px. The fill in the counter is clearer at 32 px.",
        "svg": MONOGRAM,
    },
]


def font(path, size):
    return ImageFont.truetype(path, size)


def render_svg(text, path, size):
    cairosvg.svg2png(
        bytestring=text.encode(),
        write_to=str(path),
        output_width=size,
        output_height=size,
    )


def text_width(draw, text, f, tracking):
    return sum(draw.textlength(ch, font=f) + tracking for ch in text) - tracking


def draw_tracked(draw, xy, text, f, fill, tracking):
    x, y = xy
    for ch in text:
        draw.text((x, y), ch, font=f, fill=fill, anchor="ls")
        x += draw.textlength(ch, font=f) + tracking
    return x


def wrap(draw, text, f, width):
    lines, cur = [], ""
    for word in text.split():
        trial = word if not cur else f"{cur} {word}"
        if draw.textlength(trial, font=f) <= width:
            cur = trial
        else:
            if cur:
                lines.append(cur)
            cur = word
    if cur:
        lines.append(cur)
    return lines


def open_rgba(path):
    return Image.open(path).convert("RGBA")


def paste(base, im, xy):
    base.paste(im, xy, im)


def lockup(mark_path, path):
    mark_px = 128
    pad_x, pad_y = 28, 28
    gap = 24
    f = font(INTER_BOLD, 96)
    tracking = -1.2
    probe = ImageDraw.Draw(Image.new("RGB", (1, 1)))
    tw = text_width(probe, "Pantry", f, tracking)
    w = pad_x + mark_px + gap + int(tw) + pad_x
    h = pad_y + mark_px + pad_y
    im = Image.new("RGB", (w, h), WHITE)
    icon = open_rgba(mark_path).resize((mark_px, mark_px), Image.Resampling.LANCZOS)
    paste(im, icon, (pad_x, pad_y))
    draw = ImageDraw.Draw(im)
    # Baseline sits so the cap height, not the descenders, centers on the mark.
    ascent, _descent = f.getmetrics()
    cap = ascent * 0.727
    baseline = pad_y + mark_px / 2 + cap / 2
    draw_tracked(draw, (pad_x + mark_px + gap, baseline), "Pantry", f, INK, tracking)
    im.save(path)
    return im


def blow(path, scale):
    im = open_rgba(path)
    return im.resize((im.width * scale, im.height * scale), Image.Resampling.NEAREST)


def framed(im):
    """Hairline around a pixel preview so the tile edge stays visible."""
    framed_im = Image.new("RGBA", (im.width + 2, im.height + 2), RULE)
    framed_im.paste(im, (1, 1), im)
    return framed_im


def render_marks():
    shelf_svg = SHELF_SVG_PATH.read_text()
    shelf = HERE / "shelf-current"
    shelf.mkdir(exist_ok=True)
    (shelf / "mark.svg").write_text(shelf_svg)
    for size, name in ((512, "icon-512.png"), (48, "mark-48.png"), (32, "mark-32.png"), (16, "mark-16.png")):
        render_svg(shelf_svg, shelf / name, size)
    lockup(shelf / "icon-512.png", shelf / "lockup.png")

    for concept in CONCEPTS:
        folder = HERE / concept["slug"]
        folder.mkdir(exist_ok=True)
        (folder / "mark.svg").write_text(concept["svg"])
        for size, name in ((512, "icon-512.png"), (48, "mark-48.png"), (32, "mark-32.png"), (16, "mark-16.png")):
            render_svg(concept["svg"], folder / name, size)
        lockup(folder / "icon-512.png", folder / "lockup.png")


def board(concept, shelf_dir):
    name_f = font(INTER_BOLD, 40)
    idea_f = font(INTER_REG, 22)
    label_f = font(INTER_SEMI, 16)
    verdict_f = font(INTER_REG, 20)
    small_f = font(INTER_REG, 15)

    margin = 48
    icon = 512
    gap = 48
    width = margin + icon + gap + icon + margin
    probe = ImageDraw.Draw(Image.new("RGB", (1, 1)))
    verdict_lines = wrap(probe, concept["verdict"], verdict_f, width - margin * 2)
    height = margin + 52 + 36 + icon + 36 + 150 + 28 + 28 * len(verdict_lines) + margin
    im = Image.new("RGB", (width, height), WHITE)
    draw = ImageDraw.Draw(im)

    y = margin
    draw.text((margin, y), concept["name"], font=name_f, fill=INK)
    y += 52
    draw.text((margin, y), concept["idea"], font=idea_f, fill=MUTED)
    y += 46
    icon_y = y
    concept_icon = open_rgba(HERE / concept["slug"] / "icon-512.png")
    shelf_icon = open_rgba(shelf_dir / "icon-512.png")
    paste(im, concept_icon, (margin, icon_y))
    paste(im, shelf_icon, (margin + icon + gap, icon_y))
    cap_y = icon_y + icon + 10
    draw.text((margin, cap_y), concept["name"], font=label_f, fill=MUTED)
    draw.text((margin + icon + gap, cap_y), "Shelf, current", font=label_f, fill=MUTED)

    y = cap_y + 36
    mark48 = open_rgba(HERE / concept["slug"] / "mark-48.png")
    b32 = framed(blow(HERE / concept["slug"] / "mark-32.png", 4))
    b16 = framed(blow(HERE / concept["slug"] / "mark-16.png", 8))
    paste(im, mark48, (margin, y + 40))
    draw.text((margin, y + 96), "48 px", font=small_f, fill=MUTED)
    paste(im, b32, (margin + 120, y))
    draw.text((margin + 120, y + 140), "32 px, enlarged", font=small_f, fill=MUTED)
    paste(im, b16, (margin + 280, y))
    draw.text((margin + 280, y + 140), "16 px, enlarged", font=small_f, fill=MUTED)

    lock = Image.open(HERE / concept["slug"] / "lockup.png").convert("RGB")
    lock.thumbnail((width - margin - 460, 150), Image.Resampling.LANCZOS)
    im.paste(lock, (margin + 460, y))
    draw.text((margin + 460, y + lock.height + 6), "Header", font=small_f, fill=MUTED)

    y = y + 168
    for line in verdict_lines:
        draw.text((margin, y), line, font=verdict_f, fill=INK)
        y += 28
    im.save(HERE / concept["slug"] / "board.png")


def comparison_sheet():
    name_f = font(INTER_SEMI, 22)
    idea_f = font(INTER_REG, 15)
    title_f = font(INTER_BOLD, 36)
    sub_f = font(INTER_REG, 18)
    head_f = font(INTER_SEMI, 14)
    foot_f = font(INTER_REG, 15)

    rows = [
        {
            "slug": "shelf-current",
            "name": "Shelf",
            "idea": "The mark in the app today. A parcel on a plank.",
            "verdict": "Holds at 16 px. The parcel and the plank stay separate.",
        },
        *[{k: c[k] for k in ("slug", "name", "idea", "verdict")} for c in CONCEPTS],
    ]

    width = 1560
    margin = 40
    name_w = 250
    mark = 168
    blow32 = 32 * 4 + 2
    blow16 = 16 * 8 + 2
    lock_w = 360
    shelf_s = 112
    row_h = 224
    header_h = 132
    footer_h = 56
    height = header_h + len(rows) * row_h + footer_h

    im = Image.new("RGB", (width, height), WHITE)
    draw = ImageDraw.Draw(im)
    draw.text((margin, 32), "Bin logo concepts", font=title_f, fill=INK)
    draw.text(
        (margin, 80),
        "Review only. These do not replace the Shelf mark, and they are not wired into the app.",
        font=sub_f,
        fill=MUTED,
    )

    # Column guides. Sentence case, because the columns are sizes, not a brand voice.
    cols = {
        "mark": margin + name_w,
        "s48": margin + name_w + mark + 28,
        "s32": margin + name_w + mark + 28 + 72,
        "s16": margin + name_w + mark + 28 + 72 + blow32 + 18,
        "lock": margin + name_w + mark + 28 + 72 + blow32 + 18 + blow16 + 24,
        "shelf": width - margin - shelf_s,
    }
    label_y = header_h - 22
    draw.text((cols["mark"], label_y), "Mark", font=head_f, fill=MUTED)
    draw.text((cols["s48"], label_y), "48 px", font=head_f, fill=MUTED)
    draw.text((cols["s32"], label_y), "32 px", font=head_f, fill=MUTED)
    draw.text((cols["s16"], label_y), "16 px", font=head_f, fill=MUTED)
    draw.text((cols["lock"], label_y), "Header", font=head_f, fill=MUTED)
    draw.text((cols["shelf"], label_y), "Shelf", font=head_f, fill=MUTED)
    draw.line((margin, header_h - 1, width - margin, header_h - 1), fill=RULE)

    for i, row in enumerate(rows):
        top = header_h + i * row_h
        draw.line((margin, top + row_h - 1, width - margin, top + row_h - 1), fill=RULE)
        folder = HERE / row["slug"]
        cy = top + row_h / 2
        draw.text((margin, top + 48), row["name"], font=name_f, fill=INK)
        idea_lines = wrap(draw, row["idea"], idea_f, name_w - 16)
        verdict_lines = wrap(draw, row["verdict"], idea_f, name_w - 16)
        ty = top + 80
        for line in idea_lines:
            draw.text((margin, ty), line, font=idea_f, fill=MUTED)
            ty += 20
        ty += 6
        for line in verdict_lines:
            draw.text((margin, ty), line, font=idea_f, fill=INK)
            ty += 20

        icon = open_rgba(folder / "icon-512.png").resize((mark, mark), Image.Resampling.LANCZOS)
        paste(im, icon, (cols["mark"], int(cy - mark / 2)))

        m48 = open_rgba(folder / "mark-48.png")
        paste(im, m48, (cols["s48"], int(cy - 24)))

        b32 = framed(blow(folder / "mark-32.png", 4))
        paste(im, b32, (cols["s32"], int(cy - b32.height / 2)))
        b16 = framed(blow(folder / "mark-16.png", 8))
        paste(im, b16, (cols["s16"], int(cy - b16.height / 2)))

        lock = Image.open(folder / "lockup.png").convert("RGBA")
        scale = min(lock_w / lock.width, 120 / lock.height)
        lock = lock.resize((int(lock.width * scale), int(lock.height * scale)), Image.Resampling.LANCZOS)
        paste(im, lock, (cols["lock"], int(cy - lock.height / 2)))

        if row["slug"] != "shelf-current":
            shelf = open_rgba(HERE / "shelf-current" / "icon-512.png").resize(
                (shelf_s, shelf_s), Image.Resampling.LANCZOS
            )
            paste(im, shelf, (cols["shelf"], int(cy - shelf_s / 2)))

    draw.text(
        (margin, height - 36),
        "Enlarged 32 px and 16 px cells are nearest-neighbor, so the squares are the real favicon pixels.",
        font=foot_f,
        fill=MUTED,
    )
    im.save(HERE / "comparison-sheet.png")


def main():
    render_marks()
    shelf = HERE / "shelf-current"
    for concept in CONCEPTS:
        board(concept, shelf)
    comparison_sheet()
    print("rendered", HERE)


if __name__ == "__main__":
    main()
