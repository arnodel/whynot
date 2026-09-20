package systemfont

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/adrg/sysfont"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/arnodel/whynot"
)

type fakeFinder struct{ result *sysfont.Font }

func (f fakeFinder) Match(string) *sysfont.Font { return f.result }

// fakeFinderByQuery lets a test control exactly which query strings
// resolve to something, so RegisterPreferredFont's "try each candidate in
// order" loop can be tested directly - a key with no entry (the zero
// value, nil) behaves like a candidate that isn't installed.
type fakeFinderByQuery map[string]*sysfont.Font

func (f fakeFinderByQuery) Match(query string) *sysfont.Font { return f[query] }

// spyFaceSelector records the arguments it was called with, so tests can
// assert whether the fallback was actually invoked.
type spyFaceSelector struct {
	selectFaceCalls []whynot.TextStyle
}

func (s *spyFaceSelector) SelectFace(style whynot.TextStyle) (font.Face, error) {
	s.selectFaceCalls = append(s.selectFaceCalls, style)
	return nil, nil
}

func (s *spyFaceSelector) SetDPI(float64) {}

func writeGoregularCopy(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "goregular.ttf")
	if err := os.WriteFile(path, goregular.TTF, 0o644); err != nil {
		t.Fatalf("writing test font file: %v", err)
	}
	return path
}

func TestRegisterSystemFontEndToEnd(t *testing.T) {
	path := writeGoregularCopy(t)
	spy := &spyFaceSelector{}
	s := NewSystemFontFaceSelector(72, whynot.WithFallback(spy))
	s.finder = fakeFinder{result: &sysfont.Font{Family: "Go", Filename: path}}

	s.RegisterSystemFont(whynot.Proportional, "Go")

	if _, err := s.SelectFace(whynot.TextStyle{Size: 16, Family: whynot.Proportional}); err != nil {
		t.Errorf("SelectFace: %v", err)
	}
	if len(spy.selectFaceCalls) != 0 {
		t.Errorf("fallback.SelectFace called %d times, want 0 - the slot should have been filled by RegisterSystemFont", len(spy.selectFaceCalls))
	}
}

func TestRegisterSystemFontNoMatch(t *testing.T) {
	spy := &spyFaceSelector{}
	s := NewSystemFontFaceSelector(72, whynot.WithFallback(spy))
	s.finder = fakeFinder{result: nil}

	s.RegisterSystemFont(whynot.Proportional, "Nonexistent")

	if _, err := s.SelectFace(whynot.TextStyle{Size: 16, Family: whynot.Proportional}); err != nil {
		t.Errorf("SelectFace: %v", err)
	}
	if len(spy.selectFaceCalls) != 1 {
		t.Errorf("fallback.SelectFace called %d times, want 1", len(spy.selectFaceCalls))
	}
}

func TestRegisterSystemFontMissingFile(t *testing.T) {
	spy := &spyFaceSelector{}
	s := NewSystemFontFaceSelector(72, whynot.WithFallback(spy))
	s.finder = fakeFinder{result: &sysfont.Font{Family: "Go", Filename: "/nonexistent/path.ttf"}}

	s.RegisterSystemFont(whynot.Proportional, "Go")

	if _, err := s.SelectFace(whynot.TextStyle{Size: 16, Family: whynot.Proportional}); err != nil {
		t.Errorf("SelectFace: %v", err)
	}
	if len(spy.selectFaceCalls) != 1 {
		t.Errorf("fallback.SelectFace called %d times, want 1", len(spy.selectFaceCalls))
	}
}

// TestRegisterPreferredFontTriesEachCandidate proves the "try candidates
// in order until one works" loop actually advances past a failing
// candidate, using this platform's own real candidate list (via
// preferredFontCandidates(runtime.GOOS, ...)) so the test stays valid
// wherever it runs - only the LAST candidate is wired to resolve.
func TestRegisterPreferredFontTriesEachCandidate(t *testing.T) {
	candidates := preferredFontCandidates(runtime.GOOS, whynot.Proportional)
	if len(candidates) < 2 {
		t.Skipf("need at least 2 proportional candidates for GOOS=%s to exercise fallthrough, have %v", runtime.GOOS, candidates)
	}
	path := writeGoregularCopy(t)
	spy := &spyFaceSelector{}
	s := NewSystemFontFaceSelector(72, whynot.WithFallback(spy))
	s.finder = fakeFinderByQuery{
		candidates[len(candidates)-1]: {Family: "Go", Filename: path},
	}

	s.RegisterPreferredFont(whynot.Proportional)

	if _, err := s.SelectFace(whynot.TextStyle{Size: 16, Family: whynot.Proportional}); err != nil {
		t.Errorf("SelectFace: %v", err)
	}
	if len(spy.selectFaceCalls) != 0 {
		t.Errorf("fallback.SelectFace called %d times, want 0 - the last candidate should have filled the slot", len(spy.selectFaceCalls))
	}
}

func TestRegisterPreferredFontNoneResolve(t *testing.T) {
	spy := &spyFaceSelector{}
	s := NewSystemFontFaceSelector(72, whynot.WithFallback(spy))
	s.finder = fakeFinderByQuery{} // matches nothing, for any candidate

	s.RegisterPreferredFont(whynot.Proportional)

	if _, err := s.SelectFace(whynot.TextStyle{Size: 16, Family: whynot.Proportional}); err != nil {
		t.Errorf("SelectFace: %v", err)
	}
	if len(spy.selectFaceCalls) != 1 {
		t.Errorf("fallback.SelectFace called %d times, want 1", len(spy.selectFaceCalls))
	}
}

// TestRegisterPreferredFontSmallCapsHasNoCandidates checks that an empty
// candidate list (SmallCaps, on every platform) is a clean no-op straight
// to the fallback, never even consulting the finder.
func TestRegisterPreferredFontSmallCapsHasNoCandidates(t *testing.T) {
	spy := &spyFaceSelector{}
	s := NewSystemFontFaceSelector(72, whynot.WithFallback(spy))
	s.finder = fakeFinderByQuery{}

	s.RegisterPreferredFont(whynot.SmallCaps)

	if _, err := s.SelectFace(whynot.TextStyle{Size: 16, Family: whynot.SmallCaps}); err != nil {
		t.Errorf("SelectFace: %v", err)
	}
	if len(spy.selectFaceCalls) != 1 {
		t.Errorf("fallback.SelectFace called %d times, want 1 (no curated SmallCaps candidates to try)", len(spy.selectFaceCalls))
	}
}
