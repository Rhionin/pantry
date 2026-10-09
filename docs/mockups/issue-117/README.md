# Product group mockups for issue 117

Static mockups only. Nothing here changes the app. Merging this branch does not close issue 117.

Each direction is shown at 390px and 1440px, for the group detail page and the Product groups list. Screenshots are 2× captures of those layouts. The sample group is Gatorade powder: one 18.3 oz Lemon-Lime canister on hand, a 48 oz target, and the rule “buy the same kind that ran out.” Glacier Freeze (50.9 oz) and Fruit Punch (18.3 oz) are members with none on hand. The list also shows a months target, the household default (3 months), and a group that still needs a rule.

These follow Anthropic’s frontend-design skill (fetched from the skills repo; it was not in this repo yet). Shared choices from that skill: the resting page answers what is in the group, how much is on hand versus the target, and what gets bought when it runs out. Rename, change target, change rule, add a product, and remove or move are actions. The ounces field, months field, and household default are not left open on the page. The target action still chooses ounces, a number of months, or the household default.

## The bin

The group is a bin. Its height is the 48 ounces to keep, and the lemon powder in it is the 18.3 ounces on hand. Fraunces is used only for names, the way a label is lettered. Atkinson Hyperlegible is for everything you read while standing at the shelf. The fill is the one bold move. The list repeats the bin at thumbnail size, with a yellow fill only when the target is ounces.

Optimizes seeing stock against the target. Sacrifices a precise comparison when the target is months or the household default: those groups stay as sentences, with an empty bin.

## The answer

The first thing on the page is the rule, set large in Petrona: “When this runs out, buy the same kind that ran out.” Stock and target are the next sentence. Source Sans 3 is only for controls and package facts. The page is almost one color, left aligned, and stays a single column on a wide screen. The list is the same kind of sentence, with the group name as the link.

Optimizes the buying decision. Sacrifices scanning many members and lining up ounces. A long group becomes a long note.

## The scale

Schibsted Grotesk throughout, with tabular numbers. The page opens on the gap, in one cobalt sentence: “29.7 ounces short of the 48 you keep.” The rule sits beside it. Members are a tally: size, how many are on hand, and the ounces they add, with one line above the total. The list uses the same columns, and an ounce group that is short uses the same cobalt. “Pick a rule” stays an action.

Optimizes checking the arithmetic and managing members. Sacrifices the feeling of a pantry. Months and the household default do not share a unit with ounces, so the list mixes them in one column.
