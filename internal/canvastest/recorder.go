// Package canvastest provides a canvas.Canvas fake for tests: Recorder
// records each draw call instead of drawing.
package canvastest

import (
	"image"
	"image/color"

	"golang.org/x/image/font"

	"github.com/arnodel/whynot/canvas"
)

// Rect records one Canvas.DrawRect call.
type Rect struct {
	X, Y, W, H int
	Color      color.Color
}

// Recorder is a minimal Canvas fake that records draw calls - enough to
// check what gets drawn where without a real rendering backend. Area is
// what Bounds reports. A Canvas from Clip records into the same Recorder
// it was clipped from.
type Recorder struct {
	Area       image.Rectangle
	Rects      []Rect
	Images     []image.Image
	ImageRects []image.Rectangle // one per DrawImage call, parallel to images
	Texts      []Text
	Clips      []image.Rectangle // one per Clip call

	root *Recorder // set on a Canvas from Clip
}

// Text records one Canvas.DrawText call.
type Text struct {
	S    string
	X, Y int
}

var _ canvas.Canvas = (*Recorder)(nil)

// out is where c's draw calls are recorded.
func (c *Recorder) out() *Recorder {
	if c.root != nil {
		return c.root
	}
	return c
}

func (c *Recorder) Bounds() image.Rectangle { return c.Area }
func (c *Recorder) DrawText(s string, face font.Face, x, y int, clr color.Color) {
	out := c.out()
	out.Texts = append(out.Texts, Text{s, x, y})
}
func (c *Recorder) DrawImage(img image.Image, x, y, width, height int) {
	out := c.out()
	out.Images = append(out.Images, img)
	out.ImageRects = append(out.ImageRects, image.Rect(x, y, x+width, y+height))
}
func (c *Recorder) DrawRect(x, y, w, h int, clr color.Color) {
	out := c.out()
	out.Rects = append(out.Rects, Rect{x, y, w, h, clr})
}
func (c *Recorder) Clip(r image.Rectangle) canvas.Canvas {
	out := c.out()
	clipped := r.Intersect(c.Area)
	out.Clips = append(out.Clips, clipped)
	return &Recorder{Area: clipped, root: out}
}
