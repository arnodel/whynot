package fonts

import (
	"fmt"

	"golang.org/x/image/font"
)

// FaceSelector supplies the font face for a text style. It's called with
// fully resolved styles, at the dpi the text will be drawn at.
//
// The same selector may serve several Views at different scales: a face is
// identified by the (style, dpi) pair, and an implementation should keep
// no other state about scale.
type FaceSelector interface {
	// SelectFace returns a face for style at dpi. style.Size is in points
	// and dpi is in dots per inch, kept separate rather than combined into
	// a pixel size, since a font may draw the same pixel size differently
	// at different point sizes (optical sizing).
	SelectFace(style TextStyle, dpi float64) (font.Face, error)
}

// TextStyle is a fully resolved text style: what a FaceSelector is asked
// to find a face for.
type TextStyle struct {
	// Size is in points.
	Size   float64
	Style  font.Style
	Weight font.Weight
	Family Family
}

// Family is a broad kind of font a FaceSelector chooses between.
type Family int

const (
	Proportional Family = iota
	Monospace
	SmallCaps
)

// String names f, e.g. for logging which family a face was chosen for.
func (f Family) String() string {
	switch f {
	case Proportional:
		return "Proportional"
	case Monospace:
		return "Monospace"
	case SmallCaps:
		return "SmallCaps"
	default:
		return fmt.Sprintf("Family(%d)", int(f))
	}
}

// faceKey identifies a cached face: a FaceSelector's two inputs.
type faceKey struct {
	style TextStyle
	dpi   float64
}

// fontKey buckets style's Weight into one of WeightNormal, WeightMedium,
// or WeightBold, and folds StyleOblique into StyleItalic, leaving Size
// zero: the slot a style's font is registered under, for both the Go fonts
// and CustomSelector.
func fontKey(style TextStyle) TextStyle {
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
