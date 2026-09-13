// Package ebitenrenderer implements whynot.Canvas on top of ebiten. It's
// the only place in the module outside cmd/whynot that depends on ebiten -
// the core whynot package (parsing, layout, and the Canvas interface
// itself) has no rendering backend dependency at all.
package ebitenrenderer

import (
	"image"
	"image/color"
	"io"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font"

	"github.com/arnodel/whynot"
)

// ImageOpener opens the bytes for an already-resolved image src (a
// Canvas.DrawImage argument - by the time it reaches here, whatever
// whynot.ImageLoader the caller configured has already resolved it, so
// this only needs to fetch, not resolve). Pluggable via WithImageOpener
// so cmd/whynot can supply the same http(s)-or-file fetch its
// whynot.ImageLoader uses; the default matches this package's own
// previous behavior (a local file only).
type ImageOpener func(src string) (io.ReadCloser, error)

func defaultImageOpener(src string) (io.ReadCloser, error) {
	return os.Open(src)
}

// Renderer owns resources - loaded images and, per font.Face, the glyph
// cache text/v2 keeps inside a GoXFace - that should persist across frames
// and across however many Canvases get created from it. Construct one and
// keep it for the life of the program; NewCanvas is cheap enough to call
// every frame.
type Renderer struct {
	imageCache  map[string]*ebiten.Image
	faceCache   map[font.Face]*text.GoXFace
	imageOpener ImageOpener
}

// Option customizes a Renderer at construction, via New's opts parameter.
type Option func(*Renderer)

// WithImageOpener overrides the ImageOpener New otherwise defaults to
// (a local file open) - e.g. for http(s)-or-file fetching matching a
// whynot.ImageLoader configured on the View being drawn.
func WithImageOpener(open ImageOpener) Option {
	return func(r *Renderer) {
		r.imageOpener = open
	}
}

func New(opts ...Option) *Renderer {
	r := &Renderer{
		imageCache:  map[string]*ebiten.Image{},
		faceCache:   map[font.Face]*text.GoXFace{},
		imageOpener: defaultImageOpener,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// NewCanvas returns a Canvas that draws onto dst, sharing this Renderer's
// caches with every other Canvas it creates - so the same source image, or
// the same font.Face, referenced by multiple Views only ever gets decoded
// or glyph-cached once.
func (r *Renderer) NewCanvas(dst *ebiten.Image) *Canvas {
	return &Canvas{dst: dst, renderer: r}
}

// loadImage decodes src via r.imageOpener (a local file by default,
// overridable via WithImageOpener) - PNG/JPEG/GIF decoders are already
// registered process-wide by the core whynot package's own blank
// imports, since ebitenrenderer always imports it. A src that fails to
// open or decode caches a nil result (like this always has) - the
// caller (Canvas.DrawImage) treats a nil image as "draw nothing".
func (r *Renderer) loadImage(src string) *ebiten.Image {
	if img, ok := r.imageCache[src]; ok {
		return img
	}
	var img *ebiten.Image
	if rc, err := r.imageOpener(src); err == nil {
		defer rc.Close()
		if decoded, _, err := image.Decode(rc); err == nil {
			img = ebiten.NewImageFromImage(decoded)
		}
	}
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

// DrawImage scales the loaded image from its native pixel size to
// width/height (usually not the same size - see whynot.Canvas's own
// doc comment) with linear filtering, so a zoomed-in image is smoothly
// scaled rather than drawn blocky (ebiten's default nearest-neighbor
// filter) or, worse, at the wrong size entirely.
func (c *Canvas) DrawImage(src string, x, y, width, height int) {
	img := c.renderer.loadImage(src)
	if img == nil {
		return
	}
	b := img.Bounds()
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
	c.dst.DrawImage(img, opts)
}
