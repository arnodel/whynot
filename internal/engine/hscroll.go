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

// How long a revealed scrollbar - by scrolling its block, or moving the
// pointer over it - stays fully visible after that, then fades out.
const (
	HScrollRevealHold = 600 * time.Millisecond
	HScrollRevealFade = 300 * time.Millisecond
)

// ScrollBox shows a window, width wide, onto content that's wider,
// scrolled horizontally by the offset its View keeps for source. An edge
// fades into the page background wherever there's hidden content, and a
// scrollbar is drawn over the bottom edge while it's being scrolled or the
// pointer moves over it, fading out once idle (see hscrollState.
// barOpacity).
type ScrollBox struct {
	inner  BlockLayout
	width  int
	source Block
	// state is the View's; nil outside a View, where the box never
	// scrolls but still clips.
	state      *HScrollState
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

func (b *ScrollBox) drawContents(dst canvas.Canvas, x, y int, now time.Duration) {
	box := image.Rect(x, y, x+b.width, y+b.inner.Bounds().Dy())
	offset := b.offset()
	clipped := dst.Clip(box)
	DrawBlockLayout(b.inner, clipped, x-offset, y, now)
	b.drawFades(clipped, box, offset)

	if b.state == nil {
		return
	}
	b.state.record(HScrollArea{
		Source:       b.source,
		Box:          box,
		Visible:      clipped.Bounds(),
		ContentWidth: b.contentWidth(),
		scale:        b.scale,
	})
	if opacity := b.state.BarOpacity(b.source, now); opacity > 0 {
		thumb := hscrollThumb(box, clipped.Bounds(), b.contentWidth(), offset, b.scale)
		pressed := b.state.Dragging == b.source
		c := color.NRGBAModel.Convert(b.styles.ScrollbarColor(b.state.barHovered || pressed, pressed)).(color.NRGBA)
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

// HScrollState is a View's horizontal scrolling state, shared with its
// ScrollBoxes through Context.
type HScrollState struct {
	// offsets holds each scrolled block's offset, keyed by source Block
	// so it survives relayout.
	offsets map[Block]float64

	// Areas is where each ScrollBox was drawn by the last View.Draw, in
	// View coordinates (relative to origin, that Draw's x, y), innermost
	// last. Input is matched against these, so a ScrollBox needs no
	// separate hit-testing path.
	Areas  []HScrollArea
	origin image.Point

	hovered    Block       // whose box is under the pointer, or nil
	barHovered bool        // whether the pointer is on hovered's scrollbar
	pointer    image.Point // where hover last saw the pointer
	Dragging   Block       // whose scrollbar is being dragged, or nil
	grab       int         // pointer x minus thumb x, while dragging

	// revealed's scrollbar shows until revealUntil (in Context.
	// Time's clock), fading out at the end - see reveal.
	revealed    Block
	revealUntil time.Duration
}

// HScrollArea is where a ScrollBox was drawn, in View coordinates.
type HScrollArea struct {
	Source       Block
	Box          image.Rectangle // the whole box
	Visible      image.Rectangle // box clipped to what was actually drawn
	ContentWidth int
	scale        float64
}

func NewHScrollState() *HScrollState {
	return &HScrollState{offsets: make(map[Block]float64)}
}

// BeginFrame forgets the last frame's areas before a View.Draw at origin.
func (s *HScrollState) BeginFrame(origin image.Point) {
	s.Areas = s.Areas[:0]
	s.origin = origin
}

// record notes an area drawn this frame, given in the Canvas's
// coordinates.
func (s *HScrollState) record(a HScrollArea) {
	a.Box = a.Box.Sub(s.origin)
	a.Visible = a.Visible.Sub(s.origin)
	s.Areas = append(s.Areas, a)
}

// AreaAt returns the innermost area visible at p.
func (s *HScrollState) AreaAt(p image.Point) (HScrollArea, bool) {
	for i := len(s.Areas) - 1; i >= 0; i-- {
		if p.In(s.Areas[i].Visible) {
			return s.Areas[i], true
		}
	}
	return HScrollArea{}, false
}

// areaOf returns source's area from the last frame.
func (s *HScrollState) areaOf(source Block) (HScrollArea, bool) {
	for _, a := range s.Areas {
		if a.Source == source {
			return a, true
		}
	}
	return HScrollArea{}, false
}

func (s *HScrollState) offset(a HScrollArea) int {
	return clampOffset(s.offsets[a.Source], a.ContentWidth-a.Box.Dx())
}

func (s *HScrollState) Thumb(a HScrollArea) image.Rectangle {
	return hscrollThumb(a.Box, a.Visible, a.ContentWidth, s.offset(a), a.scale)
}

// barZone is the strip along the bottom of a's box that counts as its
// scrollbar for the pointer: the thumb's track, with a little slack.
func (s *HScrollState) barZone(a HScrollArea) image.Rectangle {
	thumb := s.Thumb(a)
	return image.Rect(a.Box.Min.X, thumb.Min.Y-int(hscrollBarInset*a.scale), a.Box.Max.X, a.Visible.Max.Y)
}

// Hover updates which box, and whether its scrollbar, is under p at now.
// Moving over a box reveals its scrollbar; a resting pointer lets it fade.
// The box being dragged stays hovered until the drag ends.
func (s *HScrollState) Hover(p image.Point, now time.Duration) {
	moved := p != s.pointer
	s.pointer = p
	if s.Dragging != nil {
		return
	}
	a, ok := s.AreaAt(p)
	if !ok {
		s.Unhover()
		return
	}
	s.hovered = a.Source
	s.barHovered = p.In(s.barZone(a))
	if moved {
		s.Reveal(a.Source, now)
	}
}

// Unhover forgets any hovered box, without revealing anything.
func (s *HScrollState) Unhover() {
	s.hovered, s.barHovered = nil, false
}

// ScrollAt scrolls the box at p by dx (positive moves the content right,
// revealing its start) at now, reporting whether there was one.
func (s *HScrollState) ScrollAt(p image.Point, dx float64, now time.Duration) bool {
	a, ok := s.AreaAt(p)
	if !ok {
		return false
	}
	s.offsets[a.Source] = clampOffsetF(s.offsets[a.Source]-dx, a.ContentWidth-a.Box.Dx())
	s.Reveal(a.Source, now)
	return true
}

// BeginDrag starts dragging the scrollbar at p, if there is one.
func (s *HScrollState) BeginDrag(p image.Point) bool {
	a, ok := s.AreaAt(p)
	if !ok || !p.In(s.barZone(a)) {
		return false
	}
	thumb := s.Thumb(a)
	if p.X < thumb.Min.X || p.X >= thumb.Max.X {
		// Pressed on the track, not the thumb: grab the thumb's middle
		// there, so it jumps under the pointer.
		s.grab = thumb.Dx() / 2
	} else {
		s.grab = p.X - thumb.Min.X
	}
	s.Dragging, s.hovered, s.barHovered = a.Source, a.Source, true
	s.DragTo(p.X)
	return true
}

// DragTo moves the dragged scrollbar's thumb to follow pointer x.
func (s *HScrollState) DragTo(x int) {
	a, ok := s.areaOf(s.Dragging)
	if !ok {
		return
	}
	thumb := s.Thumb(a)
	track := a.Box.Dx() - thumb.Dx()
	if track <= 0 {
		return
	}
	ratio := float64(x-s.grab-a.Box.Min.X) / float64(track)
	maxOffset := a.ContentWidth - a.Box.Dx()
	s.offsets[a.Source] = clampOffsetF(ratio*float64(maxOffset), maxOffset)
}

// EndDrag ends a scrollbar drag at now; the scrollbar then fades out
// like after any other scroll, unless the pointer stays on it.
func (s *HScrollState) EndDrag(now time.Duration) {
	s.Reveal(s.Dragging, now)
	s.Dragging = nil
}

// ScrollSource scrolls source's box by dx, as scrollAt does, wherever it
// is now - for a touch pan, which sticks to the block it started on.
func (s *HScrollState) ScrollSource(source Block, dx float64) {
	if a, ok := s.areaOf(source); ok {
		s.offsets[source] = clampOffsetF(s.offsets[source]-dx, a.ContentWidth-a.Box.Dx())
	}
}

// Reveal shows source's scrollbar from now, fading out after
// hscrollRevealHold.
func (s *HScrollState) Reveal(source Block, now time.Duration) {
	s.revealed = source
	s.revealUntil = now + HScrollRevealHold + HScrollRevealFade
}

// BarOpacity is how visible source's scrollbar is at now, from 0
// (hidden) to 1: fully while being dragged or pointed at, and otherwise
// for a while after it's revealed (see reveal).
func (s *HScrollState) BarOpacity(source Block, now time.Duration) float64 {
	switch {
	case s.Dragging == source, s.hovered == source && s.barHovered:
		return 1
	case s.revealed != source || now >= s.revealUntil:
		return 0
	}
	return min(1, float64(s.revealUntil-now)/float64(HScrollRevealFade))
}

// Animating reports whether a revealed scrollbar is still showing at
// now, so frames must keep coming for it to fade out.
func (s *HScrollState) Animating(now time.Duration) bool {
	return s.revealed != nil && now < s.revealUntil
}
