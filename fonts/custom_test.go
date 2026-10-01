package fonts

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
	calls []faceKey
	face  font.Face
}

func (s *spyFaceSelector) SelectFace(style TextStyle, dpi float64) (font.Face, error) {
	s.calls = append(s.calls, faceKey{style, dpi})
	return s.face, nil
}

func TestCustomSelectorSelectsRegisteredFont(t *testing.T) {
	s := NewCustomSelector(WithFallback(nil))
	if err := s.AddFont(Proportional, font.WeightNormal, font.StyleNormal, goregular.TTF, 0); err != nil {
		t.Fatalf("AddFont: %v", err)
	}
	face, err := s.SelectFace(TextStyle{Size: 16, Family: Proportional}, 72)
	if err != nil {
		t.Fatalf("SelectFace: %v", err)
	}
	if face == nil {
		t.Fatal("SelectFace returned a nil face")
	}
}

func TestCustomSelectorCachesFace(t *testing.T) {
	s := NewCustomSelector(WithFallback(nil))
	if err := s.AddFont(Proportional, font.WeightNormal, font.StyleNormal, goregular.TTF, 0); err != nil {
		t.Fatalf("AddFont: %v", err)
	}
	style := TextStyle{Size: 16, Family: Proportional}
	face1, err := s.SelectFace(style, 72)
	if err != nil {
		t.Fatalf("SelectFace: %v", err)
	}
	face2, err := s.SelectFace(style, 72)
	if err != nil {
		t.Fatalf("SelectFace: %v", err)
	}
	if face1 != face2 {
		t.Error("SelectFace with the same TextStyle returned different faces, expected a cached one")
	}
	face3, err := s.SelectFace(TextStyle{Size: 24, Family: Proportional}, 72)
	if err != nil {
		t.Fatalf("SelectFace: %v", err)
	}
	if face1 == face3 {
		t.Error("SelectFace with a different Size returned the same cached face")
	}
}

func TestCustomSelectorNormalizationMatchesBuiltin(t *testing.T) {
	s := NewCustomSelector(WithFallback(nil))
	// Register at an "off" weight/style that should still bucket into the
	// Bold/Italic slot, mirroring GoSelector's own bucketing.
	if err := s.AddFont(Proportional, font.WeightSemiBold, font.StyleOblique, gobold.TTF, 0); err != nil {
		t.Fatalf("AddFont: %v", err)
	}
	face, err := s.SelectFace(TextStyle{Size: 16, Weight: font.WeightBold, Style: font.StyleItalic, Family: Proportional}, 72)
	if err != nil {
		t.Fatalf("SelectFace: %v", err)
	}
	if face == nil {
		t.Fatal("SelectFace returned a nil face; expected the registered Bold/Italic font to resolve")
	}
}

func TestCustomSelectorFallsBackForUnfilledSlot(t *testing.T) {
	goFont, err := NewGoSelector().SelectFace(TextStyle{Size: 16, Family: Proportional}, 72)
	if err != nil {
		t.Fatalf("building a face for the fallback spy: %v", err)
	}
	spy := &spyFaceSelector{face: goFont}
	s := NewCustomSelector(WithFallback(spy))
	style := TextStyle{Size: 16, Family: Monospace}
	face, err := s.SelectFace(style, 72)
	if err != nil {
		t.Fatalf("SelectFace: %v", err)
	}
	if face != spy.face {
		t.Error("SelectFace did not return the fallback's face for an unfilled slot")
	}
	if want := (faceKey{style, 72}); len(spy.calls) != 1 || spy.calls[0] != want {
		t.Errorf("fallback.SelectFace called with %v, want exactly one call with %v", spy.calls, want)
	}
}

func TestCustomSelectorDefaultFallbackIsGoFonts(t *testing.T) {
	s := NewCustomSelector()
	if _, err := s.SelectFace(TextStyle{Size: 16, Family: Monospace}, 72); err != nil {
		t.Errorf("SelectFace with no options and an unfilled slot: %v, want it served by the default GoSelector fallback", err)
	}
}

func TestCustomSelectorNilFallback(t *testing.T) {
	s := NewCustomSelector(WithFallback(nil))
	if _, err := s.SelectFace(TextStyle{Size: 16, Family: Monospace}, 72); err == nil {
		t.Error("SelectFace with a nil fallback and an unfilled slot returned no error, want one")
	}
}

func TestCustomSelectorAddFontInvalidBytes(t *testing.T) {
	s := NewCustomSelector()
	if err := s.AddFont(Proportional, font.WeightNormal, font.StyleNormal, []byte("not a font"), 0); err == nil {
		t.Fatal("AddFont with invalid bytes returned no error, want one")
	}
	// The failed registration should leave the slot unfilled, still falling
	// back cleanly.
	if _, err := s.SelectFace(TextStyle{Size: 16, Family: Proportional}, 72); err != nil {
		t.Errorf("SelectFace after a failed AddFont: %v, want a clean fallback", err)
	}
}

func TestCustomSelectorAddFontIndexOutOfRange(t *testing.T) {
	s := NewCustomSelector(WithFallback(nil))
	err := s.AddFont(Proportional, font.WeightNormal, font.StyleNormal, goregular.TTF, 3)
	if err == nil {
		t.Fatal("AddFont with an out-of-range index returned no error, want one")
	}
	if !errors.Is(err, sfnt.ErrNotFound) {
		t.Errorf("AddFont out-of-range error = %v, want it to wrap sfnt.ErrNotFound", err)
	}
	if _, err := s.SelectFace(TextStyle{Size: 16, Family: Proportional}, 72); err == nil {
		t.Error("SelectFace after a failed AddFont with a nil fallback returned no error, want one")
	}
}

func TestCustomSelectorAddFontFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "font.ttf")
	if err := os.WriteFile(path, goregular.TTF, 0o644); err != nil {
		t.Fatalf("writing test font file: %v", err)
	}
	s := NewCustomSelector(WithFallback(nil))
	if err := s.AddFontFile(Proportional, font.WeightNormal, font.StyleNormal, path, 0); err != nil {
		t.Fatalf("AddFontFile: %v", err)
	}
	if _, err := s.SelectFace(TextStyle{Size: 16, Family: Proportional}, 72); err != nil {
		t.Errorf("SelectFace after AddFontFile: %v", err)
	}

	if err := s.AddFontFile(Proportional, font.WeightNormal, font.StyleNormal, filepath.Join(dir, "missing.ttf"), 0); err == nil {
		t.Error("AddFontFile with a nonexistent path returned no error, want one")
	}
}

func TestCustomSelectorAddFontCollectionRegistersMatchingFamily(t *testing.T) {
	s := NewCustomSelector(WithFallback(nil))
	registered, err := s.AddFontCollection(Proportional, goregular.TTF, "Go")
	if err != nil {
		t.Fatalf("AddFontCollection: %v", err)
	}
	if registered != 1 {
		t.Fatalf("registered = %d, want 1", registered)
	}
	if _, err := s.SelectFace(TextStyle{Size: 16, Family: Proportional}, 72); err != nil {
		t.Errorf("SelectFace after AddFontCollection: %v", err)
	}
}

func TestCustomSelectorAddFontCollectionEmptyMatchFamily(t *testing.T) {
	s := NewCustomSelector(WithFallback(nil))
	registered, err := s.AddFontCollection(Proportional, goregular.TTF, "")
	if err != nil {
		t.Fatalf("AddFontCollection: %v", err)
	}
	if registered != 1 {
		t.Fatalf("registered = %d, want 1 (empty matchFamily should register regardless of family)", registered)
	}
}

func TestCustomSelectorAddFontCollectionFamilyMismatch(t *testing.T) {
	s := NewCustomSelector(WithFallback(nil))
	registered, err := s.AddFontCollection(Proportional, goregular.TTF, "Arial")
	if err != nil {
		t.Fatalf("AddFontCollection: %v", err)
	}
	if registered != 0 {
		t.Fatalf("registered = %d, want 0 (real family is \"Go\", not \"Arial\")", registered)
	}
}

func TestCustomSelectorAddFontCollectionInvalidBytes(t *testing.T) {
	s := NewCustomSelector(WithFallback(nil))
	if _, err := s.AddFontCollection(Proportional, []byte("not a font"), ""); err == nil {
		t.Error("AddFontCollection with invalid bytes returned no error, want one")
	}
}

func TestCustomSelectorAddFontCollectionFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "font.ttf")
	if err := os.WriteFile(path, goregular.TTF, 0o644); err != nil {
		t.Fatalf("writing test font file: %v", err)
	}
	s := NewCustomSelector(WithFallback(nil))
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

// TestCustomSelectorCachesFacePerDPI checks that faces are cached per
// (style, dpi): the same style at another dpi is a different face, and
// going back to the first dpi reuses its face.
func TestCustomSelectorCachesFacePerDPI(t *testing.T) {
	s := NewCustomSelector(WithFallback(nil))
	if err := s.AddFont(Proportional, font.WeightNormal, font.StyleNormal, goregular.TTF, 0); err != nil {
		t.Fatalf("AddFont: %v", err)
	}
	style := TextStyle{Size: 16, Family: Proportional}
	at72, _ := s.SelectFace(style, 72)
	at144, _ := s.SelectFace(style, 144)
	if at72 == at144 {
		t.Error("SelectFace at two dpis returned the same face")
	}
	if again, _ := s.SelectFace(style, 72); again != at72 {
		t.Error("SelectFace back at the first dpi returned a new face, want the cached one")
	}
}
