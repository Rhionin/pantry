# Row, redrawn as pantry containers

Review only. These mockups do not replace the Shelf mark, they are not wired into the app, and merging this branch does not close issue #147.

This is round 2 on the Row idea from #143 / draft PR #145. Row keeps three amounts for one household. The first drawing made those amounts out of three identical pills, so it read as a bar chart. These four keep the three amounts and change the outline: a lid, a shoulder, a mismatched set, or a flare. The amber is what is inside. It is not the silhouette.

Shelf stays the live mark (`frontend/public/favicon.svg`, `internal/brand/logo.png`). `references/shelf/` is a fresh render of `internal/brand/concepts/shelf.svg`. `references/row/` is the Row mark from #145, copied here so this round can sit next to the chart it is answering. Nothing under `frontend/` or `internal/brand/` changes.

An earlier pass at this round never landed on a branch. It still read as columns. The redraw pushes the lid past the body, the shoulder in from the wall, and the rim out past the foot until the outline is a container with the fill ignored.

## How to look

`comparison-sheet.png` is the whole set, with Shelf and Row on the first two rows. Each concept folder has the same files:

| File | What it is |
| --- | --- |
| `mark.svg` | Source |
| `icon-512.png` | App icon |
| `mark-48.png` | Favicon scale, actual size |
| `mark-32.png`, `mark-16.png` | The sizes the notes below talk about |
| `lockup.png` | The mark beside the word Pantry |
| `board.png` | That concept next to Shelf and Row, with the small sizes enlarged |

The 32 px and 16 px cells on the sheet are nearest-neighbor enlargements. The squares are the real pixels, not a blurry shrink. `render.py` rebuilds the PNGs from the SVG marks.

## Shared choices

The tile, the amber, and the cream are the Shelf palette, so the comparison is the silhouette and not a new color story.

| | |
| --- | --- |
| Tile | `#1C7C4E` |
| Cream | `#F6F1E6` |
| Amber | `#F0B429` |

"Pantry" is Inter Bold, the same lockup as the #145 boards, set so the cap height lines up with the mark.

Each concept spends its difference on the container outline. No gradients, no ticks, no shared axis, no food drawings. Where a plank appears, it is the thick Shelf plank, a surface the containers sit on, not a baseline under a chart. Lids and Mismatched use it. Shoulder and Flare leave the feet in the tile so the base of the vessel stays part of the silhouette.

The fill is not a staircase. On Lids, Shoulder, and Flare the tallest container holds the least, so height does not encode the amount.

## Favicon

Judged from the 16 px and 32 px rasters, not from the 512 px drawing.

| Concept | 32 px | 16 px |
| --- | --- | --- |
| Shelf, current | Holds. Parcel and plank separate. | Holds. |
| Row, from #145 | Holds. Low, middle, and high are obvious. | Does not hold. The gaps collapse and the three matching pills read as one chart. |
| Lids | Holds. Lid, neck, and body are three widths. The tall canister is the least full. | The neck shrinks to about a pixel, so the pinch softens. The lids still overhang the bodies. |
| Shoulder | Holds. Neck, shoulder, and rounded foot all show. | The shoulders stay stepped and the feet stay rounded. The necks get thin. |
| Mismatched | Holds. Jam cap, bottle neck, and square tin stay different outlines. | The bottle neck is one column. The three masses still do not match. |
| Flare | Holds. The rim steps out past the wall and the mouth stays open. | The mouths close. What is left is a tapered lump, which is still not a column. |

Mismatched is the one that reads as a pantry even before you notice the amber. Row is still the one that needs the larger sizes, and it needs them because the containers match. Lids and Shoulder hold the "three amounts" idea more clearly than Flare once the icon is small. Flare is the one that gives up the open mouth first.

## The four

**Lids.** Three canisters on the Shelf plank. The lid is a cap wider than the body, with a narrow neck between them, the way a tin lid sits on a canister. The left one is short and nearly full. The right one is the tallest and only has a little in the bottom.

**Shoulder.** Three preserve jars, no plank. The wall stays wide through the belly, then curves in to a neck under a cap that steps back out. One jar is full into the neck, one stops in the belly, one has barely been started. The rounded foot is part of the drawing so the shape cannot be a test tube.

**Mismatched.** A jam jar, an oil bottle, and a tin, on the plank. The jam cap is wider than the mouth. The bottle is narrow at the neck and the foot and wide only in the belly, and the amber stays in the belly so the neck reads empty. The tin has square corners and a lid that overhangs. Same three amounts, three outlines.

**Flare.** Three open crocks, no plank. The rim is a band that steps out past the wall, the mouth is a hole in that band, and the wall narrows to the foot. Wide and full, tall and spare, small and half full.
