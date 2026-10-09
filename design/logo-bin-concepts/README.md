# Bin logo concepts

Review only. These mockups do not replace the Shelf mark, they are not wired into the app, and merging this branch does not close issue #143.

Shelf stays the live mark (`frontend/public/favicon.svg`, `internal/brand/logo.png`). `shelf-current/` is a fresh render of `internal/brand/concepts/shelf.svg` so each riff can sit next to it.

## How to look

`comparison-sheet.png` is the whole set. Each concept folder has the same files:

| File | What it is |
| --- | --- |
| `mark.svg` | Source |
| `icon-512.png` | App icon |
| `mark-48.png` | Favicon scale, actual size |
| `mark-32.png`, `mark-16.png` | The sizes the notes below talk about |
| `lockup.png` | The mark beside the word Pantry |
| `board.png` | That concept next to Shelf, with the small sizes enlarged |

The 32 px and 16 px cells on the sheet are nearest-neighbor enlargements. The squares are the real pixels, not a blurry shrink. `render.py` rebuilds the PNGs from the SVG marks.

## Shared choices

The tile, the amber, and the cream are the Shelf palette, so the comparison is the silhouette and not a new color story.

| | |
| --- | --- |
| Tile | `#1C7C4E` |
| Rim | `#14281C` |
| Cream | `#F6F1E6` |
| Amber | `#F0B429` |
| Glacier, punch | `#2C6E97`, `#E15A2D`, and only on Members |

The punch red is the red already used in the tag concept. Glacier is the second member swatch.

"Pantry" is Inter Bold. That is the header word, set so the cap height lines up with the mark. The bin mockups use Fraunces for group names on the page. Using it here would make the lockup a different product from the header these marks are meant to fit.

Each concept spends its difference on one silhouette. No gradients, no food drawings, no lettering inside the tile except the monogram.

## Favicon

Judged from the 16 px and 32 px rasters, not from the 512 px drawing.

| Concept | 32 px | 16 px |
| --- | --- | --- |
| Shelf, current | Holds. Parcel and plank separate. | Holds. |
| Level | Holds. | Holds. Rim, empty well, and amber stay separate. |
| Measure | Holds. Brim, neck, and fill are clear. | Holds, smaller. The brim still sticks out. The amber is a short block, not a speck. |
| Row | Holds. Low, middle, and high are obvious. | Does not hold. The gaps collapse and the three heights barely separate. |
| Plank | Holds. | Close to Shelf. Both are a shape over a cream bar. The pale well above the amber is the difference. |
| Members | Holds. Three swatches in a bin. | The bin holds. Each swatch is about two pixels wide, so the colors are specks. |
| Monogram | Holds. The fill line in the counter is obvious. | The P holds. The fill is there and thin. It reads as a letter before it reads as a bin. |

Level is the one that survives smallest without giving up the bin. Row is the one that needs the larger sizes. A dark rim on each bin in the row would have erased the levels, so those three are solid cream and amber.

## The six

**Level.** One tub. The flat top of the amber is how full it is. This is the bin from the group mockup, drawn with the same masses as Shelf: a few shapes, no outline stroke.

**Measure.** A tall bin used as a gauge. The brim is wider than the cup, and the amber stops short of it. The empty neck is the shortfall.

**Row.** Three amounts for one household. A vertical stack would redo the existing Stack concept (two offset cards in `internal/brand/concepts/stack.svg`), so these sit in a row.

**Plank.** The Shelf plank is the same rectangle as the live mark. A bin stands where the parcel was.

**Members.** One bin, and a swatch for each product standing in it. Amber, glacier, and punch are the member colors, not a new palette. The empty cream above them is the unfilled part of the bin.

**Monogram.** A P whose counter is the bin. It locks the initial. The other concepts do not.
