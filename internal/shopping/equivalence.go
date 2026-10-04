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

// ReplenishmentItem is one pantry item that can fill a shared need.
// How many units to buy is decided by the supply plan, not by a target on this item.
type ReplenishmentItem struct {
	ItemID         string
	Name           string
	UnitOfMeasure  string
	HasTarget      bool
	TargetQuantity int
	CurrentCount   int
}

// SameNeed reports whether two products are one replenishment need. A blank
// unit does not match a specified unit.
func SameNeed(nameA, unitA, nameB, unitB string) bool {
	a, aOK := equivalenceKey(nameA, unitA)
	b, bOK := equivalenceKey(nameB, unitB)
	return aOK && bOK && a == b
}

func equivalenceKey(name, unit string) (string, bool) {
	generic := GenericProductName(name)
	if generic == "" {
		return "", false
	}
	return generic + "\x00" + strings.Join(normalizeTokens(unit), " "), true
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
