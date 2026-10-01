// Package canvas is the drawing contract between whynot and a rendering
// backend: a View draws a document onto a Canvas, which a backend
// implements on top of its framework (Ebitengine, Gio, ...).
package canvas

import (
	"image"
	"image/color"

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
// scale and zoom, so a Canvas never scales them itself.
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
}
