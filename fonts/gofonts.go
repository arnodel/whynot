package fonts

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

// GoSelector is a FaceSelector serving the Go fonts bundled with
// golang.org/x/image. They have no bold small caps: asking for one is an
// error. Like the faces it returns ([font.Face]), it's not safe for
// concurrent use.
type GoSelector struct {
	fonts map[TextStyle]*opentype.Font // parsed on first use, keyed by fontKey
	faces map[faceKey]font.Face
}

var _ FaceSelector = (*GoSelector)(nil)

func NewGoSelector() *GoSelector {
	return &GoSelector{
		fonts: map[TextStyle]*opentype.Font{},
		faces: map[faceKey]font.Face{},
	}
}

func (s *GoSelector) SelectFace(style TextStyle, dpi float64) (font.Face, error) {
	key := faceKey{style, dpi}
	if face, ok := s.faces[key]; ok {
		return face, nil
	}
	parsed, err := s.font(fontKey(style))
	if err != nil {
		return nil, err
	}
	face, err := newFace(parsed, style, dpi)
	if err != nil {
		return nil, err
	}
	s.faces[key] = face
	return face, nil
}

// font returns the parsed Go font for slot, parsing it on first use.
func (s *GoSelector) font(slot TextStyle) (*opentype.Font, error) {
	if parsed, ok := s.fonts[slot]; ok {
		return parsed, nil
	}
	data, ok := goFonts[slot]
	if !ok {
		return nil, fmt.Errorf("fonts: no Go font for %v %v %v", slot.Family, slot.Weight, slot.Style)
	}
	parsed, err := opentype.Parse(data)
	if err != nil {
		return nil, err
	}
	s.fonts[slot] = parsed
	return parsed, nil
}

// newFace makes the face for style at dpi from a parsed font.
func newFace(parsed *opentype.Font, style TextStyle, dpi float64) (font.Face, error) {
	return opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    style.Size,
		DPI:     dpi,
		Hinting: font.HintingNone,
	})
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
