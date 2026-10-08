package group

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// memberBrand is a brand already written in the product name.
// Words in front of the suggestion title are that brand. A listed store
// brand that sits after the food name is still recognized. Anything else
// stays blank so the page does not invent a brand.
func memberBrand(name, title string) string {
	if brand := brandAheadOfTitle(name, title); brand != "" {
		return brand
	}
	return matchedStoreBrand(name)
}

// memberVariety is a differentiator the look-alike matcher already treats as
// a reason two products might not be the same, such as crunchy or organic.
func memberVariety(name string) string {
	labels, _ := takeDifferentiators(normalizeTokens(name))
	if len(labels) == 0 {
		return ""
	}
	shown := make([]string, len(labels))
	for i, label := range labels {
		shown[i] = displayPhrase(label)
	}
	return strings.Join(shown, ", ")
}

func cleanBrand(brand string) string {
	brand = strings.Map(func(r rune) rune {
		switch r {
		case '®', '™', '©':
			return -1
		}
		return r
	}, brand)
	return strings.Trim(strings.TrimSpace(brand), " \t,./-()[]")
}

func displayPhrase(label string) string {
	r := []rune(strings.TrimSpace(label))
	if len(r) == 0 {
		return ""
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func brandAheadOfTitle(name, title string) string {
	titleTokens := normalizeTokens(title)
	if len(titleTokens) == 0 {
		return ""
	}
	base, _ := baseLookAlike(name)
	if !hasSuffixTokens(strings.Fields(base), titleTokens) {
		return ""
	}
	spans := tokenSpans(name)
	idx := lastTokenPhrase(spans, titleTokens)
	if idx <= 0 {
		return ""
	}
	var lead []tokenSpan
	for _, span := range spans[:idx] {
		if isSizeOrPackage(span.norm) {
			break
		}
		lead = append(lead, span)
	}
	if len(lead) == 0 {
		return ""
	}
	brand := cleanBrand(name[lead[0].start:lead[len(lead)-1].end])
	if brand == "" || onlyDifferentiators(brand) {
		return ""
	}
	return brand
}

func matchedStoreBrand(name string) string {
	tokens := normalizeTokens(name)
	for _, phrase := range storeBrandPhrases {
		if _, ok := removeBrandPhrase(tokens, phrase); !ok {
			continue
		}
		joined := strings.Join(phrase, " ")
		for _, brand := range storeBrandNames {
			if strings.Join(normalizeTokens(brand), " ") == joined {
				return brand
			}
		}
	}
	return ""
}

func onlyDifferentiators(text string) bool {
	tokens := dropSizeAndPackage(normalizeTokens(text))
	if len(tokens) == 0 {
		return true
	}
	labels, rest := takeDifferentiators(tokens)
	return len(labels) > 0 && len(rest) == 0
}

func hasSuffixTokens(tokens, phrase []string) bool {
	if len(phrase) == 0 || len(phrase) > len(tokens) {
		return false
	}
	start := len(tokens) - len(phrase)
	for i := range phrase {
		if tokens[start+i] != phrase[i] {
			return false
		}
	}
	return true
}

func isSizeOrPackage(tok string) bool {
	return hasDigit(tok) || hasWord(unitWords, tok) || hasWord(packageWords, tok) || tok == "x"
}

type tokenSpan struct {
	norm       string
	start, end int
}

func tokenSpans(name string) []tokenSpan {
	var spans []tokenSpan
	for i := 0; i < len(name); {
		r, size := utf8.DecodeRuneInString(name[i:])
		if r == '&' {
			spans = append(spans, tokenSpan{norm: "and", start: i, end: i + size})
			i += size
			continue
		}
		if isTokenBreak(r) {
			i += size
			continue
		}
		start := i
		var b strings.Builder
		for i < len(name) {
			r, size = utf8.DecodeRuneInString(name[i:])
			if r == '&' || isTokenBreak(r) {
				break
			}
			switch r {
			case '®', '™', '©', '\'', '’', '"', '“', '”':
				i += size
				continue
			}
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '%' {
				b.WriteRune(unicode.ToLower(r))
			}
			i += size
		}
		if b.Len() > 0 {
			spans = append(spans, tokenSpan{norm: b.String(), start: start, end: i})
		}
	}
	return spans
}

func isTokenBreak(r rune) bool {
	if unicode.IsSpace(r) {
		return true
	}
	switch r {
	case '-', '/', ',', '.', '(', ')', ':', ';', '!', '?', '+':
		return true
	}
	return false
}

func lastTokenPhrase(spans []tokenSpan, phrase []string) int {
	found := -1
	if len(phrase) == 0 {
		return found
	}
	for i := 0; i+len(phrase) <= len(spans); i++ {
		match := true
		for j := range phrase {
			if spans[i+j].norm != phrase[j] {
				match = false
				break
			}
		}
		if match {
			found = i
		}
	}
	return found
}
