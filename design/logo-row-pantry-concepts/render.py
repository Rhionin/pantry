#!/usr/bin/env python3
"""Render Row riffs that read as pantry containers.

Review artifacts only. Nothing here is wired into the app.
The amber is contents, clipped to the glass. The lid, shoulder, or
flare is part of the outline, so the mark is still a container when
the fill is ignored.
"""

from pathlib import Path

import cairosvg
from PIL import Image, ImageDraw, ImageFont

HERE = Path(__file__).resolve().parent
SHELF_SVG_PATH = HERE.parents[1] / "internal" / "brand" / "concepts" / "shelf.svg"

TILE = "#1C7C4E"
CREAM = "#F6F1E6"
AMBER = "#F0B429"
INK = "#1C1915"
MUTED = "#5E584E"
RULE = "#E4DFD4"
WHITE = "#FFFFFF"

INTER_BOLD = "/usr/share/fonts/truetype/macos/Inter-Bold.ttf"
INTER_SEMI = "/usr/share/fonts/truetype/macos/Inter-SemiBold.ttf"
INTER_REG = "/usr/share/fonts/truetype/macos/Inter-Regular.ttf"

# Shared seat. A thick plank is furniture; a hairline under the jars would be an axis.
BASE = 424
PLANK = f'<rect x="40" y="408" width="432" height="68" rx="34" fill="{CREAM}"/>'


def svg(body, title, desc):
    return f"""<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512">
  <title>{title}</title>
  <desc>{desc}</desc>
  <rect width="512" height="512" rx="118" fill="{TILE}"/>
  {body}
</svg>
"""


def glass_fill(uid, outline, glass, fill_top, x, width, base):
    """Cream vessel, amber contents. The lid stays out of the clip."""
    height = base - fill_top + 12
    return f"""<defs><clipPath id="{uid}"><path d="{glass}"/></clipPath></defs>
  <path d="{outline}" fill="{CREAM}"/>
  <rect x="{x:.1f}" y="{fill_top:.1f}" width="{width:.1f}" height="{height:.1f}" fill="{AMBER}" clip-path="url(#{uid})"/>"""


def level(top, bottom, ratio):
    ratio = min(0.92, max(0.18, ratio))
    return bottom - (bottom - top) * ratio


def canister(uid, cx, body_hw, body_h, neck_hw, neck_h, lid_hw, lid_h, ratio, base=BASE):
    """Cookie-tin canister. The lid is wider than the body, and the neck is the pinch."""
    top = base - (lid_h + neck_h + body_h)
    lid_bot = top + lid_h
    neck_bot = lid_bot + neck_h
    br = min(body_hw * 0.55, 34)
    lr = min(16, lid_h * 0.4)
    outline = (
        f"M{cx - lid_hw + lr:.1f} {top:.1f}"
        f"H{cx + lid_hw - lr:.1f}"
        f"Q{cx + lid_hw:.1f} {top:.1f} {cx + lid_hw:.1f} {top + lr:.1f}"
        f"V{lid_bot:.1f}H{cx + neck_hw:.1f}V{neck_bot:.1f}H{cx + body_hw:.1f}"
        f"V{base - br:.1f}"
        f"Q{cx + body_hw:.1f} {base:.1f} {cx + body_hw - br:.1f} {base:.1f}"
        f"H{cx - body_hw + br:.1f}"
        f"Q{cx - body_hw:.1f} {base:.1f} {cx - body_hw:.1f} {base - br:.1f}"
        f"V{neck_bot:.1f}H{cx - neck_hw:.1f}V{lid_bot:.1f}H{cx - lid_hw:.1f}"
        f"V{top + lr:.1f}"
        f"Q{cx - lid_hw:.1f} {top:.1f} {cx - lid_hw + lr:.1f} {top:.1f}Z"
    )
    glass = (
        f"M{cx - body_hw:.1f} {neck_bot:.1f}H{cx + body_hw:.1f}"
        f"V{base - br:.1f}"
        f"Q{cx + body_hw:.1f} {base:.1f} {cx + body_hw - br:.1f} {base:.1f}"
        f"H{cx - body_hw + br:.1f}"
        f"Q{cx - body_hw:.1f} {base:.1f} {cx - body_hw:.1f} {base - br:.1f}Z"
    )
    return glass_fill(uid, outline, glass, level(neck_bot, base, ratio), cx - body_hw - 4, body_hw * 2 + 8, base)


def preserve(uid, cx, body_hw, body_h, shoulder_h, neck_hw, neck_h, lid_hw, lid_h, ratio, base=BASE):
    """Preserve jar. The shoulder stays wide, then tucks into a neck under a cap."""
    top = base - (body_h + shoulder_h + neck_h + lid_h)
    lid_bot = top + lid_h
    mouth = lid_bot + neck_h
    body_top = mouth + shoulder_h
    br = min(body_hw * 0.48, 36)
    lr = min(14, lid_h * 0.38)
    c1 = mouth + shoulder_h * 0.62
    c2 = mouth + shoulder_h * 0.22
    outline = (
        f"M{cx - lid_hw + lr:.1f} {top:.1f}"
        f"H{cx + lid_hw - lr:.1f}"
        f"Q{cx + lid_hw:.1f} {top:.1f} {cx + lid_hw:.1f} {top + lr:.1f}"
        f"V{lid_bot:.1f}H{cx + neck_hw:.1f}V{mouth:.1f}"
        f"C{cx + neck_hw:.1f} {c1:.1f} {cx + body_hw:.1f} {c2:.1f} {cx + body_hw:.1f} {body_top:.1f}"
        f"V{base - br:.1f}"
        f"Q{cx + body_hw:.1f} {base:.1f} {cx + body_hw - br:.1f} {base:.1f}"
        f"H{cx - body_hw + br:.1f}"
        f"Q{cx - body_hw:.1f} {base:.1f} {cx - body_hw:.1f} {base - br:.1f}"
        f"V{body_top:.1f}"
        f"C{cx - body_hw:.1f} {c2:.1f} {cx - neck_hw:.1f} {c1:.1f} {cx - neck_hw:.1f} {mouth:.1f}"
        f"V{lid_bot:.1f}H{cx - lid_hw:.1f}"
        f"V{top + lr:.1f}"
        f"Q{cx - lid_hw:.1f} {top:.1f} {cx - lid_hw + lr:.1f} {top:.1f}Z"
    )
    glass = (
        f"M{cx - neck_hw:.1f} {lid_bot:.1f}H{cx + neck_hw:.1f}V{mouth:.1f}"
        f"C{cx + neck_hw:.1f} {c1:.1f} {cx + body_hw:.1f} {c2:.1f} {cx + body_hw:.1f} {body_top:.1f}"
        f"V{base - br:.1f}"
        f"Q{cx + body_hw:.1f} {base:.1f} {cx + body_hw - br:.1f} {base:.1f}"
        f"H{cx - body_hw + br:.1f}"
        f"Q{cx - body_hw:.1f} {base:.1f} {cx - body_hw:.1f} {base - br:.1f}"
        f"V{body_top:.1f}"
        f"C{cx - body_hw:.1f} {c2:.1f} {cx - neck_hw:.1f} {c1:.1f} {cx - neck_hw:.1f} {mouth:.1f}Z"
    )
    return glass_fill(uid, outline, glass, level(lid_bot, base, ratio), cx - body_hw - 4, body_hw * 2 + 8, base)


def jam(uid, cx, body_hw, body_h, lid_hw, lid_h, ratio, base=BASE):
    """Short jam jar. Cap wider than the mouth, belly rounding out under it."""
    top = base - body_h - lid_h
    lid_bot = top + lid_h
    mouth_hw = body_hw * 0.62
    shoulder = min(48, body_h * 0.34)
    br = min(body_hw * 0.5, 42)
    lr = min(14, lid_h * 0.36)
    outline = (
        f"M{cx - lid_hw + lr:.1f} {top:.1f}"
        f"H{cx + lid_hw - lr:.1f}"
        f"Q{cx + lid_hw:.1f} {top:.1f} {cx + lid_hw:.1f} {top + lr:.1f}"
        f"V{lid_bot:.1f}H{cx + mouth_hw:.1f}"
        f"C{cx + mouth_hw:.1f} {lid_bot + shoulder * 0.7:.1f} {cx + body_hw:.1f} {lid_bot + shoulder * 0.35:.1f} {cx + body_hw:.1f} {lid_bot + shoulder:.1f}"
        f"V{base - br:.1f}"
        f"Q{cx + body_hw:.1f} {base:.1f} {cx + body_hw - br:.1f} {base:.1f}"
        f"H{cx - body_hw + br:.1f}"
        f"Q{cx - body_hw:.1f} {base:.1f} {cx - body_hw:.1f} {base - br:.1f}"
        f"V{lid_bot + shoulder:.1f}"
        f"C{cx - body_hw:.1f} {lid_bot + shoulder * 0.35:.1f} {cx - mouth_hw:.1f} {lid_bot + shoulder * 0.7:.1f} {cx - mouth_hw:.1f} {lid_bot:.1f}"
        f"H{cx - lid_hw:.1f}V{top + lr:.1f}"
        f"Q{cx - lid_hw:.1f} {top:.1f} {cx - lid_hw + lr:.1f} {top:.1f}Z"
    )
    glass = (
        f"M{cx - mouth_hw:.1f} {lid_bot:.1f}H{cx + mouth_hw:.1f}"
        f"C{cx + mouth_hw:.1f} {lid_bot + shoulder * 0.7:.1f} {cx + body_hw:.1f} {lid_bot + shoulder * 0.35:.1f} {cx + body_hw:.1f} {lid_bot + shoulder:.1f}"
        f"V{base - br:.1f}"
        f"Q{cx + body_hw:.1f} {base:.1f} {cx + body_hw - br:.1f} {base:.1f}"
        f"H{cx - body_hw + br:.1f}"
        f"Q{cx - body_hw:.1f} {base:.1f} {cx - body_hw:.1f} {base - br:.1f}"
        f"V{lid_bot + shoulder:.1f}"
        f"C{cx - body_hw:.1f} {lid_bot + shoulder * 0.35:.1f} {cx - mouth_hw:.1f} {lid_bot + shoulder * 0.7:.1f} {cx - mouth_hw:.1f} {lid_bot:.1f}Z"
    )
    return glass_fill(uid, outline, glass, level(lid_bot, base, ratio), cx - body_hw - 4, body_hw * 2 + 8, base)


def bottle(uid, cx, cap_hw, cap_h, neck_hw, neck_h, belly_hw, belly_h, foot_hw, ratio, base=BASE):
    """Oil bottle. Narrow at the cap and the foot, wide only in the belly."""
    top = base - (cap_h + neck_h + belly_h)
    cap_bot = top + cap_h
    neck_bot = cap_bot + neck_h
    mid = neck_bot + belly_h * 0.46
    lr = min(12, cap_h * 0.4)
    outline = (
        f"M{cx - cap_hw + lr:.1f} {top:.1f}"
        f"H{cx + cap_hw - lr:.1f}"
        f"Q{cx + cap_hw:.1f} {top:.1f} {cx + cap_hw:.1f} {top + lr:.1f}"
        f"V{cap_bot:.1f}H{cx + neck_hw:.1f}V{neck_bot:.1f}"
        f"C{cx + neck_hw:.1f} {neck_bot + belly_h * 0.16:.1f} {cx + belly_hw:.1f} {mid - belly_h * 0.16:.1f} {cx + belly_hw:.1f} {mid:.1f}"
        f"C{cx + belly_hw:.1f} {mid + belly_h * 0.30:.1f} {cx + foot_hw + 8:.1f} {base - 28:.1f} {cx + foot_hw:.1f} {base:.1f}"
        f"H{cx - foot_hw:.1f}"
        f"C{cx - foot_hw - 8:.1f} {base - 28:.1f} {cx - belly_hw:.1f} {mid + belly_h * 0.30:.1f} {cx - belly_hw:.1f} {mid:.1f}"
        f"C{cx - belly_hw:.1f} {mid - belly_h * 0.16:.1f} {cx - neck_hw:.1f} {neck_bot + belly_h * 0.16:.1f} {cx - neck_hw:.1f} {neck_bot:.1f}"
        f"V{cap_bot:.1f}H{cx - cap_hw:.1f}V{top + lr:.1f}"
        f"Q{cx - cap_hw:.1f} {top:.1f} {cx - cap_hw + lr:.1f} {top:.1f}Z"
    )
    # Contents stay in the belly. An empty neck is what makes it a bottle.
    glass = (
        f"M{cx - neck_hw:.1f} {neck_bot - 2:.1f}H{cx + neck_hw:.1f}"
        f"C{cx + neck_hw:.1f} {neck_bot + belly_h * 0.16:.1f} {cx + belly_hw:.1f} {mid - belly_h * 0.16:.1f} {cx + belly_hw:.1f} {mid:.1f}"
        f"C{cx + belly_hw:.1f} {mid + belly_h * 0.30:.1f} {cx + foot_hw + 8:.1f} {base - 28:.1f} {cx + foot_hw:.1f} {base:.1f}"
        f"H{cx - foot_hw:.1f}"
        f"C{cx - foot_hw - 8:.1f} {base - 28:.1f} {cx - belly_hw:.1f} {mid + belly_h * 0.30:.1f} {cx - belly_hw:.1f} {mid:.1f}"
        f"C{cx - belly_hw:.1f} {mid - belly_h * 0.16:.1f} {cx - neck_hw:.1f} {neck_bot + belly_h * 0.16:.1f} {cx - neck_hw:.1f} {neck_bot - 2:.1f}Z"
    )
    return glass_fill(uid, outline, glass, level(neck_bot, base, ratio), cx - belly_hw - 4, belly_hw * 2 + 8, base)


def tin(uid, cx, body_hw, body_h, lid_hw, lid_h, ratio, base=BASE):
    """Square tin. Sharp corners, so it is not another rounded tube."""
    top = base - body_h - lid_h
    lid_bot = top + lid_h
    lr = 8
    br = 12

    def band(hw, y0, y1, rad_top, rad_bot):
        return (
            f"M{cx - hw + rad_top:.1f} {y0:.1f}H{cx + hw - rad_top:.1f}"
            f"Q{cx + hw:.1f} {y0:.1f} {cx + hw:.1f} {y0 + rad_top:.1f}"
            f"V{y1 - rad_bot:.1f}"
            f"Q{cx + hw:.1f} {y1:.1f} {cx + hw - rad_bot:.1f} {y1:.1f}"
            f"H{cx - hw + rad_bot:.1f}"
            f"Q{cx - hw:.1f} {y1:.1f} {cx - hw:.1f} {y1 - rad_bot:.1f}"
            f"V{y0 + rad_top:.1f}"
            f"Q{cx - hw:.1f} {y0:.1f} {cx - hw + rad_top:.1f} {y0:.1f}Z"
        )

    outline = (
        f"M{cx - lid_hw + lr:.1f} {top:.1f}H{cx + lid_hw - lr:.1f}"
        f"Q{cx + lid_hw:.1f} {top:.1f} {cx + lid_hw:.1f} {top + lr:.1f}"
        f"V{lid_bot:.1f}H{cx + body_hw:.1f}"
        f"V{base - br:.1f}"
        f"Q{cx + body_hw:.1f} {base:.1f} {cx + body_hw - br:.1f} {base:.1f}"
        f"H{cx - body_hw + br:.1f}"
        f"Q{cx - body_hw:.1f} {base:.1f} {cx - body_hw:.1f} {base - br:.1f}"
        f"V{lid_bot:.1f}H{cx - lid_hw:.1f}"
        f"V{top + lr:.1f}"
        f"Q{cx - lid_hw:.1f} {top:.1f} {cx - lid_hw + lr:.1f} {top:.1f}Z"
    )
    glass = band(body_hw, lid_bot, base, 2, br)
    return glass_fill(uid, outline, glass, level(lid_bot, base, ratio), cx - body_hw - 4, body_hw * 2 + 8, base)


def crock(uid, cx, rim_hw, mouth_hw, foot_hw, body_h, lip_h, ratio, base=BASE):
    """Crock. The rim steps out past the wall, and the wall narrows toward the foot."""
    top = base - body_h - lip_h
    lip_bot = top + lip_h
    # Keep a full pixel of lip outside the wall, and a mouth inside that wall.
    wall_hw = rim_hw - 32
    mouth_hw = min(mouth_hw, wall_hw - 16)
    r = min(16, lip_h * 0.4)
    # The rim is the widest point. The wall steps in, then tapers to the foot.
    outline = (
        f"M{cx - rim_hw + r:.1f} {top:.1f}H{cx + rim_hw - r:.1f}"
        f"Q{cx + rim_hw:.1f} {top:.1f} {cx + rim_hw:.1f} {top + r:.1f}"
        f"V{lip_bot:.1f}H{cx + wall_hw:.1f}"
        f"L{cx + foot_hw:.1f} {base - 18:.1f}"
        f"Q{cx + foot_hw:.1f} {base:.1f} {cx + foot_hw * 0.35:.1f} {base:.1f}"
        f"H{cx - foot_hw * 0.35:.1f}"
        f"Q{cx - foot_hw:.1f} {base:.1f} {cx - foot_hw:.1f} {base - 18:.1f}"
        f"L{cx - wall_hw:.1f} {lip_bot:.1f}H{cx - rim_hw:.1f}"
        f"V{top + r:.1f}"
        f"Q{cx - rim_hw:.1f} {top:.1f} {cx - rim_hw + r:.1f} {top:.1f}Z"
    )
    glass = (
        f"M{cx - wall_hw:.1f} {lip_bot:.1f}H{cx + wall_hw:.1f}"
        f"L{cx + foot_hw:.1f} {base - 18:.1f}"
        f"Q{cx + foot_hw:.1f} {base:.1f} {cx + foot_hw * 0.35:.1f} {base:.1f}"
        f"H{cx - foot_hw * 0.35:.1f}"
        f"Q{cx - foot_hw:.1f} {base:.1f} {cx - foot_hw:.1f} {base - 18:.1f}Z"
    )
    mouth_h = max(20, lip_h * 0.5)
    mouth_y = top + (lip_h - mouth_h) * 0.48
    mouth = (
        f'<rect x="{cx - mouth_hw:.1f}" y="{mouth_y:.1f}" width="{mouth_hw * 2:.1f}" height="{mouth_h:.1f}" '
        f'rx="{mouth_h / 2:.1f}" fill="{TILE}"/>'
    )
    return glass_fill(uid, outline, glass, level(lip_bot, base, ratio), cx - rim_hw - 4, rim_hw * 2 + 8, base) + "\n  " + mouth


# The original Row, copied from the #145 concept so this round can sit beside it.
ROW_SVG = svg(
    f"""<rect x="52" y="118" width="112" height="276" rx="46" fill="{CREAM}"/>
  <path fill="{AMBER}" d="M68 300h80v56c0 12.1-9.9 22-22 22h-36c-12.1 0-22-9.9-22-22z"/>
  <rect x="200" y="118" width="112" height="276" rx="46" fill="{CREAM}"/>
  <path fill="{AMBER}" d="M216 236h80v120c0 12.1-9.9 22-22 22h-36c-12.1 0-22-9.9-22-22z"/>
  <rect x="348" y="118" width="112" height="276" rx="46" fill="{CREAM}"/>
  <path fill="{AMBER}" d="M364 173h80v183c0 12.1-9.9 22-22 22h-36c-12.1 0-22-9.9-22-22z"/>""",
    "Row",
    "Three identical bins. The amber is the only difference, which is why it reads as a chart.",
)

LIDS = svg(
    PLANK
    + "\n  "
    + canister("c0", 96, 50, 112, 18, 36, 78, 52, 0.82)
    + "\n  "
    + canister("c1", 256, 44, 168, 14, 40, 66, 48, 0.46)
    + "\n  "
    + canister("c2", 408, 40, 214, 12, 44, 70, 46, 0.26),
    "Lids",
    "Three canisters on a plank. Each lid is wider than its body. The tallest holds the least.",
)

SHOULDER = svg(
    preserve("s0", 102, 74, 128, 64, 30, 16, 64, 44, 0.86, base=400)
    + "\n  "
    + preserve("s1", 262, 62, 140, 92, 18, 36, 46, 36, 0.38, base=400)
    + "\n  "
    + preserve("s2", 408, 52, 116, 108, 14, 56, 40, 34, 0.22, base=400),
    "Shoulder",
    "Three preserve jars. The shoulder is the wide curve under a narrow neck. Feet show, so they are not tubes.",
)

MISMATCHED = svg(
    PLANK
    + "\n  "
    + jam("m0", 100, 62, 150, 90, 50, 0.84)
    + "\n  "
    + bottle("m1", 268, 48, 42, 16, 96, 66, 150, 34, 0.55)
    + "\n  "
    + tin("m2", 420, 48, 196, 68, 42, 0.40),
    "Mismatched",
    "A jam jar, an oil bottle, and a tin. Three amounts, three different outlines.",
)

FLARE = svg(
    crock("f0", 100, 86, 44, 26, 148, 50, 0.76, base=408)
    + "\n  "
    + crock("f1", 272, 68, 32, 20, 220, 46, 0.30, base=408)
    + "\n  "
    + crock("f2", 424, 60, 26, 18, 112, 44, 0.52, base=408),
    "Flare",
    "Three open crocks. The rim steps out past the wall, and the wall narrows to the foot.",
)

CONCEPTS = [
    {
        "slug": "lids",
        "name": "Lids",
        "idea": "Canisters on a plank. The lid is the widest part of each one.",
        "verdict": "Holds at 32 px: lid, neck, and body are three widths. At 16 px the neck is one pixel and the lids still overhang.",
        "svg": LIDS,
    },
    {
        "slug": "shoulder",
        "name": "Shoulder",
        "idea": "Preserve jars. The neck is narrow and the shoulder is the curve under the cap.",
        "verdict": "Holds at 32 px: neck, shoulder, and foot all show. At 16 px the shoulders stay stepped and the necks get thin.",
        "svg": SHOULDER,
    },
    {
        "slug": "mismatched",
        "name": "Mismatched",
        "idea": "Jam jar, oil bottle, tin. The amounts differ because the containers differ.",
        "verdict": "Holds at 32 px. Jam cap, bottle neck, and square tin stay different. At 16 px the neck is one column and the three masses still do not match.",
        "svg": MISMATCHED,
    },
    {
        "slug": "flare",
        "name": "Flare",
        "idea": "Crocks, wide at the mouth and narrow at the foot.",
        "verdict": "Holds at 32 px: the rim steps out and the mouth stays open. At 16 px the mouths close into tapered lumps, which is still not a column.",
        "svg": FLARE,
    },
]

REFERENCES = [
    {
        "slug": "shelf",
        "name": "Shelf",
        "idea": "The mark in the app today. A parcel on a plank.",
        "verdict": "Holds at 16 px. The parcel and the plank stay separate.",
        "svg": None,
    },
    {
        "slug": "row",
        "name": "Row",
        "idea": "The #145 riff. Three matching bins, three amounts.",
        "verdict": "Holds at 32 px. At 16 px the gaps collapse and it reads as one chart.",
        "svg": ROW_SVG,
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


def text_width(draw, text, face, tracking):
    return sum(draw.textlength(ch, font=face) + tracking for ch in text) - tracking


def draw_tracked(draw, xy, text, face, fill, tracking):
    x, y = xy
    for ch in text:
        draw.text((x, y), ch, font=face, fill=fill, anchor="ls")
        x += draw.textlength(ch, font=face) + tracking
    return x


def wrap(draw, text, face, width):
    lines, cur = [], ""
    for word in text.split():
        trial = word if not cur else f"{cur} {word}"
        if draw.textlength(trial, font=face) <= width:
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
    face = font(INTER_BOLD, 96)
    tracking = -1.2
    probe = ImageDraw.Draw(Image.new("RGB", (1, 1)))
    tw = text_width(probe, "Pantry", face, tracking)
    w = pad_x + mark_px + gap + int(tw) + pad_x
    h = pad_y + mark_px + pad_y
    im = Image.new("RGB", (w, h), WHITE)
    icon = open_rgba(mark_path).resize((mark_px, mark_px), Image.Resampling.LANCZOS)
    paste(im, icon, (pad_x, pad_y))
    draw = ImageDraw.Draw(im)
    ascent, _descent = face.getmetrics()
    cap = ascent * 0.727
    baseline = pad_y + mark_px / 2 + cap / 2
    draw_tracked(draw, (pad_x + mark_px + gap, baseline), "Pantry", face, INK, tracking)
    im.save(path)
    return im


def blow(path, scale):
    im = open_rgba(path)
    return im.resize((im.width * scale, im.height * scale), Image.Resampling.NEAREST)


def framed(im):
    framed_im = Image.new("RGBA", (im.width + 2, im.height + 2), RULE)
    framed_im.paste(im, (1, 1), im)
    return framed_im


def write_mark(folder, text):
    folder.mkdir(parents=True, exist_ok=True)
    (folder / "mark.svg").write_text(text)
    for size, name in ((512, "icon-512.png"), (48, "mark-48.png"), (32, "mark-32.png"), (16, "mark-16.png")):
        render_svg(text, folder / name, size)
    lockup(folder / "icon-512.png", folder / "lockup.png")


def render_marks():
    shelf_svg = SHELF_SVG_PATH.read_text()
    write_mark(HERE / "references" / "shelf", shelf_svg)
    write_mark(HERE / "references" / "row", ROW_SVG)
    for concept in CONCEPTS:
        write_mark(HERE / concept["slug"], concept["svg"])


def board(concept):
    name_f = font(INTER_BOLD, 40)
    idea_f = font(INTER_REG, 22)
    label_f = font(INTER_SEMI, 16)
    verdict_f = font(INTER_REG, 20)
    small_f = font(INTER_REG, 15)

    margin = 40
    icon = 420
    gap = 28
    width = margin + icon * 3 + gap * 2 + margin
    probe = ImageDraw.Draw(Image.new("RGB", (1, 1)))
    verdict_lines = wrap(probe, concept["verdict"], verdict_f, width - margin * 2)
    height = margin + 52 + 36 + icon + 36 + 168 + 28 + 28 * len(verdict_lines) + margin
    im = Image.new("RGB", (width, height), WHITE)
    draw = ImageDraw.Draw(im)

    y = margin
    draw.text((margin, y), concept["name"], font=name_f, fill=INK)
    y += 52
    draw.text((margin, y), concept["idea"], font=idea_f, fill=MUTED)
    y += 46
    icon_y = y
    slots = (
        (HERE / concept["slug"] / "icon-512.png", concept["name"]),
        (HERE / "references" / "shelf" / "icon-512.png", "Shelf, current"),
        (HERE / "references" / "row" / "icon-512.png", "Row, from #145"),
    )
    for i, (path, label) in enumerate(slots):
        x = margin + i * (icon + gap)
        pic = open_rgba(path).resize((icon, icon), Image.Resampling.LANCZOS)
        paste(im, pic, (x, icon_y))
        draw.text((x, icon_y + icon + 10), label, font=label_f, fill=MUTED)

    y = icon_y + icon + 44
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
    lock.thumbnail((width - margin - 480, 150), Image.Resampling.LANCZOS)
    im.paste(lock, (margin + 480, y))
    draw.text((margin + 480, y + lock.height + 6), "Header", font=small_f, fill=MUTED)

    y = y + 176
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

    rows = []
    for ref in REFERENCES:
        folder = "references/" + ref["slug"]
        rows.append({**ref, "folder": folder})
    for concept in CONCEPTS:
        rows.append({**concept, "folder": concept["slug"]})

    width = 1560
    margin = 40
    name_w = 280
    mark = 168
    blow32 = 32 * 4 + 2
    blow16 = 16 * 8 + 2
    lock_w = 340
    row_h = 248
    header_h = 148
    footer_h = 64
    height = header_h + len(rows) * row_h + footer_h

    im = Image.new("RGB", (width, height), WHITE)
    draw = ImageDraw.Draw(im)
    draw.text((margin, 28), "Row, redrawn as pantry containers", font=title_f, fill=INK)
    draw.text(
        (margin, 76),
        "Review only. These do not replace the Shelf mark, and they are not wired into the app.",
        font=sub_f,
        fill=MUTED,
    )
    draw.text(
        (margin, 102),
        "Issue #147. The Row riff from #145 is included so the chart problem sits next to the redraw.",
        font=sub_f,
        fill=MUTED,
    )

    cols = {
        "mark": margin + name_w,
        "s48": margin + name_w + mark + 28,
        "s32": margin + name_w + mark + 28 + 72,
        "s16": margin + name_w + mark + 28 + 72 + blow32 + 18,
        "lock": margin + name_w + mark + 28 + 72 + blow32 + 18 + blow16 + 24,
    }
    label_y = header_h - 22
    draw.text((cols["mark"], label_y), "Mark", font=head_f, fill=MUTED)
    draw.text((cols["s48"], label_y), "48 px", font=head_f, fill=MUTED)
    draw.text((cols["s32"], label_y), "32 px", font=head_f, fill=MUTED)
    draw.text((cols["s16"], label_y), "16 px", font=head_f, fill=MUTED)
    draw.text((cols["lock"], label_y), "Header", font=head_f, fill=MUTED)
    draw.line((margin, header_h - 1, width - margin, header_h - 1), fill=RULE)

    for i, row in enumerate(rows):
        top = header_h + i * row_h
        draw.line((margin, top + row_h - 1, width - margin, top + row_h - 1), fill=RULE)
        folder = HERE / row["folder"]
        cy = top + row_h / 2
        draw.text((margin, top + 36), row["name"], font=name_f, fill=INK)
        ty = top + 68
        for line in wrap(draw, row["idea"], idea_f, name_w - 20):
            draw.text((margin, ty), line, font=idea_f, fill=MUTED)
            ty += 20
        ty += 8
        for line in wrap(draw, row["verdict"], idea_f, name_w - 20):
            draw.text((margin, ty), line, font=idea_f, fill=INK)
            ty += 20

        icon = open_rgba(folder / "icon-512.png").resize((mark, mark), Image.Resampling.LANCZOS)
        paste(im, icon, (cols["mark"], int(cy - mark / 2)))
        paste(im, open_rgba(folder / "mark-48.png"), (cols["s48"], int(cy - 24)))
        b32 = framed(blow(folder / "mark-32.png", 4))
        paste(im, b32, (cols["s32"], int(cy - b32.height / 2)))
        b16 = framed(blow(folder / "mark-16.png", 8))
        paste(im, b16, (cols["s16"], int(cy - b16.height / 2)))
        lock = Image.open(folder / "lockup.png").convert("RGBA")
        scale = min(lock_w / lock.width, 120 / lock.height)
        lock = lock.resize((int(lock.width * scale), int(lock.height * scale)), Image.Resampling.LANCZOS)
        paste(im, lock, (cols["lock"], int(cy - lock.height / 2)))

    draw.text(
        (margin, height - 40),
        "Enlarged 32 px and 16 px cells are nearest-neighbor, so the squares are the real favicon pixels.",
        font=foot_f,
        fill=MUTED,
    )
    im.save(HERE / "comparison-sheet.png")


def ascii_mark(path):
    im = Image.open(path).convert("RGB").resize((16, 16), Image.Resampling.NEAREST)
    lines = []
    for y in range(16):
        chars = []
        for x in range(16):
            r, g, b = im.getpixel((x, y))
            if r > 180 and g > 120 and b < 140:
                chars.append("A")
            elif r > 210 and g > 200 and b > 180:
                chars.append("C")
            elif g > r and g > 70:
                chars.append(".")
            else:
                chars.append("?")
        lines.append("".join(chars))
    return "\n".join(lines)


def main():
    render_marks()
    for concept in CONCEPTS:
        board(concept)
    comparison_sheet()
    print("rendered", HERE)
    for concept in CONCEPTS:
        print("\n", concept["name"], "16px  (C cream, A amber, . tile)")
        print(ascii_mark(HERE / concept["slug"] / "mark-16.png"))
        print(concept["name"], "32px")
        im = Image.open(HERE / concept["slug"] / "mark-32.png").convert("RGB")
        # 32 is busy; print every pixel anyway, wrapped by the function at 16.
        # A 32-wide map is still readable in a log.
        lines = []
        for y in range(32):
            chars = []
            for x in range(32):
                r, g, b = im.getpixel((x, y))
                if r > 180 and g > 120 and b < 140:
                    chars.append("A")
                elif r > 210 and g > 200 and b > 180:
                    chars.append("C")
                elif g > r and g > 70:
                    chars.append(".")
                else:
                    chars.append("?")
            lines.append("".join(chars))
        print("\n".join(lines))


if __name__ == "__main__":
    main()
