// Package giorenderer implements whynot.Canvas on top of Gio (gioui.org).
// It's the only place in the module outside examples/gio that depends on
// Gio - the core whynot package has no rendering backend dependency at
// all.
//
// Gio has no equivalent of a font.Face-consuming text drawer (its own
// text.Shaper owns the whole shaping pipeline from raw font bytes, with
// no public per-glyph API) - so DrawText rasterizes each rune itself via
// font.Face.Glyph and paints the resulting bitmap through Gio's op/paint
// primitives, caching the rasterized result.
package giorenderer

import (
	"image"
	"image/color"
	"image/draw"

	"gioui.org/f32"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/arnodel/whynot"
)

// Renderer owns resources - uploaded images and rasterized glyphs - that
// should persist across frames and across however many Canvases get
// created from it. Construct one and keep it for the life of the
// program; NewCanvas is cheap enough to call every frame.
type Renderer struct {
	// imageCache holds each image.Image's Gio-specific conversion, keyed
	// by the image.Image's own identity - see whynot.Canvas's own doc
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
// reporting bounds for Bounds() - Gio has no sub-image concept to derive
// it from, unlike ebitenrenderer's ebiten.Image.SubImage. Shares this
// Renderer's caches with every other Canvas it creates.
func (r *Renderer) NewCanvas(ops *op.Ops, bounds image.Rectangle) *Canvas {
	return &Canvas{ops: ops, bounds: bounds, renderer: r}
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

type Canvas struct {
	ops      *op.Ops
	bounds   image.Rectangle
	renderer *Renderer
}

var _ whynot.Canvas = (*Canvas)(nil)

func (c *Canvas) Bounds() image.Rectangle {
	return c.bounds
}

func (c *Canvas) DrawText(s string, face font.Face, x, y int, clr color.Color) {
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
	stack := op.Offset(image.Pt(x, y)).Push(c.ops)
	paint.FillShape(c.ops, toNRGBA(clr), clip.Rect(image.Rect(0, 0, w, h)).Op())
	stack.Pop()
}

// DrawImage scales the loaded image from its native pixel size to
// width/height (usually not the same size - see whynot.Canvas's own doc
// comment).
func (c *Canvas) DrawImage(img image.Image, x, y, width, height int) {
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
