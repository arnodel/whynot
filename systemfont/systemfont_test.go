package systemfont

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adrg/sysfont"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/arnodel/whynot"
)

type fakeFinder struct{ result *sysfont.Font }

func (f fakeFinder) Match(string) *sysfont.Font { return f.result }

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
