package whynot

import (
	"image"
	"time"

	"github.com/arnodel/whynot/internal/engine"
)

// How long a revealed scrollbar - by scrolling its block, or moving the
// pointer over it - stays fully visible after that, then fades out.
const (
	hscrollRevealHold = 600 * time.Millisecond
	hscrollRevealFade = 300 * time.Millisecond
)

// hscrollState is a View's horizontal scrolling state: each block's
// offset, and the hover, drag and reveal state of their scrollbars. The
// ScrollBoxes laid out for the View draw with it, as their
// engine.HScroller.
type hscrollState struct {
	// offsets holds each scrolled block's offset, keyed by source Block
	// so it survives relayout.
	offsets map[engine.Block]float64

	// areas is where each ScrollBox was drawn by the last View.Draw, in
	// View coordinates (relative to origin, that Draw's x, y), innermost
	// last. Input is matched against these, so a ScrollBox needs no
	// separate hit-testing path.
	areas  []engine.HScrollArea
	origin image.Point

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

var _ engine.HScroller = (*hscrollState)(nil)

func newHScrollState() *hscrollState {
	return &hscrollState{offsets: make(map[engine.Block]float64)}
}

// Offset implements engine.HScroller.
func (s *hscrollState) Offset(source engine.Block) float64 {
	return s.offsets[source]
}

// Drawn implements engine.HScroller, noting an area drawn this frame.
func (s *hscrollState) Drawn(a engine.HScrollArea) {
	a.Box = a.Box.Sub(s.origin)
	a.Visible = a.Visible.Sub(s.origin)
	s.areas = append(s.areas, a)
}

// Bar implements engine.HScroller.
func (s *hscrollState) Bar(source engine.Block, now time.Duration) (opacity float64, hover, pressed bool) {
	pressed = s.dragging == source
	hover = pressed || s.hovered == source && s.barHovered
	return s.barOpacity(source, now), hover, pressed
}

// beginFrame forgets the last frame's areas before a View.Draw at origin.
func (s *hscrollState) beginFrame(origin image.Point) {
	s.areas = s.areas[:0]
	s.origin = origin
}

// areaAt returns the innermost area visible at p.
func (s *hscrollState) areaAt(p image.Point) (engine.HScrollArea, bool) {
	for i := len(s.areas) - 1; i >= 0; i-- {
		if p.In(s.areas[i].Visible) {
			return s.areas[i], true
		}
	}
	return engine.HScrollArea{}, false
}

// areaOf returns source's area from the last frame.
func (s *hscrollState) areaOf(source engine.Block) (engine.HScrollArea, bool) {
	for _, a := range s.areas {
		if a.Source == source {
			return a, true
		}
	}
	return engine.HScrollArea{}, false
}

func (s *hscrollState) offset(a engine.HScrollArea) int {
	return int(a.Clamp(s.offsets[a.Source]))
}

func (s *hscrollState) thumb(a engine.HScrollArea) image.Rectangle {
	return a.Thumb(s.offset(a))
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
	a, ok := s.areaAt(p)
	if !ok {
		s.unhover()
		return
	}
	s.hovered = a.Source
	s.barHovered = p.In(a.BarZone(s.offset(a)))
	if moved {
		s.reveal(a.Source, now)
	}
}

// unhover forgets any hovered box, without revealing anything.
func (s *hscrollState) unhover() {
	s.hovered, s.barHovered = nil, false
}

// scrollAt scrolls the box at p by dx (positive moves the content right,
// revealing its start) at now, reporting whether there was one.
func (s *hscrollState) scrollAt(p image.Point, dx float64, now time.Duration) bool {
	a, ok := s.areaAt(p)
	if !ok {
		return false
	}
	s.offsets[a.Source] = a.Clamp(s.offsets[a.Source] - dx)
	s.reveal(a.Source, now)
	return true
}

// beginDrag starts dragging the scrollbar at p, if there is one.
func (s *hscrollState) beginDrag(p image.Point) bool {
	a, ok := s.areaAt(p)
	if !ok || !p.In(a.BarZone(s.offset(a))) {
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
	a, ok := s.areaOf(s.dragging)
	if !ok {
		return
	}
	thumb := s.thumb(a)
	track := a.Box.Dx() - thumb.Dx()
	if track <= 0 {
		return
	}
	ratio := float64(x-s.grab-a.Box.Min.X) / float64(track)
	s.offsets[a.Source] = a.Clamp(ratio * float64(a.MaxOffset()))
}

// endDrag ends a scrollbar drag at now; the scrollbar then fades out
// like after any other scroll, unless the pointer stays on it.
func (s *hscrollState) endDrag(now time.Duration) {
	s.reveal(s.dragging, now)
	s.dragging = nil
}

// scrollSource scrolls source's box by dx, as scrollAt does, wherever it
// is now - for a touch pan, which sticks to the block it started on.
func (s *hscrollState) scrollSource(source engine.Block, dx float64) {
	if a, ok := s.areaOf(source); ok {
		s.offsets[source] = a.Clamp(s.offsets[source] - dx)
	}
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
