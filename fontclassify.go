package whynot

import (
	"strings"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
)

// classifySubfamily classifies a font's subfamily/style name (e.g. "Bold
// Italic", "BoldOblique", "Medium", "") into a (weight, style) slot,
// conservatively: every space/hyphen/camelCase-separated token in name
// must be recognized, or classification is refused (ok=false) rather than
// guessed. This deliberately does NOT try to place tokens like "Light",
// "SemiBold", "ExtraBold", "Black", or "Condensed" into the nearest of
// whynot's three weight buckets - doing so would be a silent,
// iteration-order-dependent guess (e.g. "SemiBold" landing on "Normal" or
// "Bold" depending on what else got registered into that slot first).
// Skipping leaves that slot for the fallback FaceSelector instead -
// graceful degradation over silently-wrong behavior, the same principle
// ARCHITECTURE.md documents for unsupported Markdown constructs.
func classifySubfamily(name string) (weight font.Weight, style font.Style, ok bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return font.WeightNormal, font.StyleNormal, true
	}
	var sawBold, sawMedium, sawItalic bool
	for _, tok := range splitStyleTokens(name) {
		switch tok {
		case "italic", "oblique":
			sawItalic = true
		case "regular", "roman", "book", "normal", "text", "plain":
			// weight-neutral; nothing to record beyond "recognized"
		case "medium":
			sawMedium = true
		case "bold":
			sawBold = true
		default:
			return 0, 0, false
		}
	}
	style = font.StyleNormal
	if sawItalic {
		style = font.StyleItalic
	}
	switch {
	case sawBold:
		weight = font.WeightBold
	case sawMedium:
		weight = font.WeightMedium
	default:
		weight = font.WeightNormal
	}
	return weight, style, true
}

// splitStyleTokens splits a subfamily string like "BoldItalic",
// "Bold Italic", or "Bold-Italic" into normalized lowercase tokens
// ("bold", "italic"): space/hyphen/underscore/slash are separators, and a
// lowercase-to-uppercase boundary is also a separator, so "BoldItalic"
// splits the same way "Bold Italic" would.
func splitStyleTokens(name string) []string {
	var tokens []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			tokens = append(tokens, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	runes := []rune(name)
	for i, r := range runes {
		switch {
		case r == ' ' || r == '-' || r == '_' || r == '/':
			flush()
		case i > 0 && unicode.IsUpper(r) && unicode.IsLower(runes[i-1]):
			flush()
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return tokens
}

// subfontName returns the first non-empty name f has for any of ids, tried
// in order - e.g. (NameIDTypographicFamily, NameIDFamily) to prefer the
// typographic name when present.
func subfontName(f *sfnt.Font, buf *sfnt.Buffer, ids ...sfnt.NameID) string {
	for _, id := range ids {
		if name, err := f.Name(buf, id); err == nil && name != "" {
			return name
		}
	}
	return ""
}

// subfontBelongsToFamily reports whether f's own embedded family name
// reasonably matches want - a defensive check against a collection file
// bundling unrelated families, so a mismatched subfont sharing that file
// doesn't get registered. want == "" always matches (nothing to compare
// against). Loosely (case/space/punctuation-insensitive substring, either
// direction) rather than exact, since a font's own embedded family string
// commonly differs cosmetically from the name a caller matched it by
// (e.g. embedded "Arial" vs resolved "Arial MT").
func subfontBelongsToFamily(f *sfnt.Font, buf *sfnt.Buffer, want string) bool {
	if want == "" {
		return true
	}
	got := subfontName(f, buf, sfnt.NameIDTypographicFamily, sfnt.NameIDFamily)
	if got == "" {
		return false
	}
	a, b := normalizeFontName(got), normalizeFontName(want)
	return a == b || strings.Contains(a, b) || strings.Contains(b, a)
}

// normalizeFontName lowercases name and strips spaces/hyphens/underscores,
// so cosmetically different renderings of the same family name compare
// equal ("Arial MT" / "arial-mt" / "ArialMT" all normalize to "arialmt").
func normalizeFontName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r == ' ' || r == '-' || r == '_' {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
