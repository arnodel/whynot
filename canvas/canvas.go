package canvas

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/font"
)

// Canvas is a destination a View draws a document onto. It's the
// boundary between layout, which never depends on a rendering backend,
// and drawing, which necessarily does. Implementing it is how whynot is
// ported to a new backend.
//
// Coordinates are integer device pixels, in the destination's own
// coordinate space: the same space as Bounds, whose Min need not be
// (0, 0). A View passes coordinates already scaled for its display
// scale and zoom, so a Canvas only scales them as Transform asks.
//
// Colors may be translucent: every method draws by compositing its
// color or image over what is already there (Porter-Duff "source
// over"), never by replacing it.
//
// Nothing is ever visible outside Bounds. A View relies on this: it
// draws content that is only partly visible, such as a block that
// starts above the top of the viewport, and leaves cutting it off to
// the Canvas.
//
// A Canvas is used from a single goroutine at a time. Its methods may
// cache per-value conversions (an uploaded texture per image.Image, a
// glyph cache per font.Face): see DrawImage and DrawText for what
// stays the same between calls.
type Canvas interface {
	// Bounds returns the visible area of the destination. A View fills
	// it with its background color, and lays out and draws only what
	// intersects it.
	Bounds() image.Rectangle

	// DrawText draws s in face and clr, on one line, starting at (x, y),
	// where y is the baseline. Glyphs are placed as font.Drawer places
	// them: each advanced by face's advance width plus its kerning with
	// the previous glyph, in face's fixed-point units, with no rounding
	// between glyphs. A View measures text with font.BoundString, so
	// anything else drifts from where layout expects the text to be.
	//
	// The same font.Face value comes back for the same text style at the
	// same scale, as long as the View's FaceSelector caches its faces
	// (those in package fonts do), so it's a suitable cache key.
	DrawText(s string, face font.Face, x, y int, clr color.Color)

	// DrawImage draws the whole of img (all of img.Bounds()) scaled to
	// width by height pixels, with its top-left corner at (x, y). The
	// size is generally not img's own pixel size, so an implementation
	// must scale, preferably with smooth (e.g. bilinear) filtering.
	//
	// img is never modified after it's passed in, and the same loaded
	// image comes back as the same image.Image value every frame, so
	// img is a suitable cache key, e.g. for an uploaded texture.
	DrawImage(img image.Image, x, y, width, height int)

	// DrawRect fills the rectangle with top-left corner (x, y), w pixels
	// wide and h pixels high, with clr. A rectangle with zero width or
	// height draws nothing.
	DrawRect(x, y, w, h int, clr color.Color)

	// Clip returns a Canvas that draws onto the same destination, in the
	// same coordinates, but only within r intersected with Bounds() -
	// which is what the returned Canvas's Bounds reports. That
	// intersection may be empty, in which case nothing it draws is
	// visible. Clips nest: clipping the returned Canvas again intersects
	// further. The original Canvas is unaffected and stays usable.
	Clip(r image.Rectangle) Canvas

	// Transform returns a Canvas that draws onto the same destination,
	// with each point mapped through t, then as the original Canvas maps
	// it. Its Bounds are the smallest rectangle containing the original's
	// Bounds mapped back through t (see [Transform.ApplyRect]), and
	// nothing it draws is visible outside the original's Bounds.
	// Transforms nest, and the original Canvas is unaffected and stays
	// usable.
	//
	// Clip on the returned Canvas takes r in its own coordinates. Where
	// r's edges don't land on whole device pixels, an implementation may
	// round them, so clipping before transforming is the exact way to
	// cut off a transformed drawing.
	//
	// With a Scale above 1, an implementation may draw text and images
	// by enlarging their pixels, which blurs them: drawing them at a
	// larger size is laying them out again.
	Transform(t Transform) Canvas
}

// A Transform maps the point (x, y) to (x*Scale + X, y*Scale + Y), as
// Canvas.Transform applies it. Scale must be positive. Kinds of
// transform added later will be fields that mean "none" when zero.
type Transform struct {
	Scale, X, Y float64
}

// Identity is the Transform that leaves every point where it is.
var Identity = Transform{Scale: 1}

// Then returns the transform that applies t, then u.
func (t Transform) Then(u Transform) Transform {
	return Transform{Scale: t.Scale * u.Scale, X: t.X*u.Scale + u.X, Y: t.Y*u.Scale + u.Y}
}

// Invert returns the transform that undoes t.
func (t Transform) Invert() Transform {
	return Transform{Scale: 1 / t.Scale, X: -t.X / t.Scale, Y: -t.Y / t.Scale}
}

// Apply returns where t maps (x, y).
func (t Transform) Apply(x, y float64) (float64, float64) {
	return x*t.Scale + t.X, y*t.Scale + t.Y
}

// ApplyRect returns the smallest rectangle containing r mapped through t.
func (t Transform) ApplyRect(r image.Rectangle) image.Rectangle {
	x0, y0 := t.Apply(float64(r.Min.X), float64(r.Min.Y))
	x1, y1 := t.Apply(float64(r.Max.X), float64(r.Max.Y))
	return image.Rect(int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1)), int(math.Ceil(y1)))
}
