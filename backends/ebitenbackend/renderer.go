package ebitenbackend

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font"

	"github.com/arnodel/whynot/canvas"
)

// Renderer owns resources - loaded images and, per font.Face, the glyph
// cache text/v2 keeps inside a GoXFace - that should persist across frames
// and across however many Canvases get created from it. Construct one and
// keep it for the life of the program; NewCanvas is cheap enough to call
// every frame.
type Renderer struct {
	// imageCache holds each image.Image's ebiten-specific conversion,
	// keyed by the image.Image's own identity - whynot's image cache
	// already guarantees the same resolved src yields the same decoded
	// image.Image every time GetInlineLayout asks for it, so this
	// package never fetches or decodes anything itself; it only
	// uploads a texture at most once per distinct decoded image.
	imageCache map[image.Image]*ebiten.Image
	faceCache  map[font.Face]*text.GoXFace
}

func New() *Renderer {
	return &Renderer{
		imageCache: map[image.Image]*ebiten.Image{},
		faceCache:  map[font.Face]*text.GoXFace{},
	}
}

// NewCanvas returns a Canvas that draws onto dst, sharing this Renderer's
// caches with every other Canvas it creates - so the same decoded image, or
// the same font.Face, referenced by multiple Views only ever gets uploaded
// as a texture or glyph-cached once.
func (r *Renderer) NewCanvas(dst *ebiten.Image) *Canvas {
	return &Canvas{dst: dst, bounds: dst.Bounds(), transform: canvas.Identity, renderer: r}
}

// ebitenImage converts img to an *ebiten.Image, memoized by img's own
// identity so the same decoded image is only ever uploaded as a
// texture once.
func (r *Renderer) ebitenImage(img image.Image) *ebiten.Image {
	if ei, ok := r.imageCache[img]; ok {
		return ei
	}
	ei := ebiten.NewImageFromImage(img)
	r.imageCache[img] = ei
	return ei
}

// goXFace returns text/v2's wrapper for face, memoized: GoXFace carries its
// own internal glyph cache, and text/v2's own docs say to reuse the same
// instance as much as possible rather than rewrapping the same face on
// every draw.
func (r *Renderer) goXFace(face font.Face) *text.GoXFace {
	if f, ok := r.faceCache[face]; ok {
		return f
	}
	f := text.NewGoXFace(face)
	r.faceCache[face] = f
	return f
}

// Canvas draws onto dst, a sub-image of the destination that clips to
// the Canvas's area in device pixels. bounds is that area in the
// Canvas's own coordinates, which transform maps to device pixels.
type Canvas struct {
	dst       *ebiten.Image
	bounds    image.Rectangle
	transform canvas.Transform
	renderer  *Renderer
}

var _ canvas.Canvas = (*Canvas)(nil)

func (c *Canvas) Bounds() image.Rectangle {
	return c.bounds
}

// geoM returns c's transform as a GeoM.
func (c *Canvas) geoM() ebiten.GeoM {
	var m ebiten.GeoM
	m.Scale(c.transform.Scale, c.transform.Scale)
	m.Translate(c.transform.X, c.transform.Y)
	return m
}

func (c *Canvas) DrawText(s string, face font.Face, x, y int, clr color.Color) {
	goXFace := c.renderer.goXFace(face)

	// whynot's layout (inline_layout.go's TextBox.BoundsAndAdvance)
	// computes every line position treating y as the text baseline, using
	// face.Metrics().Ascent.Ceil() - the *rounded* ascent. text/v2's own
	// rendering origin is the top of the line, not the baseline, so this
	// shifts up by that same rounded ascent to land exactly where layout
	// expects. Using text/v2's own raw (unrounded) HAscent here instead
	// would round-trip correctly for any single face in isolation, but
	// two different faces almost never share the exact same fractional
	// ascent - so mixing faces on one line (e.g. a monospace code span
	// next to proportional text, both sized so layout treats them as
	// sharing a baseline) would land each face's glyphs on a different
	// sub-pixel row: a small but real, visible baseline misalignment.
	// Matching layout's own rounding convention exactly removes that
	// per-face drift.
	ascent := float64(face.Metrics().Ascent.Ceil())

	opts := &text.DrawOptions{}
	opts.GeoM.Translate(float64(x), float64(y)-ascent)
	opts.GeoM.Concat(c.geoM())
	opts.ColorScale.SetWithColor(clr)
	text.Draw(c.dst, s, goXFace, opts)
}

// Clip draws through a sub-image, which ebiten clips to while keeping
// the parent's coordinates. A sub-image is in whole device pixels, so a
// transformed r is rounded to the nearest: clips side by side still
// meet.
func (c *Canvas) Clip(r image.Rectangle) canvas.Canvas {
	bounds := r.Intersect(c.bounds)
	device := roundRect(c.transform, bounds).Intersect(c.dst.Bounds())
	return &Canvas{dst: c.dst.SubImage(device).(*ebiten.Image), bounds: bounds, transform: c.transform, renderer: c.renderer}
}

func (c *Canvas) Transform(t canvas.Transform) canvas.Canvas {
	return &Canvas{dst: c.dst, bounds: t.Invert().ApplyRect(c.bounds), transform: t.Then(c.transform), renderer: c.renderer}
}

// roundRect returns r mapped through t, with its edges rounded to the
// nearest pixel.
func roundRect(t canvas.Transform, r image.Rectangle) image.Rectangle {
	x0, y0 := t.Apply(float64(r.Min.X), float64(r.Min.Y))
	x1, y1 := t.Apply(float64(r.Max.X), float64(r.Max.Y))
	return image.Rect(int(math.Round(x0)), int(math.Round(y0)), int(math.Round(x1)), int(math.Round(y1)))
}

// DrawRect antialiases a transformed rectangle, whose edges can fall
// between pixels.
func (c *Canvas) DrawRect(x, y, w, h int, clr color.Color) {
	x0, y0 := c.transform.Apply(float64(x), float64(y))
	s := c.transform.Scale
	vector.DrawFilledRect(c.dst, float32(x0), float32(y0), float32(float64(w)*s), float32(float64(h)*s), clr, c.transform != canvas.Identity)
}

// DrawImage scales the loaded image from its native pixel size to
// width/height (usually not the same size - see [canvas.Canvas]'s own
// doc comment) with linear filtering, so a zoomed-in image is smoothly
// scaled rather than drawn blocky (ebiten's default nearest-neighbor
// filter) or, worse, at the wrong size entirely.
func (c *Canvas) DrawImage(img image.Image, x, y, width, height int) {
	ei := c.renderer.ebitenImage(img)
	b := ei.Bounds()
	sx, sy := 1.0, 1.0
	if bw := b.Dx(); bw > 0 {
		sx = float64(width) / float64(bw)
	}
	if bh := b.Dy(); bh > 0 {
		sy = float64(height) / float64(bh)
	}
	opts := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	opts.GeoM.Scale(sx, sy)
	opts.GeoM.Translate(float64(x), float64(y))
	opts.GeoM.Concat(c.geoM())
	c.dst.DrawImage(ei, opts)
}
