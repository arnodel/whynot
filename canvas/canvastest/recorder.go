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
// A Canvas from Clip or Transform records into the Recorder it came from,
// with its own bounds, and Clips records each Clip call. Calls are
// recorded as they're made, in the coordinates they're made in: what a
// Clip would cut off isn't removed, and a call through a Transform
// records the transform rather than mapping its coordinates.
type Recorder struct {
	Area   image.Rectangle
	Texts  []Text
	Rects  []Rect
	Images []Image
	Clips  []image.Rectangle // the bounds of each Canvas from Clip

	root      *Recorder        // set on a Canvas from Clip or Transform
	transform canvas.Transform // to root's coordinates; zero for none
}

// Text records a DrawText call.
type Text struct {
	S     string
	Face  font.Face
	X, Y  int
	Color color.Color
	// Transform is what the call was drawn through, from the
	// Recorder's coordinates: zero for none.
	Transform canvas.Transform
}

// Rect records a DrawRect call.
type Rect struct {
	X, Y, W, H int
	Color      color.Color
	Transform  canvas.Transform // as for Text
}

// Image records a DrawImage call.
type Image struct {
	Img        image.Image
	X, Y, W, H int
	Transform  canvas.Transform // as for Text
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
	out.Texts = append(out.Texts, Text{s, face, x, y, clr, c.transform})
}

// DrawImage records an Image.
func (c *Recorder) DrawImage(img image.Image, x, y, width, height int) {
	out := c.out()
	out.Images = append(out.Images, Image{img, x, y, width, height, c.transform})
}

// DrawRect records a Rect.
func (c *Recorder) DrawRect(x, y, w, h int, clr color.Color) {
	out := c.out()
	out.Rects = append(out.Rects, Rect{x, y, w, h, clr, c.transform})
}

// Clip returns a Canvas whose bounds are r within c's, recording into the
// same Recorder as c, and records those bounds in Clips.
func (c *Recorder) Clip(r image.Rectangle) canvas.Canvas {
	out := c.out()
	clipped := r.Intersect(c.Area)
	out.Clips = append(out.Clips, clipped)
	return &Recorder{Area: clipped, root: out, transform: c.transform}
}

// Transform returns a Canvas whose bounds are c's mapped back through t,
// recording into the same Recorder as c, with t followed by c's own
// transform.
func (c *Recorder) Transform(t canvas.Transform) canvas.Canvas {
	through := canvas.Identity
	if c.transform != (canvas.Transform{}) {
		through = c.transform
	}
	return &Recorder{Area: t.Invert().ApplyRect(c.Area), root: c.out(), transform: t.Then(through)}
}
