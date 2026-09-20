package whynot

import (
	"fmt"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// CustomFontFaceSelector is a FaceSelector backed by caller-supplied
// TTF/OTF font bytes, registered per (FontFamily, weight-bucket, style)
// slot via AddFont. Any (family, weight, style) combination with nothing
// registered is delegated to a fallback FaceSelector - NewGoFontFaceSelector
// by default, overridable via WithFallback - so callers only need to supply
// the fonts they actually care about overriding. WithFallback(nil) means
// "no fallback": SelectFace returns an error for any unfilled slot instead
// of delegating.
type CustomFontFaceSelector struct {
	fonts       map[TextStyle]*opentype.Font // keyed by normalizedFontKey
	faceCache   map[TextStyle]font.Face      // keyed by the full resolved TextStyle
	dpi         float64
	fallback    FaceSelector
	fontHinting font.Hinting
}

var _ FaceSelector = (*CustomFontFaceSelector)(nil)

// CustomFontFaceSelectorOption customizes a CustomFontFaceSelector at
// construction, via NewCustomFontFaceSelector's opts parameter.
type CustomFontFaceSelectorOption func(*CustomFontFaceSelector)

// WithFallback overrides the FaceSelector NewCustomFontFaceSelector
// otherwise defaults to (NewGoFontFaceSelector) for any (family, weight,
// style) slot nothing was registered for. Pass nil to disable fallback
// entirely - SelectFace then returns an error for an unfilled slot instead.
func WithFallback(fallback FaceSelector) CustomFontFaceSelectorOption {
	return func(s *CustomFontFaceSelector) {
		s.fallback = fallback
	}
}

// NewCustomFontFaceSelector returns a CustomFontFaceSelector with nothing
// registered yet - every SelectFace call delegates to its fallback
// FaceSelector (NewGoFontFaceSelector(dpi) unless overridden via
// WithFallback) until AddFont/AddFontFile is called.
func NewCustomFontFaceSelector(dpi float64, opts ...CustomFontFaceSelectorOption) *CustomFontFaceSelector {
	s := &CustomFontFaceSelector{
		fonts:       map[TextStyle]*opentype.Font{},
		faceCache:   map[TextStyle]font.Face{},
		dpi:         dpi,
		fallback:    NewGoFontFaceSelector(dpi),
		fontHinting: font.HintingNone,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// AddFont registers data (TTF or OTF bytes) as the font to serve for the
// (family, weight, style) slot, replacing whatever was registered for that
// slot before. weight and style are bucketed exactly like a resolved
// TextStyle is in SelectFace (see normalizedFontKey).
//
// data is parsed immediately, so a malformed font is reported here, at
// registration time, rather than surfacing later from SelectFace. On a
// parse error nothing is registered and any already-cached faces are left
// untouched.
func (s *CustomFontFaceSelector) AddFont(family FontFamily, weight font.Weight, style font.Style, data []byte) error {
	parsed, err := opentype.Parse(data)
	if err != nil {
		return fmt.Errorf("whynot: parsing font: %w", err)
	}
	key := normalizedFontKey(TextStyle{Weight: weight, Style: style, Family: family})
	s.fonts[key] = parsed
	// A face rasterized from the old (or absent) font for this slot may
	// already be cached at some size - drop every cached face so the next
	// SelectFace call re-rasterizes from what was just registered. AddFont
	// is meant to be called during setup, before rendering starts, so this
	// is not a hot-path concern.
	s.faceCache = map[TextStyle]font.Face{}
	return nil
}

// AddFontFile reads path and registers its contents for the (family, weight,
// style) slot, exactly as AddFont would with the file's bytes. A convenience
// for the common case of loading a font straight from disk; font bytes
// coming from anywhere else (embed.FS, a network fetch) still go through
// AddFont directly, the same way NewView itself only ever takes []byte.
func (s *CustomFontFaceSelector) AddFontFile(family FontFamily, weight font.Weight, style font.Style, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("whynot: reading font file: %w", err)
	}
	return s.AddFont(family, weight, style, data)
}

func (s *CustomFontFaceSelector) SelectFace(style TextStyle) (font.Face, error) {
	if face, ok := s.faceCache[style]; ok {
		return face, nil
	}
	key := normalizedFontKey(style)
	parsed, ok := s.fonts[key]
	if !ok {
		if s.fallback == nil {
			return nil, fmt.Errorf("whynot: no font registered for %+v and no fallback selector configured", key)
		}
		return s.fallback.SelectFace(style)
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    style.Size,
		DPI:     s.dpi,
		Hinting: s.fontHinting,
	})
	if err != nil {
		return nil, err
	}
	s.faceCache[style] = face
	return face, nil
}

func (s *CustomFontFaceSelector) SetDPI(dpi float64) {
	if dpi != s.dpi {
		s.faceCache = map[TextStyle]font.Face{}
		s.dpi = dpi
	}
	if s.fallback != nil {
		s.fallback.SetDPI(dpi)
	}
}
