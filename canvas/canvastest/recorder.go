package canvastest

import (
	"image"
	"image/color"

	"golang.org/x/image/font"

	"github.com/arnodel/whynot/canvas"
)

// Recorder is a canvas.Canvas that records each call drawing on it,
// instead of drawing. Area is what its Bounds reports.
//
// A Canvas from Clip records into the Recorder it was clipped from, with
// the clipped bounds, and Clips records each Clip call. Calls are recorded
// as they're made: what a Clip would cut off isn't removed.
type Recorder struct {
	Area   image.Rectangle
	Texts  []Text
	Rects  []Rect
	Images []Image
	Clips  []image.Rectangle // the bounds of each Canvas from Clip

	root *Recorder // set on a Canvas from Clip
}

// Text records a DrawText call.
type Text struct {
	S     string
	Face  font.Face
	X, Y  int
	Color color.Color
}

// Rect records a DrawRect call.
type Rect struct {
	X, Y, W, H int
	Color      color.Color
}

// Image records a DrawImage call.
type Image struct {
	Img        image.Image
	X, Y, W, H int
}

var _ canvas.Canvas = (*Recorder)(nil)

// out is where c's draw calls are recorded.
func (c *Recorder) out() *Recorder {
	if c.root != nil {
		return c.root
	}
	return c
}

// Bounds returns c.Area.
func (c *Recorder) Bounds() image.Rectangle { return c.Area }

// DrawText records a Text.
func (c *Recorder) DrawText(s string, face font.Face, x, y int, clr color.Color) {
	out := c.out()
	out.Texts = append(out.Texts, Text{s, face, x, y, clr})
}

// DrawImage records an Image.
func (c *Recorder) DrawImage(img image.Image, x, y, width, height int) {
	out := c.out()
	out.Images = append(out.Images, Image{img, x, y, width, height})
}

// DrawRect records a Rect.
func (c *Recorder) DrawRect(x, y, w, h int, clr color.Color) {
	out := c.out()
	out.Rects = append(out.Rects, Rect{x, y, w, h, clr})
}

// Clip returns a Canvas whose bounds are r within c's, recording into the
// same Recorder as c, and records those bounds in Clips.
func (c *Recorder) Clip(r image.Rectangle) canvas.Canvas {
	out := c.out()
	clipped := r.Intersect(c.Area)
	out.Clips = append(out.Clips, clipped)
	return &Recorder{Area: clipped, root: out}
}
