package systemfont

import (
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/adrg/sysfont"

	"github.com/arnodel/whynot/fonts"
)

const logPrefix = "whynot/systemfont: "

// finder is satisfied by *sysfont.Finder. A local interface so tests can
// substitute a fake that doesn't depend on what's actually installed on
// the machine running the test.
type finder interface {
	Match(query string) *sysfont.Font
}

// Selector is a fonts.FaceSelector that resolves fonts by name from
// whatever's installed on the host machine. It wraps a
// fonts.CustomSelector: RegisterSystemFont locates an installed
// font file for a family and registers every subfont in it that
// AddFontCollection can classify and match - anything it can't find or
// classify is left for the underlying CustomSelector's fallback (a
// fonts.GoSelector by default) to serve instead.
//
// New scans the host's real font directories once,
// synchronously - construct it during setup, not per frame or document.
type Selector struct {
	*fonts.CustomSelector
	finder finder
}

var _ fonts.FaceSelector = (*Selector)(nil)

// New returns a Selector with nothing
// registered yet - every SelectFace call delegates to its fallback
// FaceSelector until RegisterSystemFont is called. opts configures the
// underlying CustomSelector exactly as they would fonts.NewCustomSelector,
// e.g. fonts.WithFallback.
func New(opts ...fonts.CustomOption) *Selector {
	return &Selector{
		CustomSelector: fonts.NewCustomSelector(opts...),
		finder:         sysfont.NewFinder(nil),
	}
}

// ErrNotInstalled is the error, as reported by [errors.Is], when no
// installed font matches a query.
var ErrNotInstalled = errors.New("no installed font matches")

// RegisterSystemFont locates an installed font matching query (e.g.
// "Arial", "Helvetica Neue") and registers every subfont in its file that
// AddFontCollection can both classify into a (weight, style) slot and
// confirm belongs to the matched family. Slots it doesn't fill are served
// by the fallback FaceSelector.
//
// It returns an error if nothing was registered: no installed font
// matches query ([ErrNotInstalled]), or its file can't be read or parsed,
// or none of its subfonts is usable. On success, it logs which font it
// picked, since a query can resolve to a differently named font.
func (s *Selector) RegisterSystemFont(family fonts.Family, query string) error {
	match := s.finder.Match(query)
	if match == nil || match.Filename == "" {
		return fmt.Errorf("systemfont: %v: %w %q", family, ErrNotInstalled, query)
	}
	data, err := os.ReadFile(match.Filename)
	if err != nil {
		return fmt.Errorf("systemfont: %v: reading %s (matched %q as %q): %w", family, match.Filename, query, match.Family, err)
	}
	registered, err := s.AddFontCollection(family, data, match.Family)
	if err != nil {
		return fmt.Errorf("systemfont: %v: parsing %s (matched %q as %q): %w", family, match.Filename, query, match.Family, err)
	}
	if registered == 0 {
		return fmt.Errorf("systemfont: %v: matched %q to %q (%s), but none of its subfonts is usable", family, query, match.Family, match.Filename)
	}
	// match.Family is what sysfont resolved query to, which can differ
	// from it (fuzzy matching, or sysfont's own fallbacks), so both are
	// logged.
	log.Printf(logPrefix+"%v: registered %d subfont(s) from %q (%s) for %q", family, registered, match.Family, match.Filename, query)
	return nil
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
// the first one that actually registers something. If none does (or
// family has no curated candidates at all - see preferredFontCandidates),
// it returns an error, joining each candidate's, and every slot is left
// to the fallback FaceSelector, as with a failed RegisterSystemFont.
func (s *Selector) RegisterPreferredFont(family fonts.Family) error {
	candidates := preferredFontCandidates(runtime.GOOS, family)
	errs := []error{fmt.Errorf("systemfont: %v: none of the preferred fonts for %s is usable (tried %q)", family, runtime.GOOS, candidates)}
	for _, query := range candidates {
		err := s.RegisterSystemFont(family, query)
		if err == nil {
			return nil
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
