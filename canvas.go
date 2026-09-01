package whynot

import (
	"image"
	"image/color"

	"golang.org/x/image/font"
)

// Canvas is the draw destination for a Box/InlineBox tree - the boundary
// between layout (which never depends on a rendering backend) and actual
// drawing (which necessarily does). DrawImage takes a source path rather
// than loaded pixel data: layout only ever probes an image's dimensions
// (see InlineImage.GetInlineBox), never its pixels, so loading is entirely
// a Canvas implementation's concern - including whether and how to cache
// what it loads.
type Canvas interface {
	Bounds() image.Rectangle
	DrawText(s string, face font.Face, x, y int, clr color.Color)
	DrawImage(src string, x, y int)
	DrawRect(x, y, w, h int, clr color.Color)
}
