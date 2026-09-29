package whynot

import (
	"image"
	"image/color"
	"math"
	"time"
)

// Horizontal scrolling for blocks wider than the width they're laid out
// at (see issue #25): a ScrollBox shows a window onto the content,
// shifted by an offset the View keeps per source Block, so scrolling -
// like hovering, which reveals the scrollbar - never needs a relayout.

// Dimensions of a ScrollBox's edge fades and scrollbar, in logical
// (unscaled) pixels.
const (
	hscrollFadeWidth     = 24
	hscrollFadeSteps     = 8
	hscrollBarThickness  = 6
	hscrollBarInset      = 2
	hscrollMinThumbWidth = 24
)

// How long a scrollbar revealed by touch panning (there's no hover on
// touch) stays fully visible after the last movement, then fades out.
const (
	hscrollRevealHold = 600 * time.Millisecond
	hscrollRevealFade = 300 * time.Millisecond
)

// ScrollBox shows a window, width wide, onto content that's wider,
// scrolled horizontally by the offset its View keeps for source. An edge
// fades into the page background wherever there's hidden content, and a
// scrollbar is drawn over the bottom edge while the pointer is over it.
type ScrollBox struct {
	inner  BlockLayout
	width  int
	source Block
	// state is the View's; nil outside a View, where the box never
	// scrolls but still clips.
	state      *hscrollState
	scale      float64
	background color.Color
	styleSheet StyleSheet
}

var _ BlockLayout = (*ScrollBox)(nil)

// scrollIfWider returns inner wrapped in a ScrollBox if it's wider than
// width, and inner itself otherwise.
func scrollIfWider(ctx RenderingContext, source Block, inner BlockLayout, width int) BlockLayout {
	if width <= 0 || inner.Bounds().Dx() <= width {
		return inner
	}
	return &ScrollBox{
		inner:      inner,
		width:      width,
		source:     source,
		state:      ctx.hscroll,
		scale:      ctx.Scale,
		background: ctx.StyleSheet.BackgroundColor(),
		styleSheet: ctx.StyleSheet,
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
	return clampOffset(b.state.offsets[b.source], b.contentWidth()-b.width)
}

func clampOffset(offset float64, max int) int {
	return int(clampOffsetF(offset, max))
}

func clampOffsetF(offset float64, max int) float64 {
	return math.Max(0, math.Min(float64(max), offset))
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

func (b *ScrollBox) drawContents(dst Canvas, x, y int, now time.Duration) {
	box := image.Rect(x, y, x+b.width, y+b.inner.Bounds().Dy())
	offset := b.offset()
	clipped := dst.Clip(box)
	DrawBlockLayout(b.inner, clipped, x-offset, y, now)
	b.drawFades(clipped, box, offset)

	if b.state == nil {
		return
	}
	b.state.record(hscrollArea{
		source:       b.source,
		box:          box,
		visible:      clipped.Bounds(),
		contentWidth: b.contentWidth(),
		scale:        b.scale,
	})
	if opacity := b.state.barOpacity(b.source, now); opacity > 0 {
		thumb := hscrollThumb(box, clipped.Bounds(), b.contentWidth(), offset, b.scale)
		pressed := b.state.dragging == b.source
		c := color.NRGBAModel.Convert(scrollbarColor(b.styleSheet, b.state.barHovered || pressed, pressed)).(color.NRGBA)
		c.A = uint8(float64(c.A) * opacity)
		dst.DrawRect(thumb.Min.X, thumb.Min.Y, thumb.Dx(), thumb.Dy(), c)
	}
}

// drawFades fades each edge of box that has content hidden past it into
// the page background, with stepped transparency.
func (b *ScrollBox) drawFades(dst Canvas, box image.Rectangle, offset int) {
	r, g, bl, _ := b.background.RGBA()
	width := int(hscrollFadeWidth * b.scale)
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

// hscrollThumb is the scrollbar thumb for a ScrollBox drawn at box, of
// which visible is the part actually on screen, showing content
// contentWidth wide scrolled to offset - in the same coordinates as box.
// It sits along the bottom of the visible part, so a box taller than the
// viewport still shows it.
func hscrollThumb(box, visible image.Rectangle, contentWidth, offset int, scale float64) image.Rectangle {
	w := box.Dx()
	thumbWidth := min(w, max(w*w/contentWidth, int(hscrollMinThumbWidth*scale)))
	x := box.Min.X
	if maxOffset := contentWidth - w; maxOffset > 0 {
		x += offset * (w - thumbWidth) / maxOffset
	}
	inset := int(hscrollBarInset * scale)
	bottom := min(box.Max.Y, visible.Max.Y) - inset
	return image.Rect(x, bottom-int(hscrollBarThickness*scale), x+thumbWidth, bottom)
}

// scrollbarColor is the StyleSheet's scrollbar color if it has one (see
// ScrollbarStyleSheet), else a neutral translucent gray.
func scrollbarColor(s StyleSheet, hover, pressed bool) color.Color {
	if ss, ok := s.(ScrollbarStyleSheet); ok {
		return ss.ScrollbarColor(hover, pressed)
	}
	return color.RGBA{0x80, 0x80, 0x80, 0xA0}
}

// hscrollState is a View's horizontal scrolling state, shared with its
// ScrollBoxes through RenderingContext.
type hscrollState struct {
	// offsets holds each scrolled block's offset, keyed by source Block
	// so it survives relayout.
	offsets map[Block]float64

	// areas is where each ScrollBox was drawn by the last View.Draw, in
	// View coordinates (relative to origin, that Draw's x, y), innermost
	// last. Input is matched against these, so a ScrollBox needs no
	// separate hit-testing path.
	areas  []hscrollArea
	origin image.Point

	hovered    Block // whose box is under the pointer, or nil
	barHovered bool  // whether the pointer is on hovered's scrollbar
	dragging   Block // whose scrollbar is being dragged, or nil
	grab       int   // pointer x minus thumb x, while dragging

	// revealed's scrollbar shows until revealUntil (in RenderingContext.
	// Time's clock), fading out at the end - see reveal.
	revealed    Block
	revealUntil time.Duration
}

// hscrollArea is where a ScrollBox was drawn, in View coordinates.
type hscrollArea struct {
	source       Block
	box          image.Rectangle // the whole box
	visible      image.Rectangle // box clipped to what was actually drawn
	contentWidth int
	scale        float64
}

func newHScrollState() *hscrollState {
	return &hscrollState{offsets: make(map[Block]float64)}
}

// beginFrame forgets the last frame's areas before a View.Draw at origin.
func (s *hscrollState) beginFrame(origin image.Point) {
	s.areas = s.areas[:0]
	s.origin = origin
}

// record notes an area drawn this frame, given in the Canvas's
// coordinates.
func (s *hscrollState) record(a hscrollArea) {
	a.box = a.box.Sub(s.origin)
	a.visible = a.visible.Sub(s.origin)
	s.areas = append(s.areas, a)
}

// areaAt returns the innermost area visible at p.
func (s *hscrollState) areaAt(p image.Point) (hscrollArea, bool) {
	for i := len(s.areas) - 1; i >= 0; i-- {
		if p.In(s.areas[i].visible) {
			return s.areas[i], true
		}
	}
	return hscrollArea{}, false
}

// areaOf returns source's area from the last frame.
func (s *hscrollState) areaOf(source Block) (hscrollArea, bool) {
	for _, a := range s.areas {
		if a.source == source {
			return a, true
		}
	}
	return hscrollArea{}, false
}

func (s *hscrollState) offset(a hscrollArea) int {
	return clampOffset(s.offsets[a.source], a.contentWidth-a.box.Dx())
}

func (s *hscrollState) thumb(a hscrollArea) image.Rectangle {
	return hscrollThumb(a.box, a.visible, a.contentWidth, s.offset(a), a.scale)
}

// barZone is the strip along the bottom of a's box that counts as its
// scrollbar for the pointer: the thumb's track, with a little slack.
func (s *hscrollState) barZone(a hscrollArea) image.Rectangle {
	thumb := s.thumb(a)
	return image.Rect(a.box.Min.X, thumb.Min.Y-int(hscrollBarInset*a.scale), a.box.Max.X, a.visible.Max.Y)
}

// hover updates which box, and whether its scrollbar, is under p. The
// box being dragged stays hovered until the drag ends.
func (s *hscrollState) hover(p image.Point) {
	if s.dragging != nil {
		return
	}
	a, ok := s.areaAt(p)
	if !ok {
		s.hovered, s.barHovered = nil, false
		return
	}
	s.hovered = a.source
	s.barHovered = p.In(s.barZone(a))
}

// scrollAt scrolls the box at p by dx (positive moves the content right,
// revealing its start), reporting whether there was one.
func (s *hscrollState) scrollAt(p image.Point, dx float64) bool {
	a, ok := s.areaAt(p)
	if !ok {
		return false
	}
	s.offsets[a.source] = clampOffsetF(s.offsets[a.source]-dx, a.contentWidth-a.box.Dx())
	return true
}

// beginDrag starts dragging the scrollbar at p, if there is one.
func (s *hscrollState) beginDrag(p image.Point) bool {
	a, ok := s.areaAt(p)
	if !ok || !p.In(s.barZone(a)) {
		return false
	}
	thumb := s.thumb(a)
	if p.X < thumb.Min.X || p.X >= thumb.Max.X {
		// Pressed on the track, not the thumb: grab the thumb's middle
		// there, so it jumps under the pointer.
		s.grab = thumb.Dx() / 2
	} else {
		s.grab = p.X - thumb.Min.X
	}
	s.dragging, s.hovered, s.barHovered = a.source, a.source, true
	s.dragTo(p.X)
	return true
}

// dragTo moves the dragged scrollbar's thumb to follow pointer x.
func (s *hscrollState) dragTo(x int) {
	a, ok := s.areaOf(s.dragging)
	if !ok {
		return
	}
	thumb := s.thumb(a)
	track := a.box.Dx() - thumb.Dx()
	if track <= 0 {
		return
	}
	ratio := float64(x-s.grab-a.box.Min.X) / float64(track)
	maxOffset := a.contentWidth - a.box.Dx()
	s.offsets[a.source] = clampOffsetF(ratio*float64(maxOffset), maxOffset)
}

func (s *hscrollState) endDrag() {
	s.dragging = nil
}

// scrollSource scrolls source's box by dx, as scrollAt does, wherever it
// is now - for a touch pan, which sticks to the block it started on.
func (s *hscrollState) scrollSource(source Block, dx float64) {
	if a, ok := s.areaOf(source); ok {
		s.offsets[source] = clampOffsetF(s.offsets[source]-dx, a.contentWidth-a.box.Dx())
	}
}

// reveal shows source's scrollbar from now, for touch panning, where
// there's no hover to show it.
func (s *hscrollState) reveal(source Block, now time.Duration) {
	s.revealed = source
	s.revealUntil = now + hscrollRevealHold + hscrollRevealFade
}

// barOpacity is how visible source's scrollbar is at now, from 0
// (hidden) to 1.
func (s *hscrollState) barOpacity(source Block, now time.Duration) float64 {
	switch {
	case s.hovered == source || s.dragging == source:
		return 1
	case s.revealed != source || now >= s.revealUntil:
		return 0
	}
	return min(1, float64(s.revealUntil-now)/float64(hscrollRevealFade))
}

// animating reports whether a revealed scrollbar is still showing at
// now, so frames must keep coming for it to fade out.
func (s *hscrollState) animating(now time.Duration) bool {
	return s.revealed != nil && now < s.revealUntil
}
