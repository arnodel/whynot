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
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font"

	"github.com/arnodel/whynot"
)

// Renderer owns resources - currently just loaded images - that should
// persist across frames and across however many Canvases get created from
// it. Construct one and keep it for the life of the program; NewCanvas is
// cheap enough to call every frame.
type Renderer struct {
	imageCache map[string]*ebiten.Image
}

func New() *Renderer {
	return &Renderer{imageCache: map[string]*ebiten.Image{}}
}

// NewCanvas returns a Canvas that draws onto dst, sharing this Renderer's
// image cache with every other Canvas it creates - so the same source
// image referenced by multiple Views only ever gets decoded and uploaded
// to the GPU once.
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

type Canvas struct {
	dst      *ebiten.Image
	renderer *Renderer
}

var _ whynot.Canvas = (*Canvas)(nil)

func (c *Canvas) Bounds() image.Rectangle {
	return c.dst.Bounds()
}

func (c *Canvas) DrawText(s string, face font.Face, x, y int, clr color.Color) {
	text.Draw(c.dst, s, face, x, y, clr)
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
