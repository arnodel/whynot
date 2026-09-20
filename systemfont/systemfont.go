// Package systemfont resolves fonts by name from whatever's installed on
// the host machine (e.g. "Arial", "Helvetica Neue"), on top of
// whynot.CustomFontFaceSelector.AddFontCollection. It's split out from the
// core whynot package to keep its github.com/adrg/sysfont dependency (and
// that package's own dependencies) out of the core library's dependency
// graph - the same reasoning ebitenrenderer keeps the ebiten dependency
// out of the core library. This package does no font classification of
// its own - locating the right file is its only job; reading what's
// inside it is whynot.CustomFontFaceSelector.AddFontCollection's.
package systemfont

import (
	"log"
	"os"

	"github.com/adrg/sysfont"

	"github.com/arnodel/whynot"
)

const logPrefix = "whynot/systemfont: "

// finder is satisfied by *sysfont.Finder. A local interface so tests can
// substitute a fake that doesn't depend on what's actually installed on
// the machine running the test.
type finder interface {
	Match(query string) *sysfont.Font
}

// SystemFontFaceSelector is a whynot.FaceSelector that resolves fonts by
// name from whatever's installed on the host machine. It wraps a
// whynot.CustomFontFaceSelector: RegisterSystemFont locates an installed
// font file for a family and registers every subfont in it that
// AddFontCollection can classify and match - anything it can't find or
// classify is left for the underlying CustomFontFaceSelector's fallback
// (whynot.NewGoFontFaceSelector by default) to serve instead.
//
// NewSystemFontFaceSelector scans the host's real font directories once,
// synchronously - construct it during setup, not per frame or document.
type SystemFontFaceSelector struct {
	*whynot.CustomFontFaceSelector
	finder finder
}

var _ whynot.FaceSelector = (*SystemFontFaceSelector)(nil)

// NewSystemFontFaceSelector returns a SystemFontFaceSelector with nothing
// registered yet - every SelectFace call delegates to its fallback
// FaceSelector until RegisterSystemFont is called. opts configures the
// underlying CustomFontFaceSelector exactly as they would
// whynot.NewCustomFontFaceSelector, e.g. whynot.WithFallback.
func NewSystemFontFaceSelector(dpi float64, opts ...whynot.CustomFontFaceSelectorOption) *SystemFontFaceSelector {
	return &SystemFontFaceSelector{
		CustomFontFaceSelector: whynot.NewCustomFontFaceSelector(dpi, opts...),
		finder:                 sysfont.NewFinder(nil),
	}
}

// RegisterSystemFont locates an installed font matching query (e.g.
// "Arial", "Helvetica Neue") and registers every subfont in its file that
// AddFontCollection can both classify into a (weight, style) slot and
// confirm belongs to the matched family. A bad or unmatched query, or one
// AddFontCollection can't usefully register anything from, is logged and
// otherwise a no-op - those slots are served by the fallback FaceSelector
// instead, which is why this has no error return.
func (s *SystemFontFaceSelector) RegisterSystemFont(family whynot.FontFamily, query string) {
	match := s.finder.Match(query)
	if match == nil || match.Filename == "" {
		log.Printf(logPrefix+"no installed font found for %q", query)
		return
	}
	data, err := os.ReadFile(match.Filename)
	if err != nil {
		log.Printf(logPrefix+"reading %s (matched %q): %v", match.Filename, query, err)
		return
	}
	registered, err := s.AddFontCollection(family, data, match.Family)
	if err != nil {
		log.Printf(logPrefix+"parsing %s (matched %q): %v", match.Filename, query, err)
		return
	}
	if registered == 0 {
		log.Printf(logPrefix+"matched %q to %s but registered no usable subfont from it", query, match.Filename)
		return
	}
	log.Printf(logPrefix+"registered %d subfont(s) from %s for %q", registered, match.Filename, query)
}
