package engine

import (
	"image"
	"image/color"
	"math"
	"time"

	"github.com/arnodel/whynot/canvas"
	"github.com/arnodel/whynot/internal/styling"
)

// Horizontal scrolling for blocks wider than the width they're laid out
// at (see issue #25): a ScrollBox shows a window onto the content,
// shifted by an offset the View keeps per source Block, so scrolling -
// like hovering, which reveals the scrollbar - never needs a relayout.

// Dimensions of a ScrollBox's edge fades and scrollbar, in logical
// (unscaled) pixels.
const (
	HScrollFadeWidth     = 24
	hscrollFadeSteps     = 8
	hscrollBarThickness  = 6
	hscrollBarInset      = 2
	hscrollMinThumbWidth = 24
)

// ScrollBox shows a window, width wide, onto content that's wider,
// scrolled horizontally by the offset its View keeps for source. An edge
// fades into the page background wherever there's hidden content, and a
// scrollbar is drawn over the bottom edge as its HScroller says (see
// HScroller.Bar).
type ScrollBox struct {
	inner  BlockLayout
	width  int
	source Block
	// state is the View's; nil outside a View, where the box never
	// scrolls but still clips.
	state      HScroller
	scale      float64
	background color.Color
	styles     styling.Styles
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
		state:      ctx.HScroll,
		scale:      ctx.Scale,
		background: ctx.Styles.BackgroundColor(),
		styles:     ctx.Styles,
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
	if b.state == nil {
		return 0
	}
	return int(math.Max(0, math.Min(float64(b.contentWidth()-b.width), b.state.Offset(b.source))))
}

// area describes b drawn at box, with visible the part actually drawn.
func (b *ScrollBox) area(box, visible image.Rectangle) HScrollArea {
	return HScrollArea{
		Source:       b.source,
		Box:          box,
		Visible:      visible,
		ContentWidth: b.contentWidth(),
		Scale:        b.scale,
	}
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

	if b.state == nil {
		return
	}
	area := b.area(box, clipped.Bounds())
	b.state.Drawn(area)
	if opacity, hover, pressed := b.state.Bar(b.source, now); opacity > 0 {
		thumb := area.Thumb(offset)
		c := color.NRGBAModel.Convert(b.styles.ScrollbarColor(hover || pressed, pressed)).(color.NRGBA)
		c.A = uint8(float64(c.A) * opacity)
		dst.DrawRect(thumb.Min.X, thumb.Min.Y, thumb.Dx(), thumb.Dy(), c)
	}
}

// drawFades fades each edge of box that has content hidden past it into
// the page background, with stepped transparency.
func (b *ScrollBox) drawFades(dst canvas.Canvas, box image.Rectangle, offset int) {
	r, g, bl, _ := b.background.RGBA()
	width := int(HScrollFadeWidth * b.scale)
	step := max(1, width/hscrollFadeSteps)
	for i := range hscrollFadeSteps {
		// Opaque-ish at the edge, fading toward the content.
		alpha := uint8(255 * (hscrollFadeSteps - i) / (hscrollFadeSteps + 1))
		c := color.NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), alpha}
		if offset > 0 {
			dst.DrawRect(box.Min.X+i*step, box.Min.Y, step, box.Dy(), c)
		}
		if offset < b.contentWidth()-b.width {
			dst.DrawRect(box.Max.X-(i+1)*step, box.Min.Y, step, box.Dy(), c)
		}
	}
}

// HScroller is the sideways-scroll state a ScrollBox is drawn with:
// which way each block is scrolled, and how its scrollbar shows. It's
// interaction state, so the View keeps it; a nil HScroller in a Context
// means boxes never scroll, but still clip.
type HScroller interface {
	// Offset is how far source's content is scrolled, unclamped: a
	// ScrollBox clamps it to its current content.
	Offset(source Block) float64
	// Drawn records where a ScrollBox was drawn this frame, in its
	// Canvas's coordinates, so input can be matched against it.
	Drawn(area HScrollArea)
	// Bar is how source's scrollbar shows at now: its opacity, from 0
	// (hidden) to 1, and whether it's hovered or pressed.
	Bar(source Block, now time.Duration) (opacity float64, hover, pressed bool)
}

// HScrollArea is where a ScrollBox was drawn, and the geometry of its
// scrollbar there.
type HScrollArea struct {
	Source       Block
	Box          image.Rectangle // the whole box
	Visible      image.Rectangle // Box clipped to what was actually drawn
	ContentWidth int
	Scale        float64
}

// MaxOffset is how far a's content can scroll.
func (a HScrollArea) MaxOffset() int {
	return a.ContentWidth - a.Box.Dx()
}

// Clamp limits offset to what a's content allows.
func (a HScrollArea) Clamp(offset float64) float64 {
	return math.Max(0, math.Min(float64(a.MaxOffset()), offset))
}

// Thumb is the scrollbar thumb with the content scrolled to offset. It
// sits along the bottom of the visible part, so a box taller than the
// viewport still shows it.
func (a HScrollArea) Thumb(offset int) image.Rectangle {
	w := a.Box.Dx()
	thumbWidth := min(w, max(w*w/a.ContentWidth, int(hscrollMinThumbWidth*a.Scale)))
	x := a.Box.Min.X
	if maxOffset := a.MaxOffset(); maxOffset > 0 {
		x += offset * (w - thumbWidth) / maxOffset
	}
	inset := int(hscrollBarInset * a.Scale)
	bottom := min(a.Box.Max.Y, a.Visible.Max.Y) - inset
	return image.Rect(x, bottom-int(hscrollBarThickness*a.Scale), x+thumbWidth, bottom)
}

// BarZone is the strip along the bottom of a's box that counts as its
// scrollbar for the pointer: the thumb's track, with a little slack.
func (a HScrollArea) BarZone(offset int) image.Rectangle {
	thumb := a.Thumb(offset)
	return image.Rect(a.Box.Min.X, thumb.Min.Y-int(hscrollBarInset*a.Scale), a.Box.Max.X, a.Visible.Max.Y)
}
