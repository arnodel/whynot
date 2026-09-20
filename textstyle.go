package whynot

import (
	"fmt"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/gomediumitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/gofont/gosmallcaps"
	"golang.org/x/image/font/gofont/gosmallcapsitalic"
	"golang.org/x/image/font/opentype"
)

type FontFamily int

const (
	Proportional FontFamily = iota
	Monospace
	SmallCaps
)

// String names f for logging/debugging - e.g. a FaceSelector reporting
// which family a font-resolution decision was made for.
func (f FontFamily) String() string {
	switch f {
	case Proportional:
		return "Proportional"
	case Monospace:
		return "Monospace"
	case SmallCaps:
		return "SmallCaps"
	default:
		return fmt.Sprintf("FontFamily(%d)", int(f))
	}
}

type TextStyle struct {
	Size   float64
	Style  font.Style
	Weight font.Weight
	Family FontFamily
}

type FaceSelector interface {
	SelectFace(TextStyle) (font.Face, error)
	// SetDPI updates the DPI used to rasterize faces going forward,
	// invalidating any cached font.Face built at the old DPI. Called every
	// View.Layout frame (see view.go), so implementations must treat a
	// call with an unchanged dpi as a cheap no-op.
	SetDPI(float64)
}

// normalizedFontKey buckets style's Weight into one of WeightNormal,
// WeightMedium, or WeightBold, and folds StyleOblique into StyleItalic -
// Size and any other fields are left zero. This is the lookup-key shape
// both goFonts and CustomFontFaceSelector's registered fonts use, so a
// resolved TextStyle (arbitrary Size/Weight) maps onto the same slot
// whether it's served by the built-in fonts or a caller-registered one.
func normalizedFontKey(style TextStyle) TextStyle {
	var key TextStyle
	switch {
	case style.Weight <= font.WeightNormal:
		key.Weight = font.WeightNormal
	case style.Weight <= font.WeightMedium:
		key.Weight = font.WeightMedium
	default:
		key.Weight = font.WeightBold
	}
	if style.Style == font.StyleOblique {
		key.Style = font.StyleItalic
	} else {
		key.Style = style.Style
	}
	key.Family = style.Family
	return key
}

type GoFontFaceSelector struct {
	cache       map[TextStyle]font.Face
	dpi         float64
	fontHinting font.Hinting
}

func NewGoFontFaceSelector(dpi float64) *GoFontFaceSelector {
	return &GoFontFaceSelector{
		cache:       map[TextStyle]font.Face{},
		dpi:         dpi,
		fontHinting: font.HintingNone,
	}
}

func (s *GoFontFaceSelector) SetDPI(dpi float64) {
	if dpi != s.dpi {
		s.cache = map[TextStyle]font.Face{}
		s.dpi = dpi
	}
}

func (s *GoFontFaceSelector) SelectFace(style TextStyle) (font.Face, error) {
	face, ok := s.cache[style]
	if ok {
		return face, nil
	}
	normStyle := normalizedFontKey(style)
	fontsrc := goFonts[normStyle]
	goFont, err := opentype.Parse(fontsrc)
	if err != nil {
		return nil, err
	}
	face, err = opentype.NewFace(goFont, &opentype.FaceOptions{
		Size:    style.Size,
		DPI:     s.dpi,
		Hinting: s.fontHinting,
	})
	if err != nil {
		return nil, err
	}
	s.cache[style] = face
	return face, nil
}

var goFonts = map[TextStyle][]byte{
	{0, font.StyleNormal, font.WeightNormal, Proportional}: goregular.TTF,
	{0, font.StyleItalic, font.WeightNormal, Proportional}: goitalic.TTF,
	{0, font.StyleNormal, font.WeightMedium, Proportional}: gomedium.TTF,
	{0, font.StyleItalic, font.WeightMedium, Proportional}: gomediumitalic.TTF,
	{0, font.StyleNormal, font.WeightBold, Proportional}:   gobold.TTF,
	{0, font.StyleItalic, font.WeightBold, Proportional}:   gobolditalic.TTF,
	{0, font.StyleNormal, font.WeightNormal, Monospace}:    gomono.TTF,
	{0, font.StyleItalic, font.WeightNormal, Monospace}:    gomonoitalic.TTF,
	{0, font.StyleNormal, font.WeightBold, Monospace}:      gomonobold.TTF,
	{0, font.StyleItalic, font.WeightBold, Monospace}:      gomonobolditalic.TTF,
	{0, font.StyleNormal, font.WeightNormal, SmallCaps}:    gosmallcaps.TTF,
	{0, font.StyleItalic, font.WeightNormal, SmallCaps}:    gosmallcapsitalic.TTF,
}
