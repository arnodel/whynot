package whynot

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
)

// spyFaceSelector records the arguments it was called with, so tests can
// assert delegation to a fallback passes through unnormalized/unchanged.
type spyFaceSelector struct {
	selectFaceCalls []TextStyle
	setDPICalls     []float64
	face            font.Face
}

func (s *spyFaceSelector) SelectFace(style TextStyle) (font.Face, error) {
	s.selectFaceCalls = append(s.selectFaceCalls, style)
	return s.face, nil
}

func (s *spyFaceSelector) SetDPI(dpi float64) {
	s.setDPICalls = append(s.setDPICalls, dpi)
}

func TestCustomFontFaceSelectorSelectsRegisteredFont(t *testing.T) {
	s := NewCustomFontFaceSelector(72, WithFallback(nil))
	if err := s.AddFont(Proportional, font.WeightNormal, font.StyleNormal, goregular.TTF, 0); err != nil {
		t.Fatalf("AddFont: %v", err)
	}
	face, err := s.SelectFace(TextStyle{Size: 16, Family: Proportional})
	if err != nil {
		t.Fatalf("SelectFace: %v", err)
	}
	if face == nil {
		t.Fatal("SelectFace returned a nil face")
	}
}

func TestCustomFontFaceSelectorCachesFace(t *testing.T) {
	s := NewCustomFontFaceSelector(72, WithFallback(nil))
	if err := s.AddFont(Proportional, font.WeightNormal, font.StyleNormal, goregular.TTF, 0); err != nil {
		t.Fatalf("AddFont: %v", err)
	}
	style := TextStyle{Size: 16, Family: Proportional}
	face1, err := s.SelectFace(style)
	if err != nil {
		t.Fatalf("SelectFace: %v", err)
	}
	face2, err := s.SelectFace(style)
	if err != nil {
		t.Fatalf("SelectFace: %v", err)
	}
	if face1 != face2 {
		t.Error("SelectFace with the same TextStyle returned different faces, expected a cached one")
	}
	face3, err := s.SelectFace(TextStyle{Size: 24, Family: Proportional})
	if err != nil {
		t.Fatalf("SelectFace: %v", err)
	}
	if face1 == face3 {
		t.Error("SelectFace with a different Size returned the same cached face")
	}
}

func TestCustomFontFaceSelectorNormalizationMatchesBuiltin(t *testing.T) {
	s := NewCustomFontFaceSelector(72, WithFallback(nil))
	// Register at an "off" weight/style that should still bucket into the
	// Bold/Italic slot, mirroring GoFontFaceSelector's own bucketing.
	if err := s.AddFont(Proportional, font.WeightSemiBold, font.StyleOblique, gobold.TTF, 0); err != nil {
		t.Fatalf("AddFont: %v", err)
	}
	face, err := s.SelectFace(TextStyle{Size: 16, Weight: font.WeightBold, Style: font.StyleItalic, Family: Proportional})
	if err != nil {
		t.Fatalf("SelectFace: %v", err)
	}
	if face == nil {
		t.Fatal("SelectFace returned a nil face; expected the registered Bold/Italic font to resolve")
	}
}

func TestCustomFontFaceSelectorFallsBackForUnfilledSlot(t *testing.T) {
	goFont, err := NewGoFontFaceSelector(72).SelectFace(TextStyle{Size: 16, Family: Proportional})
	if err != nil {
		t.Fatalf("building a face for the fallback spy: %v", err)
	}
	spy := &spyFaceSelector{face: goFont}
	s := NewCustomFontFaceSelector(72, WithFallback(spy))
	style := TextStyle{Size: 16, Family: Monospace}
	face, err := s.SelectFace(style)
	if err != nil {
		t.Fatalf("SelectFace: %v", err)
	}
	if face != spy.face {
		t.Error("SelectFace did not return the fallback's face for an unfilled slot")
	}
	if len(spy.selectFaceCalls) != 1 || spy.selectFaceCalls[0] != style {
		t.Errorf("fallback.SelectFace called with %v, want exactly one call with %v", spy.selectFaceCalls, style)
	}
}

func TestCustomFontFaceSelectorDefaultFallbackIsGoFonts(t *testing.T) {
	s := NewCustomFontFaceSelector(72)
	if _, err := s.SelectFace(TextStyle{Size: 16, Family: Monospace}); err != nil {
		t.Errorf("SelectFace with no options and an unfilled slot: %v, want it served by the default GoFontFaceSelector fallback", err)
	}
}

func TestCustomFontFaceSelectorNilFallback(t *testing.T) {
	s := NewCustomFontFaceSelector(72, WithFallback(nil))
	if _, err := s.SelectFace(TextStyle{Size: 16, Family: Monospace}); err == nil {
		t.Error("SelectFace with a nil fallback and an unfilled slot returned no error, want one")
	}
}

func TestCustomFontFaceSelectorAddFontInvalidBytes(t *testing.T) {
	s := NewCustomFontFaceSelector(72)
	if err := s.AddFont(Proportional, font.WeightNormal, font.StyleNormal, []byte("not a font"), 0); err == nil {
		t.Fatal("AddFont with invalid bytes returned no error, want one")
	}
	// The failed registration should leave the slot unfilled, still falling
	// back cleanly.
	if _, err := s.SelectFace(TextStyle{Size: 16, Family: Proportional}); err != nil {
		t.Errorf("SelectFace after a failed AddFont: %v, want a clean fallback", err)
	}
}

func TestCustomFontFaceSelectorAddFontIndexOutOfRange(t *testing.T) {
	s := NewCustomFontFaceSelector(72, WithFallback(nil))
	err := s.AddFont(Proportional, font.WeightNormal, font.StyleNormal, goregular.TTF, 3)
	if err == nil {
		t.Fatal("AddFont with an out-of-range index returned no error, want one")
	}
	if !errors.Is(err, sfnt.ErrNotFound) {
		t.Errorf("AddFont out-of-range error = %v, want it to wrap sfnt.ErrNotFound", err)
	}
	if _, err := s.SelectFace(TextStyle{Size: 16, Family: Proportional}); err == nil {
		t.Error("SelectFace after a failed AddFont with a nil fallback returned no error, want one")
	}
}

func TestCustomFontFaceSelectorAddFontFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "font.ttf")
	if err := os.WriteFile(path, goregular.TTF, 0o644); err != nil {
		t.Fatalf("writing test font file: %v", err)
	}
	s := NewCustomFontFaceSelector(72, WithFallback(nil))
	if err := s.AddFontFile(Proportional, font.WeightNormal, font.StyleNormal, path, 0); err != nil {
		t.Fatalf("AddFontFile: %v", err)
	}
	if _, err := s.SelectFace(TextStyle{Size: 16, Family: Proportional}); err != nil {
		t.Errorf("SelectFace after AddFontFile: %v", err)
	}

	if err := s.AddFontFile(Proportional, font.WeightNormal, font.StyleNormal, filepath.Join(dir, "missing.ttf"), 0); err == nil {
		t.Error("AddFontFile with a nonexistent path returned no error, want one")
	}
}

func TestCustomFontFaceSelectorAddFontCollectionRegistersMatchingFamily(t *testing.T) {
	s := NewCustomFontFaceSelector(72, WithFallback(nil))
	registered, err := s.AddFontCollection(Proportional, goregular.TTF, "Go")
	if err != nil {
		t.Fatalf("AddFontCollection: %v", err)
	}
	if registered != 1 {
		t.Fatalf("registered = %d, want 1", registered)
	}
	if _, err := s.SelectFace(TextStyle{Size: 16, Family: Proportional}); err != nil {
		t.Errorf("SelectFace after AddFontCollection: %v", err)
	}
}

func TestCustomFontFaceSelectorAddFontCollectionEmptyMatchFamily(t *testing.T) {
	s := NewCustomFontFaceSelector(72, WithFallback(nil))
	registered, err := s.AddFontCollection(Proportional, goregular.TTF, "")
	if err != nil {
		t.Fatalf("AddFontCollection: %v", err)
	}
	if registered != 1 {
		t.Fatalf("registered = %d, want 1 (empty matchFamily should register regardless of family)", registered)
	}
}

func TestCustomFontFaceSelectorAddFontCollectionFamilyMismatch(t *testing.T) {
	s := NewCustomFontFaceSelector(72, WithFallback(nil))
	registered, err := s.AddFontCollection(Proportional, goregular.TTF, "Arial")
	if err != nil {
		t.Fatalf("AddFontCollection: %v", err)
	}
	if registered != 0 {
		t.Fatalf("registered = %d, want 0 (real family is \"Go\", not \"Arial\")", registered)
	}
}

func TestCustomFontFaceSelectorAddFontCollectionInvalidBytes(t *testing.T) {
	s := NewCustomFontFaceSelector(72, WithFallback(nil))
	if _, err := s.AddFontCollection(Proportional, []byte("not a font"), ""); err == nil {
		t.Error("AddFontCollection with invalid bytes returned no error, want one")
	}
}

func TestCustomFontFaceSelectorAddFontCollectionFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "font.ttf")
	if err := os.WriteFile(path, goregular.TTF, 0o644); err != nil {
		t.Fatalf("writing test font file: %v", err)
	}
	s := NewCustomFontFaceSelector(72, WithFallback(nil))
	registered, err := s.AddFontCollectionFile(Proportional, path, "Go")
	if err != nil {
		t.Fatalf("AddFontCollectionFile: %v", err)
	}
	if registered != 1 {
		t.Fatalf("registered = %d, want 1", registered)
	}

	if _, err := s.AddFontCollectionFile(Proportional, filepath.Join(dir, "missing.ttf"), "Go"); err == nil {
		t.Error("AddFontCollectionFile with a nonexistent path returned no error, want one")
	}
}

func TestCustomFontFaceSelectorSetDPIInvalidatesCache(t *testing.T) {
	s := NewCustomFontFaceSelector(72, WithFallback(nil))
	if err := s.AddFont(Proportional, font.WeightNormal, font.StyleNormal, goregular.TTF, 0); err != nil {
		t.Fatalf("AddFont: %v", err)
	}
	style := TextStyle{Size: 16, Family: Proportional}
	face1, err := s.SelectFace(style)
	if err != nil {
		t.Fatalf("SelectFace: %v", err)
	}
	s.SetDPI(96)
	face2, err := s.SelectFace(style)
	if err != nil {
		t.Fatalf("SelectFace: %v", err)
	}
	if face1 == face2 {
		t.Error("SelectFace after SetDPI to a new value returned the same cached face")
	}
}

func TestCustomFontFaceSelectorSetDPIPropagatesToFallback(t *testing.T) {
	spy := &spyFaceSelector{}
	s := NewCustomFontFaceSelector(72, WithFallback(spy))
	s.SetDPI(96)
	s.SetDPI(96) // called again with the same value: must still forward unconditionally
	want := []float64{96, 96}
	if len(spy.setDPICalls) != len(want) {
		t.Fatalf("fallback.SetDPI called %v times, want %v", spy.setDPICalls, want)
	}
	for i, dpi := range want {
		if spy.setDPICalls[i] != dpi {
			t.Errorf("fallback.SetDPI call %d = %v, want %v", i, spy.setDPICalls[i], dpi)
		}
	}
}
