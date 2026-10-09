# Product group page — icon and button mockups (#148)

Static mockups only. The Pantry app is unchanged. Open `index.html` in a browser to compare the current page with three shorter directions, at the widths the issue asks for.

Same stock as the live copy:

- Gatorade powder: 18.3 oz of Lemon-Lime in the bin, keep 48 oz, rule “same as what ran out”
- Peanut butter: no jars, keep 3 months, always buy Creamy peanut butter
- Oat milk: no cartons, household default of 3 months, rule not picked yet

The bin stays the hero. New controls are at least 44px. An icon without a visible word keeps the current accessible name (`Rename`, `Remove or move …`). The sentence that used to be on the page stays as the control’s tooltip.

## Word count

Visible words under the app header. A word is a token that contains a letter or a digit. Slashes and dots are not words. Tooltips, accessible names, and the search placeholder are not counted. List totals are the three rows only. Search, filters, and “New group” are the same on every list (25 words, plus the placeholder “Name or product”) and are left out.

| Direction | Group detail | Three list rows | Detail vs current | List vs current |
| --- | ---: | ---: | --- | --- |
| Current | 78 | 53 | — | — |
| Fraction | 31 | 23 | 47 fewer | 30 fewer |
| Chips | 37 | 31 | 41 fewer | 22 fewer |
| Toolbar | 35 | 23 | 43 fewer | 30 fewer |

## Between Chips and Fraction

The second round keeps the bin’s on-hand number and a rule name the filters already use (“Same as ran out”). It adds where the target came from.

Gatorade’s 48 oz is calculated: about 16 oz a month, kept for 3 months. That rate is behind a tap. Peanut butter says “You set 3 months” with nothing folded away. Oat milk says “Default”, because the household setting produced 3 months and there is no rate yet.

| Direction | Detail, closed | Detail, Auto open | Three list rows | Detail vs current | List vs current |
| --- | ---: | ---: | ---: | --- | --- |
| Badge | 37 | 53 | 31 | 41 fewer | 22 fewer |
| Line | 37 | 53 | 31 | 41 fewer | 22 fewer |
| Stamp | 37 | 53 | 30 | 41 fewer | 23 fewer |

**Badge.** 48 oz and a blue Auto pill. The rate opens in a small note. Manual targets are an underlined “You set 3 months”, not a pill.

**Line.** Auto is a word and a chevron. The rate opens as a sentence under the number, without a card.

**Stamp.** 48 oz stays in the bin corner, and “auto” sits inside that button. The rate opens under the bin.

## Directions

**Fraction.** The bin is the picture and the name. On-hand and target are one tappable readout, `18.3 / 48 oz`. The rule is a pill, “Same kind”. Months do not use a slash: Peanut butter and Oat milk stack short lines. Shortest page, and the one that introduces a nickname.

**Chips.** The bin keeps `48 oz` (now the target button) and `18.3 oz` on the fill. The rule pill uses the name the filters already use, “Same as what ran out”. List rows are badges. Nothing new to memorize; the real rule name costs five words.

**Toolbar.** Three labeled buttons in a row: Rename, `48 oz`, “Same kind”. The bin keeps `18.3 oz` on the fill and does not repeat the target. Easiest to read without a tooltip, and the most chrome.

All three replace “Remove or move” with a kebab. The open menu is the last frame: Remove, or move to Peanut butter or Oat milk, each a 44px row.

## Screenshots

`screenshots/` holds one PNG per frame. The first round is the current page, Fraction, Chips, and Toolbar (detail at 390 and 1440, list at 390), plus the open member menu. The second round is Badge, Line, and Stamp at 390: detail closed, detail with Auto open, and the groups list.

Regenerate with Chrome on PATH:

```
node design/group-page-icons-148/shoot.mjs
```
