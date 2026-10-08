package group

import (
	"strings"
	"unicode"

	"github.com/Rhionin/pantry/internal/shopping"
)

// Match is one product name after look-alike normalization.
type Match struct {
	Name    string
	Key     string
	Caution string
}

// differentiators stay out of the key and set the caution sentence.
// The order is the order a caution is chosen when more than one matches.
var differentiators = []string{
	"organic",
	"crunchy",
	"creamy",
	"french style",
	"no salt",
	"unsalted",
	"low sodium",
	"whole",
}

// identityWords are food words a brand strip must not eat.
// "cut green beans" and "green beans" stay different foods.
var identityWords = map[string]struct{}{
	"cut": {},
}

var packageWords = map[string]struct{}{
	"can": {}, "cans": {}, "jar": {}, "jars": {}, "box": {}, "boxes": {},
	"bag": {}, "bags": {}, "bottle": {}, "bottles": {}, "pack": {}, "packs": {},
	"ct": {}, "pk": {},
}

var unitWords = map[string]struct{}{
	"oz": {}, "ounce": {}, "ounces": {}, "fl": {}, "floz": {}, "fluid": {},
	"g": {}, "gram": {}, "grams": {}, "ml": {}, "l": {}, "kg": {}, "lb": {}, "lbs": {},
	"gallon": {}, "gallons": {}, "liter": {}, "liters": {}, "litre": {}, "litres": {},
	"milliliter": {}, "milliliters": {}, "kilogram": {}, "kilograms": {},
	"pound": {}, "pounds": {},
}

// LookAlikeKey is the only name matcher.
// known holds keys other products or groups already have, not this name's own key.
// A leading brand is dropped only when the remainder is already one of those keys.
func LookAlikeKey(name string, known map[string]struct{}) (key, caution string) {
	base, caution := baseLookAlike(name)
	if base == "" {
		return "", caution
	}
	return shorten(base, known), caution
}

// ResolveLookAlikes assigns keys across a set of names.
// extraKeys are look-alike keys an existing group already has.
func ResolveLookAlikes(names []string, extraKeys []string) []Match {
	bases := make([]string, len(names))
	cautions := make([]string, len(names))
	for i, name := range names {
		bases[i], cautions[i] = baseLookAlike(name)
	}
	out := make([]Match, len(names))
	for i, name := range names {
		known := map[string]struct{}{}
		for _, k := range extraKeys {
			if k != "" {
				known[k] = struct{}{}
			}
		}
		for j, base := range bases {
			if j == i || base == "" {
				continue
			}
			known[base] = struct{}{}
		}
		key := bases[i]
		if key != "" {
			key = shorten(key, known)
		}
		out[i] = Match{Name: name, Key: key, Caution: cautions[i]}
	}
	return out
}

func baseLookAlike(name string) (string, string) {
	tokens := shopping.StripStoreBrands(shopping.NormalizeTokens(name))
	tokens, caution := stripDifferentiators(tokens)
	tokens = dropSizeAndPackage(tokens)
	return strings.Join(tokens, " "), caution
}

func stripDifferentiators(tokens []string) ([]string, string) {
	matched := map[string]bool{}
	phrases := make([]string, len(differentiators))
	copy(phrases, differentiators)
	// Longer phrases first so "french style" is removed as a phrase.
	for i := 0; i < len(phrases); i++ {
		for j := i + 1; j < len(phrases); j++ {
			if len(strings.Fields(phrases[j])) > len(strings.Fields(phrases[i])) {
				phrases[i], phrases[j] = phrases[j], phrases[i]
			}
		}
	}
	for _, label := range phrases {
		words := strings.Fields(label)
		for {
			next, ok := removeWords(tokens, words)
			if !ok {
				break
			}
			tokens = next
			matched[label] = true
		}
	}
	for _, label := range differentiators {
		if matched[label] {
			return tokens, cautionSentence(label)
		}
	}
	return tokens, ""
}

func cautionSentence(label string) string {
	r := []rune(label)
	if len(r) == 0 {
		return ""
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r) + ". Probably not the same."
}

func dropSizeAndPackage(tokens []string) []string {
	drop := make([]bool, len(tokens))
	for i, tok := range tokens {
		if hasDigit(tok) || hasWord(unitWords, tok) || hasWord(packageWords, tok) || tok == "x" {
			drop[i] = true
		}
	}
	for i, tok := range tokens {
		if tok != "half" {
			continue
		}
		if (i+1 < len(tokens) && isGallon(tokens[i+1])) || (i > 0 && isGallon(tokens[i-1])) {
			drop[i] = true
		}
	}
	out := make([]string, 0, len(tokens))
	for i, tok := range tokens {
		if !drop[i] {
			out = append(out, tok)
		}
	}
	return out
}

func shorten(base string, known map[string]struct{}) string {
	if base == "" {
		return ""
	}
	if known == nil {
		return base
	}
	if _, ok := known[base]; ok {
		return base
	}
	tokens := strings.Fields(base)
	for n := 1; n <= len(tokens)-2; n++ {
		if containsIdentity(tokens[:n]) {
			return base
		}
		rest := strings.Join(tokens[n:], " ")
		if _, ok := known[rest]; ok {
			return rest
		}
	}
	return base
}

func containsIdentity(tokens []string) bool {
	for _, tok := range tokens {
		if hasWord(identityWords, tok) {
			return true
		}
	}
	return false
}

func removeWords(tokens, phrase []string) ([]string, bool) {
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

func hasDigit(tok string) bool {
	for _, r := range tok {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func hasWord(set map[string]struct{}, tok string) bool {
	_, ok := set[tok]
	return ok
}

func isGallon(tok string) bool {
	return tok == "gallon" || tok == "gallons"
}
