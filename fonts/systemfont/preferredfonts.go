package systemfont

import "github.com/arnodel/whynot/fonts"

// preferredFontCandidates returns, most-preferred first, the installed
// font names RegisterPreferredFont tries for (goos, family) - a small,
// curated list of names common enough on each platform to have a good
// chance of being installed and shipping a real Bold/Italic/Bold Italic
// set (not just a Regular-only substitute), not an exhaustive or
// authoritative list of that platform's actual configured UI font.
// Platforms with no curated list (anything not explicitly keyed below)
// fall back to the same list as "linux", which leans on font families
// common across open-source Unix desktop font sets (DejaVu, Liberation,
// Noto) - a reasonable generic guess for an unlisted GOOS.
//
// SmallCaps has no curated candidates on any platform: there's no common,
// reliably-installed small-caps display font to guess at, so
// RegisterPreferredFont(SmallCaps, ...) always falls straight through to
// the fallback FaceSelector.
func preferredFontCandidates(goos string, family fonts.Family) []string {
	byFamily, ok := preferredFonts[goos]
	if !ok {
		byFamily = preferredFonts["linux"]
	}
	return byFamily[family]
}

var preferredFonts = map[string]map[fonts.Family][]string{
	"darwin": {
		fonts.Proportional: {"Helvetica Neue", "Helvetica", "Arial"},
		fonts.Monospace:    {"Menlo", "Courier"},
	},
	"windows": {
		fonts.Proportional: {"Segoe UI", "Arial"},
		fonts.Monospace:    {"Consolas", "Courier New"},
	},
	"linux": {
		fonts.Proportional: {"DejaVu Sans", "Liberation Sans", "Noto Sans", "FreeSans"},
		fonts.Monospace:    {"DejaVu Sans Mono", "Liberation Mono", "Noto Sans Mono", "FreeMono"},
	},
}
