package engine

import (
	"image"
	"image/color"
	"math"
	"time"

	"github.com/arnodel/whynot/canvas"
)

// Horizontal scrolling for blocks wider than the width they're laid out
// at (see issue #25): a ScrollBox shows a window onto the content,
// shifted by an offset the View keeps per source Block, so scrolling -
// like hovering, which reveals the scrollbar - never needs a relayout.

// Dimensions of a ScrollBox's edge fades, in logical (unscaled) pixels.
const (
	scrollFadeWidth = 24
	scrollFadeSteps = 8
)

// ScrollBox shows a window, width wide, onto content that's wider,
// scrolled horizontally by the offset its View keeps for source. An edge
// fades into the page background wherever there's hidden content, and a
// scrollbar is drawn by the Context's Scrollbar hook, if any.
type ScrollBox struct {
	inner      BlockLayout
	width      int
	source     Block
	scroll     func(Block, Axis) float64 // Context.ScrollOffset; nil: never scrolls
	scrollbar  ScrollbarFunc             // Context.Scrollbar; nil: no scrollbar
	scale      float64
	background color.Color
}

var _ BlockLayout = (*ScrollBox)(nil)

// scrollIfWider returns inner wrapped in a ScrollBox if it's wider than
// width, and inner itself otherwise.
func scrollIfWider(ctx Context, source Block, inner BlockLayout, width int) BlockLayout {
	if width <= 0 || inner.Bounds().Dx() <= width {
		return inner
	}
	return &ScrollBox{
		inner:      inner,
		width:      width,
		source:     source,
		scroll:     ctx.ScrollOffset,
		scrollbar:  ctx.Scrollbar,
		scale:      ctx.Scale,
		background: ctx.Styles.BackgroundColor(),
	}
}

func (b *ScrollBox) Bounds() image.Rectangle {
	return image.Rect(0, 0, b.width, b.inner.Bounds().Dy())
}

func (b *ScrollBox) Source() Source {
	return b.inner.Source()
}

func (b *ScrollBox) PendingImages() []string {
	return b.inner.PendingImages()
}

func (b *ScrollBox) contentWidth() int {
	return b.inner.Bounds().Dx()
}

// offset is the current scroll offset, clamped to the content: the width
// may have changed since it was set.
func (b *ScrollBox) offset() int {
	if b.scroll == nil {
		return 0
	}
	return int(math.Max(0, math.Min(float64(b.contentWidth()-b.width), b.scroll(b.source, Horizontal))))
}

func (b *ScrollBox) HitTest(p image.Point) (Hit, image.Point) {
	shift := image.Pt(b.offset(), 0)
	local := p.Add(shift)
	if !local.In(b.inner.Bounds()) {
		return nil, image.Point{}
	}
	hit, offset := b.inner.HitTest(local)
	if hit == nil {
		return nil, image.Point{}
	}
	return hit, offset.Sub(shift)
}

func (b *ScrollBox) drawContents(dst canvas.Canvas, x, y int, now time.Duration) {
	box := image.Rect(x, y, x+b.width, y+b.inner.Bounds().Dy())
	offset := b.offset()
	clipped := dst.Clip(box)
	DrawBlockLayout(b.inner, clipped, x-offset, y, now)
	b.drawFades(clipped, box, offset)

	if b.scrollbar != nil {
		b.scrollbar(clipped, ScrollRegion{
			Source:      b.source,
			Axis:        Horizontal,
			Box:         box,
			Visible:     clipped.Bounds(),
			ContentSize: b.contentWidth(),
			Offset:      offset,
		}, now)
	}
}

// drawFades fades each edge of box that has content hidden past it into
// the page background, with stepped transparency.
func (b *ScrollBox) drawFades(dst canvas.Canvas, box image.Rectangle, offset int) {
	r, g, bl, _ := b.background.RGBA()
	width := int(scrollFadeWidth * b.scale)
	step := max(1, width/scrollFadeSteps)
	for i := range scrollFadeSteps {
		// Opaque-ish at the edge, fading toward the content.
		alpha := uint8(255 * (scrollFadeSteps - i) / (scrollFadeSteps + 1))
		c := color.NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), alpha}
		if offset > 0 {
			dst.DrawRect(box.Min.X+i*step, box.Min.Y, step, box.Dy(), c)
		}
		if offset < b.contentWidth()-b.width {
			dst.DrawRect(box.Max.X-(i+1)*step, box.Min.Y, step, box.Dy(), c)
		}
	}
}

// Axis is the direction a region scrolls in.
type Axis int

const (
	Horizontal Axis = iota
	Vertical
)

// ScrollRegion is a part of the document whose content is bigger than
// what it shows, as drawn this frame: where a scrollbar for it would go.
type ScrollRegion struct {
	Source      Block
	Axis        Axis
	Box         image.Rectangle // the whole region, in its Canvas's coordinates
	Visible     image.Rectangle // Box clipped to what was actually drawn
	ContentSize int             // the content's size along Axis
	Offset      int             // how far the content is scrolled, as drawn
}

// MaxOffset is how far r's content can scroll.
func (r ScrollRegion) MaxOffset() int {
	if r.Axis == Vertical {
		return r.ContentSize - r.Box.Dy()
	}
	return r.ContentSize - r.Box.Dx()
}

// ScrollbarFunc draws, or not, the scrollbar of a region as it's drawn
// onto dst at now. Whoever keeps scroll state provides it (the View),
// and decides how a scrollbar looks and behaves; it can also note the
// region, to match input against it later.
type ScrollbarFunc func(dst canvas.Canvas, r ScrollRegion, now time.Duration)
