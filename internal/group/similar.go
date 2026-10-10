package group

import (
	"strings"
)

// minOverlap is the share of the shorter name that must be the same food.
// categoryOverlap is the lower bar when both products already share a category.
const (
	minOverlap      = 0.66
	categoryOverlap = 0.60
)

// extraVarietyPhrases are stripped only for suggestion matching.
// They sit between a brand and the food, so an exact suffix would miss.
var extraVarietyPhrases = []string{
	"super chunk",
	"extra chunk",
	"chunky",
}

// varietyWords mark a dropped prefix as a real difference, not a brand.
// The suggestion keeps both products and unchecks the odd one out.
var varietyWords = map[string]struct{}{
	"italian": {}, "ranch": {}, "honey": {}, "roasted": {}, "smoked": {},
	"virgin": {}, "garlic": {}, "original": {}, "classic": {}, "traditional": {},
	"zesty": {}, "dill": {}, "spicy": {}, "mild": {}, "hot": {},
	"bbq": {}, "barbecue": {}, "vanilla": {}, "chocolate": {}, "strawberry": {},
}

// genericFoodWords are too broad to form a group on their own.
// "black beans" and "green beans" share "bean" and stay apart.
var genericFoodWords = map[string]struct{}{
	"bar": {}, "bean": {}, "beef": {}, "bread": {}, "broth": {}, "bun": {},
	"butter": {}, "cake": {}, "candy": {}, "cereal": {}, "cheese": {},
	"chicken": {}, "chip": {}, "coffee": {}, "cookie": {}, "corn": {},
	"cracker": {}, "cream": {}, "drink": {}, "egg": {}, "fish": {},
	"flake": {}, "flour": {}, "fruit": {}, "grain": {}, "gum": {},
	"juice": {}, "loaf": {}, "meat": {}, "milk": {}, "mint": {},
	"mix": {}, "noodle": {}, "nut": {}, "oil": {}, "onion": {},
	"pasta": {}, "pea": {}, "pepper": {}, "pork": {}, "powder": {},
	"rice": {}, "roll": {}, "salt": {}, "sauce": {}, "seed": {},
	"slice": {}, "snack": {}, "soda": {}, "soup": {}, "stock": {},
	"sugar": {}, "tea": {}, "tomato": {}, "tortilla": {}, "tuna": {},
	"water": {}, "wrap": {}, "yogurt": {},
}

type profile struct {
	tokens   []string
	category string
	caution  string
}

// similarMatches assigns a cluster key to each product.
// Two names match when the shorter food name is mostly contained in the longer
// one after store brands, sizes, and a few variety words are set aside.
// A shared category can confirm a near miss. It cannot create a match alone.
// extraKeys are food names an existing group already uses.
func similarMatches(products []seedProduct, extraKeys []string) []Match {
	profiles := make([]profile, len(products))
	for i, p := range products {
		tokens, caution := similarityTokens(p.name)
		profiles[i] = profile{tokens: tokens, category: normCategory(p.category), caution: caution}
	}
	var extras [][]string
	for _, key := range extraKeys {
		tokens, _ := similarityTokens(key)
		if len(tokens) > 0 {
			extras = append(extras, tokens)
		}
	}

	type edge struct {
		j   int
		key []string
	}
	edges := make([][]edge, len(products))
	best := make([][]string, len(products))
	for i := 0; i < len(profiles); i++ {
		for j := i + 1; j < len(profiles); j++ {
			key, ok := sharedFoodKey(profiles[i], profiles[j])
			if !ok {
				continue
			}
			edges[i] = append(edges[i], edge{j: j, key: key})
			edges[j] = append(edges[j], edge{j: i, key: key})
			best[i] = longerKey(best[i], key)
			best[j] = longerKey(best[j], key)
		}
		for _, extra := range extras {
			key, ok := sharedFoodKey(profiles[i], profile{tokens: extra})
			if !ok {
				continue
			}
			best[i] = longerKey(best[i], key)
		}
	}
	// A shorter overlapping match joins the longer cluster it actually touched.
	// "Ketchup" joins "tomato ketchup" instead of sitting in its own card.
	changed := true
	for changed {
		changed = false
		for i := range profiles {
			for _, e := range edges[i] {
				if best[i] == nil || best[e.j] == nil {
					continue
				}
				if len(best[e.j]) > len(best[i]) && hasSuffixTokens(best[e.j], best[i]) {
					best[i] = best[e.j]
					changed = true
				}
			}
		}
	}

	out := make([]Match, len(products))
	for i, p := range products {
		key := ""
		caution := profiles[i].caution
		if best[i] != nil {
			key = strings.Join(best[i], " ")
			if next := varietyCaution(profiles[i].tokens, best[i], caution); next != "" {
				caution = next
			}
		}
		out[i] = Match{Name: p.name, Key: key, Caution: caution}
	}
	return out
}

func sharedFoodKey(a, b profile) ([]string, bool) {
	if len(a.tokens) == 0 || len(b.tokens) == 0 {
		return nil, false
	}
	if tokensEqual(a.tokens, b.tokens) {
		return preferPlural(a.tokens, b.tokens), true
	}
	var best []string
	for _, sa := range suffixes(a.tokens) {
		for _, sb := range suffixes(b.tokens) {
			if len(sa) != len(sb) || !tokensEqual(sa, sb) {
				continue
			}
			base := overlapScore(len(sa), len(a.tokens), len(b.tokens))
			if !acceptOverlap(base, len(sa), a, b, sa) {
				continue
			}
			best = longerKey(best, preferPlural(sa, sb))
		}
	}
	if best == nil {
		return nil, false
	}
	return best, true
}

func acceptOverlap(base float64, sharedLen int, a, b profile, shared []string) bool {
	if sharedLen == 1 {
		return distinctiveSingle(a, b, shared[0])
	}
	if sharedLen >= 3 && base >= 0.75 {
		return true
	}
	need := minOverlap
	if sameCategory(a, b) {
		need = categoryOverlap
	}
	return base+1e-9 >= need
}

// distinctiveSingle groups a one-word food with its branded versions.
// Generic words stay out so "beans" does not pull in "black beans".
func distinctiveSingle(a, b profile, shared string) bool {
	if genericFood(shared) || len(stemToken(shared)) < 6 {
		return false
	}
	droppedA := len(a.tokens) - 1
	droppedB := len(b.tokens) - 1
	if droppedA < 0 || droppedB < 0 || droppedA > 2 || droppedB > 2 {
		return false
	}
	if droppedA == 0 && droppedB == 0 {
		return false
	}
	if categoriesDisagree(a.category, b.category) && droppedA > 0 && droppedB > 0 {
		return false
	}
	return true
}

func overlapScore(shared, aLen, bLen int) float64 {
	m := aLen
	if bLen < m {
		m = bLen
	}
	if m == 0 {
		return 0
	}
	return float64(shared) / float64(m)
}

func suffixes(tokens []string) [][]string {
	out := [][]string{tokens}
	for n := 1; n <= 2 && n < len(tokens); n++ {
		if containsIdentity(tokens[:n]) {
			break
		}
		out = append(out, tokens[n:])
	}
	return out
}

func longerKey(cur, next []string) []string {
	if len(next) == 0 {
		return cur
	}
	if cur == nil || len(next) > len(cur) {
		return next
	}
	if len(next) == len(cur) && strings.Join(next, " ") < strings.Join(cur, " ") {
		return next
	}
	return cur
}

func preferPlural(a, b []string) []string {
	if len(strings.Join(a, " ")) >= len(strings.Join(b, " ")) {
		return a
	}
	return b
}

func tokensEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if stemToken(a[i]) != stemToken(b[i]) {
			return false
		}
	}
	return true
}

func similarityTokens(name string) ([]string, string) {
	tokens := stripStoreBrands(normalizeTokens(name))
	tokens, caution := stripDifferentiators(tokens)
	tokens = dropSizeAndPackage(tokens)
	rest, extra := stripPhrases(tokens, extraVarietyPhrases)
	if caution == "" {
		caution = extra
	}
	return rest, caution
}

func stripPhrases(tokens []string, phrases []string) ([]string, string) {
	ordered := append([]string(nil), phrases...)
	for i := 0; i < len(ordered); i++ {
		for j := i + 1; j < len(ordered); j++ {
			if len(strings.Fields(ordered[j])) > len(strings.Fields(ordered[i])) {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
	}
	rest := append([]string(nil), tokens...)
	var first string
	for _, label := range ordered {
		words := strings.Fields(label)
		for {
			next, ok := removeWords(rest, words)
			if !ok {
				break
			}
			rest = next
			if first == "" {
				first = label
			}
		}
	}
	if first == "" {
		return tokens, ""
	}
	return rest, cautionSentence(first)
}

func varietyCaution(tokens, key []string, existing string) string {
	if existing != "" || !hasSuffixTokens(tokens, key) {
		return existing
	}
	dropped := tokens[:len(tokens)-len(key)]
	for _, tok := range dropped {
		if varietyWord(tok) {
			return cautionSentence(tok)
		}
	}
	return ""
}

func varietyWord(tok string) bool {
	_, ok := varietyWords[stemToken(tok)]
	return ok
}

func genericFood(tok string) bool {
	_, ok := genericFoodWords[stemToken(tok)]
	return ok
}

func stemToken(tok string) string {
	switch {
	case len(tok) > 4 && strings.HasSuffix(tok, "ies"):
		return strings.TrimSuffix(tok, "ies") + "y"
	case len(tok) > 4 && strings.HasSuffix(tok, "oes"):
		return strings.TrimSuffix(tok, "oes") + "o"
	case len(tok) > 4 && (strings.HasSuffix(tok, "ches") || strings.HasSuffix(tok, "shes") || strings.HasSuffix(tok, "xes") || strings.HasSuffix(tok, "zes") || strings.HasSuffix(tok, "ses")):
		return strings.TrimSuffix(tok, "es")
	case len(tok) > 3 && strings.HasSuffix(tok, "s") && !strings.HasSuffix(tok, "ss") && !strings.HasSuffix(tok, "us") && !strings.HasSuffix(tok, "is"):
		return strings.TrimSuffix(tok, "s")
	default:
		return tok
	}
}

func normCategory(s string) string {
	return strings.Join(normalizeTokens(s), " ")
}

func sameCategory(a, b profile) bool {
	return a.category != "" && a.category == b.category
}

func categoriesDisagree(a, b string) bool {
	return a != "" && b != "" && a != b
}
