package whynot

import (
	"errors"
	"testing"

	"golang.org/x/image/font"

	"github.com/arnodel/whynot/fonts"
)

// failingFaceSelector has no face for any style.
type failingFaceSelector struct{}

func (failingFaceSelector) SelectFace(fonts.TextStyle, float64) (font.Face, error) {
	return nil, errors.New("no fonts here")
}

// TestSelectFaceScalesDPI checks the dpi passed to the FaceSelector is
// Scale × 72: point size stays, the face grows.
func TestSelectFaceScalesDPI(t *testing.T) {
	selector := fonts.NewGoSelector()
	style := fonts.TextStyle{Size: 16}
	at1 := RenderingContext{Scale: 1, FaceSelector: selector}.selectFace(style)
	at2 := RenderingContext{Scale: 2, FaceSelector: selector}.selectFace(style)
	h1, h2 := at1.Metrics().Height, at2.Metrics().Height
	if h2 < 2*h1-2 || h2 > 2*h1+2 { // fixed.Int26_6: allow rounding
		t.Errorf("height at scale 2 = %v, want about 2 × %v", h2, h1)
	}
}

// TestSelectFaceFallsBackToGoFonts checks a failing FaceSelector degrades
// to the Go fonts rather than breaking layout, including for a style the
// Go fonts lack.
func TestSelectFaceFallsBackToGoFonts(t *testing.T) {
	ctx := RenderingContext{Scale: 1, FaceSelector: failingFaceSelector{}}
	for _, style := range []fonts.TextStyle{
		{Size: 16, Family: fonts.Monospace},
		{Size: 16, Weight: font.WeightBold, Family: fonts.SmallCaps},
	} {
		if face := ctx.selectFace(style); face == nil {
			t.Errorf("selectFace(%+v) = nil, want a Go font face", style)
		}
	}
}
