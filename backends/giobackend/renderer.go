package giobackend

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"gioui.org/f32"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/arnodel/whynot/canvas"
)

// Renderer owns resources - uploaded images and rasterized glyphs - that
// should persist across frames and across however many Canvases get
// created from it. Construct one and keep it for the life of the
// program; NewCanvas is cheap enough to call every frame.
type Renderer struct {
	// imageCache holds each image.Image's Gio-specific conversion, keyed
	// by the image.Image's own identity - see canvas.Canvas's own doc
	// comment on why this identity-keyed caching is safe.
	imageCache map[image.Image]paint.ImageOp
	glyphCache map[glyphKey]glyph
}

type glyphKey struct {
	face font.Face
	r    rune
	clr  color.Color
}

// glyph is a rasterized, colored glyph bitmap ready to paint. offset is
// where its top-left corner lands relative to the baseline dot it's
// drawn at (see Canvas.DrawText).
type glyph struct {
	op      paint.ImageOp
	offset  image.Point
	advance fixed.Int26_6
}

func New() *Renderer {
	return &Renderer{
		imageCache: map[image.Image]paint.ImageOp{},
		glyphCache: map[glyphKey]glyph{},
	}
}

// NewCanvas returns a Canvas that records drawing operations into ops,
// clipped to bounds - Gio has no sub-image concept to derive them from,
// unlike ebitenbackend's ebiten.Image.SubImage. Shares this Renderer's
// caches with every other Canvas it creates.
func (r *Renderer) NewCanvas(ops *op.Ops, bounds image.Rectangle) *Canvas {
	return &Canvas{ops: ops, bounds: bounds, clip: bounds, transform: canvas.Identity, renderer: r}
}

// imageOp converts img to a paint.ImageOp, memoized by img's own
// identity so the same decoded image is only ever uploaded once.
func (r *Renderer) imageOp(img image.Image) paint.ImageOp {
	if op, ok := r.imageCache[img]; ok {
		return op
	}
	op := paint.NewImageOp(img)
	r.imageCache[img] = op
	return op
}

// glyphFor rasterizes r under face, tinted clr, memoized by (face, r,
// clr) identity. Gio has no raster-mask-as-clip primitive to apply color
// at draw time the way a GPU glyph atlas normally would, so color has to
// be baked into the cached bitmap itself - a StyleSheet only ever uses a
// handful of distinct colors, so this stays bounded.
//
// Rasterized once at the origin (dot = (0,0)) regardless of where it'll
// actually be drawn: font rasterization is position-independent at
// whole-pixel dot positions, which whynot's layout already guarantees
// (every x/y Canvas.DrawText receives is a plain int) - so the cached
// bitmap is reused as-is, just translated, by every future draw.
func (r *Renderer) glyphFor(face font.Face, ru rune, clr color.Color) (glyph, bool) {
	key := glyphKey{face, ru, clr}
	if g, ok := r.glyphCache[key]; ok {
		return g, true
	}
	dr, mask, maskp, advance, ok := face.Glyph(fixed.P(0, 0), ru)
	if !ok || dr.Empty() {
		// dr.Empty() - e.g. a space - means no ink to paint, not just a
		// small one: painting a zero-sized paint.ImageOp still produced
		// visible garbage (the previous glyph, redrawn small) rather
		// than nothing, so this is treated the same as !ok - just
		// advance the pen, no image op at all.
		return glyph{}, false
	}
	// face.Glyph's own doc: the mask's contents may change after the
	// next Glyph call, so it has to be copied before caching.
	rgba := image.NewRGBA(image.Rect(0, 0, dr.Dx(), dr.Dy()))
	draw.DrawMask(rgba, rgba.Bounds(), image.NewUniform(clr), image.Point{}, mask, maskp, draw.Over)
	g := glyph{op: paint.NewImageOp(rgba), offset: dr.Min, advance: advance}
	r.glyphCache[key] = g
	return g, true
}

// Canvas records drawing operations into ops. bounds is its area in its
// own coordinates, which transform maps to device pixels, and clip is
// that area in device pixels. affine is transform as Gio applies it.
type Canvas struct {
	ops       *op.Ops
	bounds    image.Rectangle
	clip      image.Rectangle
	transform canvas.Transform
	affine    f32.Affine2D
	renderer  *Renderer
}

// Clip returns a Canvas whose draws are clipped to r within c's bounds.
// Gio clips to whole pixels here, so a transformed r is rounded to the
// nearest: clips side by side still meet.
func (c *Canvas) Clip(r image.Rectangle) canvas.Canvas {
	bounds := r.Intersect(c.bounds)
	device := roundRect(c.transform, bounds).Intersect(c.clip)
	return &Canvas{ops: c.ops, bounds: bounds, clip: device, transform: c.transform, affine: c.affine, renderer: c.renderer}
}

func (c *Canvas) Transform(t canvas.Transform) canvas.Canvas {
	transform := t.Then(c.transform)
	s := float32(transform.Scale)
	affine := f32.Affine2D{}.
		Scale(f32.Pt(0, 0), f32.Pt(s, s)).
		Offset(f32.Pt(float32(transform.X), float32(transform.Y)))
	return &Canvas{ops: c.ops, bounds: t.Invert().ApplyRect(c.bounds), clip: c.clip, transform: transform, affine: affine, renderer: c.renderer}
}

// roundRect returns r mapped through t, with its edges rounded to the
// nearest pixel.
func roundRect(t canvas.Transform, r image.Rectangle) image.Rectangle {
	x0, y0 := t.Apply(float64(r.Min.X), float64(r.Min.Y))
	x1, y1 := t.Apply(float64(r.Max.X), float64(r.Max.Y))
	return image.Rect(int(math.Round(x0)), int(math.Round(y0)), int(math.Round(x1)), int(math.Round(y1)))
}

// pushClip clips to c's area and applies c's transform until the
// returned func is called: Gio has no sub-image to draw into instead, so
// every draw is wrapped in one.
func (c *Canvas) pushClip() func() {
	clipped := clip.Rect(c.clip).Push(c.ops)
	if c.transform == canvas.Identity {
		return clipped.Pop
	}
	transformed := op.Affine(c.affine).Push(c.ops)
	return func() {
		transformed.Pop()
		clipped.Pop()
	}
}

var _ canvas.Canvas = (*Canvas)(nil)

func (c *Canvas) Bounds() image.Rectangle {
	return c.bounds
}

func (c *Canvas) DrawText(s string, face font.Face, x, y int, clr color.Color) {
	defer c.pushClip()()
	pen := x
	for _, ru := range s {
		g, ok := c.renderer.glyphFor(face, ru, clr)
		if !ok {
			if adv, ok := face.GlyphAdvance(ru); ok {
				pen += adv.Ceil()
			}
			continue
		}
		stack := op.Offset(image.Pt(pen, y).Add(g.offset)).Push(c.ops)
		g.op.Add(c.ops)
		paint.PaintOp{}.Add(c.ops)
		stack.Pop()
		pen += g.advance.Ceil()
	}
}

func (c *Canvas) DrawRect(x, y, w, h int, clr color.Color) {
	defer c.pushClip()()
	stack := op.Offset(image.Pt(x, y)).Push(c.ops)
	paint.FillShape(c.ops, toNRGBA(clr), clip.Rect(image.Rect(0, 0, w, h)).Op())
	stack.Pop()
}

// DrawImage scales the loaded image from its native pixel size to
// width/height (usually not the same size - see [canvas.Canvas]'s own doc
// comment).
func (c *Canvas) DrawImage(img image.Image, x, y, width, height int) {
	defer c.pushClip()()
	imgOp := c.renderer.imageOp(img)
	b := img.Bounds()
	sx, sy := float32(1), float32(1)
	if bw := b.Dx(); bw > 0 {
		sx = float32(width) / float32(bw)
	}
	if bh := b.Dy(); bh > 0 {
		sy = float32(height) / float32(bh)
	}
	transform := f32.Affine2D{}.
		Scale(f32.Pt(0, 0), f32.Pt(sx, sy)).
		Offset(f32.Pt(float32(x), float32(y)))
	stack := op.Affine(transform).Push(c.ops)
	imgOp.Add(c.ops)
	paint.PaintOp{}.Add(c.ops)
	stack.Pop()
}

func toNRGBA(c color.Color) color.NRGBA {
	return color.NRGBAModel.Convert(c).(color.NRGBA)
}
