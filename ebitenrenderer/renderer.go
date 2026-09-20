// Package ebitenrenderer implements whynot.Canvas on top of ebiten. It's
// the only place in the module outside cmd/whynot that depends on ebiten -
// the core whynot package (parsing, layout, and the Canvas interface
// itself) has no rendering backend dependency at all.
package ebitenrenderer

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font"

	"github.com/arnodel/whynot"
)

// Renderer owns resources - loaded images and, per font.Face, the glyph
// cache text/v2 keeps inside a GoXFace - that should persist across frames
// and across however many Canvases get created from it. Construct one and
// keep it for the life of the program; NewCanvas is cheap enough to call
// every frame.
type Renderer struct {
	// imageCache holds each image.Image's ebiten-specific conversion,
	// keyed by the image.Image's own identity - whynot.ImageCache
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
	return &Canvas{dst: dst, renderer: r}
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

type Canvas struct {
	dst      *ebiten.Image
	renderer *Renderer
}

var _ whynot.Canvas = (*Canvas)(nil)

func (c *Canvas) Bounds() image.Rectangle {
	return c.dst.Bounds()
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
	opts.ColorScale.SetWithColor(clr)
	text.Draw(c.dst, s, goXFace, opts)
}

func (c *Canvas) DrawRect(x, y, w, h int, clr color.Color) {
	vector.DrawFilledRect(c.dst, float32(x), float32(y), float32(w), float32(h), clr, false)
}

// DrawImage scales the loaded image from its native pixel size to
// width/height (usually not the same size - see whynot.Canvas's own
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
	c.dst.DrawImage(ei, opts)
}
