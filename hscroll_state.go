package whynot

import (
	"image"
	"image/color"
	"time"

	"github.com/arnodel/whynot/canvas"
	"github.com/arnodel/whynot/internal/engine"
	"github.com/arnodel/whynot/internal/styling"
)

// How long a revealed scrollbar - by scrolling its block, or moving the
// pointer over it - stays fully visible after that, then fades out.
const (
	hscrollRevealHold = 600 * time.Millisecond
	hscrollRevealFade = 300 * time.Millisecond
)

// barGeometry is a style's scrollbar geometry at the View's scale, in
// pixels.
type barGeometry struct {
	thickness, inset, minThumb int
	alwaysVisible              bool
}

func scaledBar(g styling.ScrollbarGeometry, scale float64) barGeometry {
	return barGeometry{
		thickness:     int(g.Thickness * scale),
		inset:         int(g.Inset * scale),
		minThumb:      int(g.MinThumbLength * scale),
		alwaysVisible: g.AlwaysVisible,
	}
}

// drawnRegion is a scroll region as drawn by the last View.Draw, in View
// coordinates, with the scrollbar geometry it was drawn with.
type drawnRegion struct {
	engine.ScrollRegion
	bar barGeometry
}

// hscrollState is a View's horizontal scrolling state: each block's
// offset, and the hover, drag and reveal state of their scrollbars. It
// provides the engine's ScrollOffset and Scrollbar hooks, so it also
// decides how those scrollbars look.
type hscrollState struct {
	// offsets holds each scrolled block's offset, keyed by source Block
	// so it survives relayout.
	offsets map[engine.Block]float64

	// regions is where each ScrollBox was drawn by the last View.Draw, in
	// View coordinates (relative to origin, that Draw's x, y), innermost
	// last. Input is matched against these, so a ScrollBox needs no
	// separate hit-testing path.
	regions []drawnRegion
	origin  image.Point

	hovered    engine.Block // whose box is under the pointer, or nil
	barHovered bool         // whether the pointer is on hovered's scrollbar
	pointer    image.Point  // where hover last saw the pointer
	dragging   engine.Block // whose scrollbar is being dragged, or nil
	grab       int          // pointer x minus thumb x, while dragging

	// revealed's scrollbar shows until revealUntil (on the clock of
	// View.Layout's now), fading out at the end - see reveal.
	revealed    engine.Block
	revealUntil time.Duration
}

func newHScrollState() *hscrollState {
	return &hscrollState{offsets: make(map[engine.Block]float64)}
}

// scrollOffset is the engine's ScrollOffset hook: only sideways
// scrolling is kept here.
func (s *hscrollState) scrollOffset(source engine.Block, axis engine.Axis) float64 {
	if axis != engine.Horizontal {
		return 0
	}
	return s.offsets[source]
}

// drawScrollbar is the engine's Scrollbar hook, for a region drawn with
// styles at scale: it notes the region for input, and draws its thumb as
// visible as barOpacity says.
func (s *hscrollState) drawScrollbar(dst canvas.Canvas, r engine.ScrollRegion, now time.Duration, scale float64, styles styling.Styles) {
	bar := scaledBar(styles.ScrollbarGeometry(), scale)
	thumb := regionThumb(r, r.Offset, bar)
	r.Box = r.Box.Sub(s.origin)
	r.Visible = r.Visible.Sub(s.origin)
	s.regions = append(s.regions, drawnRegion{r, bar})

	opacity := s.barOpacity(r.Source, now)
	if bar.alwaysVisible {
		opacity = 1
	}
	if opacity <= 0 {
		return
	}
	pressed := s.dragging == r.Source
	hover := pressed || s.hovered == r.Source && s.barHovered
	c := color.NRGBAModel.Convert(styles.ScrollbarColor(hover, pressed)).(color.NRGBA)
	c.A = uint8(float64(c.A) * opacity)
	dst.DrawRect(thumb.Min.X, thumb.Min.Y, thumb.Dx(), thumb.Dy(), c)
}

// regionThumb is the scrollbar thumb of r with its content scrolled to
// offset. It sits along the bottom of the visible part, so a box taller
// than the viewport still shows it.
func regionThumb(r engine.ScrollRegion, offset int, bar barGeometry) image.Rectangle {
	w := r.Box.Dx()
	thumbWidth := min(w, max(w*w/r.ContentSize, bar.minThumb))
	x := r.Box.Min.X
	if maxOffset := r.MaxOffset(); maxOffset > 0 {
		x += offset * (w - thumbWidth) / maxOffset
	}
	bottom := min(r.Box.Max.Y, r.Visible.Max.Y) - bar.inset
	return image.Rect(x, bottom-bar.thickness, x+thumbWidth, bottom)
}

// beginFrame forgets the last frame's regions before a View.Draw at
// origin.
func (s *hscrollState) beginFrame(origin image.Point) {
	s.regions = s.regions[:0]
	s.origin = origin
}

// regionAt returns the innermost region visible at p.
func (s *hscrollState) regionAt(p image.Point) (drawnRegion, bool) {
	for i := len(s.regions) - 1; i >= 0; i-- {
		if p.In(s.regions[i].Visible) {
			return s.regions[i], true
		}
	}
	return drawnRegion{}, false
}

// regionOf returns source's region from the last frame.
func (s *hscrollState) regionOf(source engine.Block) (drawnRegion, bool) {
	for _, r := range s.regions {
		if r.Source == source {
			return r, true
		}
	}
	return drawnRegion{}, false
}

// clamp limits offset to what r's content allows.
func clamp(r drawnRegion, offset float64) float64 {
	return max(0, min(float64(r.MaxOffset()), offset))
}

// offset is r's current offset, which input may have changed since r
// was drawn.
func (s *hscrollState) offset(r drawnRegion) int {
	return int(clamp(r, s.offsets[r.Source]))
}

func (s *hscrollState) thumb(r drawnRegion) image.Rectangle {
	return regionThumb(r.ScrollRegion, s.offset(r), r.bar)
}

// barZone is the strip along the bottom of r's box that counts as its
// scrollbar for the pointer: the thumb's track, with a little slack.
func (s *hscrollState) barZone(r drawnRegion) image.Rectangle {
	thumb := s.thumb(r)
	return image.Rect(r.Box.Min.X, thumb.Min.Y-r.bar.inset, r.Box.Max.X, r.Visible.Max.Y)
}

// hover updates which box, and whether its scrollbar, is under p at now.
// Moving over a box reveals its scrollbar; a resting pointer lets it fade.
// The box being dragged stays hovered until the drag ends.
func (s *hscrollState) hover(p image.Point, now time.Duration) {
	moved := p != s.pointer
	s.pointer = p
	if s.dragging != nil {
		return
	}
	a, ok := s.regionAt(p)
	if !ok {
		s.unhover()
		return
	}
	s.hovered = a.Source
	s.barHovered = p.In(s.barZone(a))
	if moved {
		s.reveal(a.Source, now)
	}
}

// unhover forgets any hovered box, without revealing anything.
func (s *hscrollState) unhover() {
	s.hovered, s.barHovered = nil, false
}

// beginDrag starts dragging the scrollbar at p, if there is one.
func (s *hscrollState) beginDrag(p image.Point) bool {
	a, ok := s.regionAt(p)
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
	s.dragging, s.hovered, s.barHovered = a.Source, a.Source, true
	s.dragTo(p.X)
	return true
}

// dragTo moves the dragged scrollbar's thumb to follow pointer x.
func (s *hscrollState) dragTo(x int) {
	a, ok := s.regionOf(s.dragging)
	if !ok {
		return
	}
	thumb := s.thumb(a)
	track := a.Box.Dx() - thumb.Dx()
	if track <= 0 {
		return
	}
	ratio := float64(x-s.grab-a.Box.Min.X) / float64(track)
	s.offsets[a.Source] = clamp(a, ratio*float64(a.MaxOffset()))
}

// endDrag ends a scrollbar drag at now; the scrollbar then fades out
// like after any other scroll, unless the pointer stays on it.
func (s *hscrollState) endDrag(now time.Duration) {
	s.reveal(s.dragging, now)
	s.dragging = nil
}

// scrollBy scrolls source's box dx pixels towards its end (towards its
// start if dx is negative), revealing its scrollbar at now. It reports
// whether the box was drawn by the last View.Draw: if not, nothing
// happens.
func (s *hscrollState) scrollBy(source engine.Block, dx float64, now time.Duration) bool {
	a, ok := s.regionOf(source)
	if !ok {
		return false
	}
	s.offsets[source] = clamp(a, s.offsets[source]+dx)
	s.reveal(source, now)
	return true
}

// reveal shows source's scrollbar from now, fading out after
// hscrollRevealHold.
func (s *hscrollState) reveal(source engine.Block, now time.Duration) {
	s.revealed = source
	s.revealUntil = now + hscrollRevealHold + hscrollRevealFade
}

// barOpacity is how visible source's scrollbar is at now, from 0
// (hidden) to 1: fully while being dragged or pointed at, and otherwise
// for a while after it's revealed (see reveal).
func (s *hscrollState) barOpacity(source engine.Block, now time.Duration) float64 {
	switch {
	case s.dragging == source, s.hovered == source && s.barHovered:
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
