package whynot

import (
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

	"github.com/arnodel/whynot/internal/styling"
)

// FontFamily is a broad family of fonts a FaceSelector chooses between.
type FontFamily = styling.FontFamily

const (
	Proportional = styling.Proportional
	Monospace    = styling.Monospace
	SmallCaps    = styling.SmallCaps
)

// TextStyle is a fully resolved text style: what a FaceSelector is asked
// to find a face for.
type TextStyle = styling.TextStyle

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
	{Style: font.StyleNormal, Weight: font.WeightNormal, Family: Proportional}: goregular.TTF,
	{Style: font.StyleItalic, Weight: font.WeightNormal, Family: Proportional}: goitalic.TTF,
	{Style: font.StyleNormal, Weight: font.WeightMedium, Family: Proportional}: gomedium.TTF,
	{Style: font.StyleItalic, Weight: font.WeightMedium, Family: Proportional}: gomediumitalic.TTF,
	{Style: font.StyleNormal, Weight: font.WeightBold, Family: Proportional}:   gobold.TTF,
	{Style: font.StyleItalic, Weight: font.WeightBold, Family: Proportional}:   gobolditalic.TTF,
	{Style: font.StyleNormal, Weight: font.WeightNormal, Family: Monospace}:    gomono.TTF,
	{Style: font.StyleItalic, Weight: font.WeightNormal, Family: Monospace}:    gomonoitalic.TTF,
	{Style: font.StyleNormal, Weight: font.WeightBold, Family: Monospace}:      gomonobold.TTF,
	{Style: font.StyleItalic, Weight: font.WeightBold, Family: Monospace}:      gomonobolditalic.TTF,
	{Style: font.StyleNormal, Weight: font.WeightNormal, Family: SmallCaps}:    gosmallcaps.TTF,
	{Style: font.StyleItalic, Weight: font.WeightNormal, Family: SmallCaps}:    gosmallcapsitalic.TTF,
}
