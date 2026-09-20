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
	"runtime"

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
	s.registerSystemFont(family, query)
}

// registerSystemFont is RegisterSystemFont's implementation, returning how
// many subfonts were registered - RegisterPreferredFont uses this to tell
// whether a candidate name actually resolved to anything usable, without
// exposing that as part of RegisterSystemFont's own public, no-return
// signature.
func (s *SystemFontFaceSelector) registerSystemFont(family whynot.FontFamily, query string) int {
	match := s.finder.Match(query)
	if match == nil || match.Filename == "" {
		log.Printf(logPrefix+"no installed font found for %q", query)
		return 0
	}
	data, err := os.ReadFile(match.Filename)
	if err != nil {
		log.Printf(logPrefix+"reading %s (matched %q): %v", match.Filename, query, err)
		return 0
	}
	registered, err := s.AddFontCollection(family, data, match.Family)
	if err != nil {
		log.Printf(logPrefix+"parsing %s (matched %q): %v", match.Filename, query, err)
		return 0
	}
	if registered == 0 {
		log.Printf(logPrefix+"matched %q to %s but registered no usable subfont from it", query, match.Filename)
		return 0
	}
	log.Printf(logPrefix+"registered %d subfont(s) from %s for %q", registered, match.Filename, query)
	return registered
}

// RegisterPreferredFont registers whatever this platform's most likely
// "system UI" font is for family, without requiring the caller to name a
// specific installed font themselves. There's no portable, dependency-free
// way to ask the OS directly for its actual configured UI font (that means
// cgo on macOS, a registry/API call on Windows, or shelling out to
// fontconfig on Linux - real, separate platform-specific work this package
// deliberately doesn't take on), so this tries a short, curated,
// GOOS-aware list of common candidate names instead (see
// preferredFontCandidates) via RegisterSystemFont, in order, stopping at
// the first one that actually registers something. If none of the
// candidates are installed (or family has no curated candidates at all -
// see preferredFontCandidates), it logs that and leaves every slot to the
// fallback FaceSelector, the same as an ordinary unmatched
// RegisterSystemFont query would.
func (s *SystemFontFaceSelector) RegisterPreferredFont(family whynot.FontFamily) {
	candidates := preferredFontCandidates(runtime.GOOS, family)
	for _, query := range candidates {
		if s.registerSystemFont(family, query) > 0 {
			return
		}
	}
	log.Printf(logPrefix+"none of the preferred-font candidates for %v resolved on GOOS=%s (tried %v)", family, runtime.GOOS, candidates)
}
