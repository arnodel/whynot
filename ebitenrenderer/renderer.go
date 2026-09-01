// Package ebitenrenderer implements whynot.Canvas on top of ebiten. It's
// the only place in the module outside cmd/whynot that depends on ebiten -
// the core whynot package (parsing, layout, and the Canvas interface
// itself) has no rendering backend dependency at all.
package ebitenrenderer

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
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
	imageCache map[string]*ebiten.Image
	faceCache  map[font.Face]*text.GoXFace
}

func New() *Renderer {
	return &Renderer{
		imageCache: map[string]*ebiten.Image{},
		faceCache:  map[font.Face]*text.GoXFace{},
	}
}

// NewCanvas returns a Canvas that draws onto dst, sharing this Renderer's
// caches with every other Canvas it creates - so the same source image, or
// the same font.Face, referenced by multiple Views only ever gets decoded
// or glyph-cached once.
func (r *Renderer) NewCanvas(dst *ebiten.Image) *Canvas {
	return &Canvas{dst: dst, renderer: r}
}

func (r *Renderer) loadImage(src string) *ebiten.Image {
	if img, ok := r.imageCache[src]; ok {
		return img
	}
	img, _, _ := ebitenutil.NewImageFromFile(src)
	r.imageCache[src] = img
	return img
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

	// whynot's layout (box.go, via font.BoundString) computes every
	// position treating y as the text baseline, but text/v2's default
	// rendering origin is the top of the line, not the baseline - so
	// shift up by the ascent to land at the baseline layout expects.
	ascent := goXFace.Metrics().HAscent

	opts := &text.DrawOptions{}
	opts.GeoM.Translate(float64(x), float64(y)-ascent)
	opts.ColorScale.SetWithColor(clr)
	text.Draw(c.dst, s, goXFace, opts)
}

func (c *Canvas) DrawRect(x, y, w, h int, clr color.Color) {
	vector.DrawFilledRect(c.dst, float32(x), float32(y), float32(w), float32(h), clr, false)
}

func (c *Canvas) DrawImage(src string, x, y int) {
	img := c.renderer.loadImage(src)
	if img == nil {
		return
	}
	geoM := ebiten.GeoM{}
	geoM.Translate(float64(x), float64(y))
	c.dst.DrawImage(img, &ebiten.DrawImageOptions{GeoM: geoM})
}
