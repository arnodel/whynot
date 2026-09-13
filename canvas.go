package whynot

import (
	"image"
	"image/color"

	"golang.org/x/image/font"
)

// Canvas is the draw destination for a BlockLayout/InlineLayout tree - the boundary
// between layout (which never depends on a rendering backend) and actual
// drawing (which necessarily does). DrawImage receives an already-decoded,
// backend-agnostic image.Image - whynot.ImageCache owns resolving,
// fetching, and decoding (see InlineImage.GetInlineLayout), so a Canvas
// implementation never fetches anything itself; its own job is purely
// whatever backend-specific conversion drawing it needs (e.g. uploading
// it as a GPU texture), which it's free to cache itself, keyed by img's
// own identity - the same resolved image comes back from the cache
// every time it's asked for. width/height are the size to draw it at -
// already scaled by RenderingContext.Scale, and not generally img's
// own pixel size - so an implementation must scale what it draws to
// fit, not blit it 1:1.
type Canvas interface {
	Bounds() image.Rectangle
	DrawText(s string, face font.Face, x, y int, clr color.Color)
	DrawImage(img image.Image, x, y, width, height int)
	DrawRect(x, y, w, h int, clr color.Color)
}
