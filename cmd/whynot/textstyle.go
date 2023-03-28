package main

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
)

type FontFamily int

const (
	Proportional FontFamily = iota
	Monospace
	SmallCaps
)

type TextStyle struct {
	Size   float64
	Style  font.Style
	Weight font.Weight
	Family FontFamily
}

type FaceSelector interface {
	SelectFace(TextStyle) (font.Face, error)
	SetDPI(float64)
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
	var normStyle TextStyle
	switch {
	case style.Weight <= font.WeightNormal:
		normStyle.Weight = font.WeightNormal
	case style.Weight <= font.WeightMedium:
		normStyle.Weight = font.WeightMedium
	default:
		normStyle.Weight = font.WeightBold
	}
	if style.Style == font.StyleOblique {
		normStyle.Style = font.StyleItalic
	} else {
		normStyle.Style = style.Style
	}
	normStyle.Family = style.Family
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
