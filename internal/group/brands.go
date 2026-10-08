package group

import (
	"sort"
	"strings"
	"unicode"
)

// storeBrandNames are private labels removed before two names are compared.
// Only a whole-token phrase from this list is a brand, so "organic" and a pack
// size stay part of the food. National brands that are not listed stay distinct.
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

func normalizeTokens(s string) []string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "&", " and ")

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '®', '™', '©', '\'', '’', '"', '“', '”':
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

func stripStoreBrands(tokens []string) []string {
	removed := true
	for removed {
		removed = false
		for _, phrase := range storeBrandPhrases {
			next, ok := removeBrandPhrase(tokens, phrase)
			if !ok {
				continue
			}
			tokens = next
			removed = true
			break
		}
	}
	return tokens
}

// needKey is the old shopping-plan identity, still used to match a saved
// brand preference to a suggestion. An empty commodity name is not a need.
func needKey(name, unit string) (string, bool) {
	generic := strings.Join(stripStoreBrands(normalizeTokens(name)), " ")
	if generic == "" {
		return "", false
	}
	return generic + "\x00" + strings.Join(normalizeTokens(unit), " "), true
}

func removeBrandPhrase(tokens, phrase []string) ([]string, bool) {
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
