package shopping

import (
	"sort"
	"strings"
	"unicode"
)

// storeBrandNames are grocery private labels removed from a product name before
// replenishment items are compared. The list is an allowlist on purpose: only a
// whole-token phrase from this list is treated as a brand, so qualifiers such as
// "organic", "no salt added", or a pack size stay part of the identity.
//
// National brands that are not listed (for example "Del Monte") stay distinct.
// Products whose names are already identical match without any brand being removed.
// Names that differ only by punctuation or case match after normalization.
var storeBrandNames = []string{
	"Kroger",
	"Great Value",
	"Western Family",
	"Simple Truth",
	"Private Selection",
	"Kirkland Signature",
	"Kirkland",
	"Good & Gather",
	"Market Pantry",
	"Favorite Day",
	"Member's Mark",
	"Food Club",
	"Essential Everyday",
	"Best Choice",
	"That's Smart!",
	"Lucerne",
	"Open Nature",
	"O Organics",
	"Signature Select",
	"Signature Kitchens",
	"Signature Farms",
	"Always Save",
	"Best Yet",
	"ShurFine",
	"Shur Fine",
	"Clear Value",
	"Full Circle",
	"Wild Harvest",
	"Our Family",
	"Hy-Vee",
	"WinCo",
	"Fred Meyer",
	"Harris Teeter",
	"Bowl & Basket",
	"Nature's Promise",
	"Guaranteed Value",
	"Gold Emblem",
	"Up & Up",
	"Walmart",
	"Costco",
	"Trader Joe's",
	"365 Everyday Value",
	"365 by Whole Foods Market",
	"Whole Foods Market",
	"Sam's Club",
	"Big K",
	"Valu Time",
	"Centrella",
	"Happy Belly",
	"Amazon Fresh",
	"Price Rite",
}

// storeBrandPhrases is storeBrandNames after the same tokenization applied to
// product names, longest phrase first so "Kirkland Signature" is removed as a
// unit rather than leaving a leftover "Signature".
var storeBrandPhrases [][]string

func init() {
	storeBrandPhrases = make([][]string, 0, len(storeBrandNames))
	for _, name := range storeBrandNames {
		tokens := normalizeTokens(name)
		if len(tokens) == 0 {
			continue
		}
		storeBrandPhrases = append(storeBrandPhrases, tokens)
	}
	sort.Slice(storeBrandPhrases, func(i, j int) bool {
		if len(storeBrandPhrases[i]) != len(storeBrandPhrases[j]) {
			return len(storeBrandPhrases[i]) > len(storeBrandPhrases[j])
		}
		return strings.Join(storeBrandPhrases[i], " ") < strings.Join(storeBrandPhrases[j], " ")
	})
}

// GenericProductName is the replenishment identity of a product name: case,
// punctuation, and recognized store-brand phrases are removed. An empty result
// means the name was only a brand, and that product is not grouped with others.
func GenericProductName(name string) string {
	tokens := normalizeTokens(name)
	removed := true
	for removed {
		removed = false
		for _, phrase := range storeBrandPhrases {
			next, ok := removePhrase(tokens, phrase)
			if !ok {
				continue
			}
			tokens = next
			removed = true
			break
		}
	}
	return strings.Join(tokens, " ")
}

// ReplenishmentItem is one pantry item considered when deciding what to buy.
// Items with HasTarget false still contribute CurrentCount when they are the
// same need as an item the user wants to keep in stock.
type ReplenishmentItem struct {
	ItemID         string
	Name           string
	UnitOfMeasure  string
	HasTarget      bool
	TargetQuantity int
	CurrentCount   int
}

// CollapseEquivalentNeeds pools on-hand stock across items that are the same
// product sold under different recognized store brands, and returns one
// DeriveInput per remaining need.
//
// Two items share a need when GenericProductName is the same non-empty value
// and the unit of measure normalizes to the same string (a blank unit does not
// match a specified unit, because the counts may not be interchangeable).
// The group's target is the largest individual target — one supply, not the
// sum of per-brand supplies — and its on-hand count is the sum of every member,
// including members with no target. The shopping line is attached to the member
// with the highest target, then the lexicographically first product name.
//
// A manual shopping-list row for any member suppresses the auto line for the
// whole need, the same way a manual row already overrides that item's own gap.
// This function only decides how much of the shared need is missing. Which
// brand the line buys is ApplyPreferences; a sale on another member is an
// Offer from ConsiderationsForLines that the shopper can accept at export.
func CollapseEquivalentNeeds(items []ReplenishmentItem, manualItemIDs map[string]struct{}) []DeriveInput {
	type needGroup struct {
		items []ReplenishmentItem
	}

	groups := make([]*needGroup, 0)
	index := make(map[string]int, len(items))
	for _, item := range items {
		key, groupable := equivalenceKey(item.Name, item.UnitOfMeasure)
		if !groupable {
			key = "\x00" + item.ItemID
		}
		i, seen := index[key]
		if !seen {
			i = len(groups)
			index[key] = i
			groups = append(groups, &needGroup{})
		}
		groups[i].items = append(groups[i].items, item)
	}

	result := make([]DeriveInput, 0)
	for _, group := range groups {
		if groupHasManual(group.items, manualItemIDs) {
			continue
		}
		targeted := make([]ReplenishmentItem, 0, len(group.items))
		total := 0
		for _, item := range group.items {
			total += item.CurrentCount
			if item.HasTarget {
				targeted = append(targeted, item)
			}
		}
		if len(targeted) == 0 {
			continue
		}
		rep := targeted[0]
		for _, item := range targeted[1:] {
			if preferRepresentative(item, rep) {
				rep = item
			}
		}
		result = append(result, DeriveInput{
			ItemID:         rep.ItemID,
			TargetQuantity: rep.TargetQuantity,
			CurrentCount:   total,
		})
	}
	return result
}

func equivalenceKey(name, unit string) (string, bool) {
	generic := GenericProductName(name)
	if generic == "" {
		return "", false
	}
	return generic + "\x00" + strings.Join(normalizeTokens(unit), " "), true
}

func groupHasManual(items []ReplenishmentItem, manualItemIDs map[string]struct{}) bool {
	if len(manualItemIDs) == 0 {
		return false
	}
	for _, item := range items {
		if _, ok := manualItemIDs[item.ItemID]; ok {
			return true
		}
	}
	return false
}

// preferRepresentative reports whether candidate should stand in for the shared
// need instead of current. Higher targets win so the line follows the supply
// the user asked to keep; equal targets use the product name so the choice does
// not move when stock counts change.
func preferRepresentative(candidate, current ReplenishmentItem) bool {
	if candidate.TargetQuantity != current.TargetQuantity {
		return candidate.TargetQuantity > current.TargetQuantity
	}
	if candidate.Name != current.Name {
		return candidate.Name < current.Name
	}
	return candidate.ItemID < current.ItemID
}

func normalizeTokens(s string) []string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "&", " and ")

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '®', '™', '©', '\'', '’', '"', '“', '”':
			// Drop marks that do not separate words, so "Member's" and
			// "Members" tokenize the same way.
		case '-', '/', ',', '.', '(', ')', ':', ';', '!', '?', '+':
			b.WriteByte(' ')
		default:
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '%' {
				b.WriteRune(r)
			} else if unicode.IsSpace(r) {
				b.WriteByte(' ')
			}
		}
	}
	return strings.Fields(b.String())
}

func removePhrase(tokens, phrase []string) ([]string, bool) {
	if len(phrase) == 0 || len(phrase) > len(tokens) {
		return tokens, false
	}
	for i := 0; i+len(phrase) <= len(tokens); i++ {
		match := true
		for j := range phrase {
			if tokens[i+j] != phrase[j] {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		out := make([]string, 0, len(tokens)-len(phrase))
		out = append(out, tokens[:i]...)
		out = append(out, tokens[i+len(phrase):]...)
		return out, true
	}
	return tokens, false
}
