package whynot

import (
	"image"
	"math"
	"strings"
	"time"

	"github.com/arnodel/whynot/internal/engine"
)

// interactionMomentumDecayPerSecond/interactionMomentumMinVelocity tune
// post-touch scroll momentum (see Interaction.Momentum): the fraction of
// velocity (pixels/second) retained after one second, and the speed
// below which it stops.
const (
	interactionMomentumDecayPerSecond = 0.05
	interactionMomentumMinVelocity    = 30
)

// Interaction drives a View from pointer and touch input in screen
// coordinates, so that each backend doesn't reimplement it: scrolling,
// link hover and clicks, sideways-scrolling blocks and their scrollbars,
// and touch flings. The View's own vertical scrollbar is the backend's.
//
// Set the fields, and keep View and Bounds up to date, before feeding it
// events.
type Interaction struct {
	View   *View
	Bounds image.Rectangle

	// OnLinkClick is called with a link's destination when it's clicked,
	// OnLinkHover when the hovered link changes ("" when none). With
	// AnchorScrolling, a "#fragment" link scrolls the View to that
	// heading instead of calling OnLinkClick.
	OnLinkClick     func(destination string)
	OnLinkHover     func(destination string)
	AnchorScrolling bool

	hoverDest string
	lastTick  time.Time

	// momentum is the page's coasting velocity after a vertical touch
	// fling; hMomentum that of hTarget, after a sideways one.
	momentum  float64
	hMomentum float64
	hTarget   engine.Block

	// The touch drag in progress - see TouchStart. touching is set from
	// TouchStart to TouchEnd, even for a touch outside Bounds.
	touching     bool
	touchAxis    touchAxis
	touchTarget  engine.Block // the sideways-scrolling block it started on, if any
	touchPending image.Point
}

// touchAxis is what a touch drag scrolls.
type touchAxis int

const (
	touchNone       touchAxis = iota // no touch, or one that started outside Bounds
	touchUndecided                   // on a sideways-scrolling block, not moved far enough to tell
	touchVertical                    // the page
	touchHorizontal                  // touchTarget
)

// touchAxisLockDistance is how far, in logical pixels, a touch starting
// on a sideways-scrolling block moves before its dominant direction
// decides whether it scrolls the block or the page.
const touchAxisLockDistance = 10

// tick returns the seconds elapsed since its last call (0 the first
// time): flings use real time, so they don't depend on the frame rate.
func (in *Interaction) tick(now time.Time) float64 {
	dt := 0.0
	if !in.lastTick.IsZero() {
		dt = now.Sub(in.lastTick).Seconds()
	}
	in.lastTick = now
	return dt
}

// Scroll applies scrollDelta (already in View.Scroll's own units) if
// cx, cy is within Bounds - e.g. one mouse wheel tick.
func (in *Interaction) Scroll(cx, cy int, scrollDelta float64) {
	if image.Pt(cx, cy).In(in.Bounds) {
		in.View.Scroll(scrollDelta)
	}
}

// TouchStart begins a touch drag at cx, cy, stopping any fling. A drag
// starting on a block that scrolls sideways locks to whichever direction
// it first clearly moves in, scrolling the block or the page; any other
// drag inside Bounds scrolls the page, and one outside Bounds nothing.
func (in *Interaction) TouchStart(cx, cy int, now time.Time) {
	in.touching = true
	in.CancelMomentum()
	in.tick(now)
	in.touchPending = image.Point{}
	in.touchTarget = nil
	p := image.Pt(cx, cy)
	switch {
	case !p.In(in.Bounds):
		in.touchAxis = touchNone
	case in.hscroll() != nil:
		if a, ok := in.hscroll().regionAt(p.Sub(in.Bounds.Min)); ok {
			in.touchTarget = a.Source
			in.touchAxis = touchUndecided
			return
		}
		fallthrough
	default:
		in.touchAxis = touchVertical
	}
}

// TouchDrag applies one frame of the touch drag begun by TouchStart: dx,
// dy is how far the finger moved since the last frame ("content follows
// the finger": positive moves content right/down). Call every frame the
// touch is down, even without movement, so a finger held still before
// lifting leaves no fling.
func (in *Interaction) TouchDrag(dx, dy int, now time.Time) {
	if in.touchAxis == touchUndecided {
		in.touchPending = in.touchPending.Add(image.Pt(dx, dy))
		d := in.touchPending
		if math.Hypot(float64(d.X), float64(d.Y)) < touchAxisLockDistance*in.View.ctx.Scale {
			in.tick(now)
			return
		}
		// Apply everything moved so far, along the locked axis.
		dx, dy = d.X, d.Y
		if abs(dx) > abs(dy) {
			in.touchAxis = touchHorizontal
		} else {
			in.touchAxis = touchVertical
		}
	}
	switch in.touchAxis {
	case touchVertical:
		in.View.Scroll(float64(dy))
		in.accumulate(&in.momentum, float64(dy), now)
	case touchHorizontal:
		in.scrollTarget(in.touchTarget, float64(dx))
		in.hTarget = in.touchTarget
		in.accumulate(&in.hMomentum, float64(dx), now)
	}
}

// TouchEnd ends the touch drag; a fling keeps coasting (see Momentum).
// After a sideways pan, the block's scrollbar fades out from now. It
// also clears hover: there's no hover on touch, so whatever the finger
// was on mustn't stay hovered - a link highlighted - after it lifts.
func (in *Interaction) TouchEnd() {
	if s := in.hscroll(); s != nil && in.touchAxis == touchHorizontal {
		s.reveal(in.touchTarget, in.View.ctx.Time)
	}
	in.touching = false
	in.touchAxis = touchNone
	in.View.Hover(-1, -1)
	if in.hoverDest != "" {
		in.hoverDest = ""
		if in.OnLinkHover != nil {
			in.OnLinkHover("")
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (in *Interaction) hscroll() *hscrollState {
	return in.View.hscroll
}

// scrollTarget scrolls the sideways-scrolling block target by dx, showing
// its scrollbar (there's no hover on touch).
func (in *Interaction) scrollTarget(target engine.Block, dx float64) {
	if s := in.hscroll(); s != nil && target != nil {
		s.scrollSource(target, dx)
		s.reveal(target, in.View.ctx.Time)
	}
}

// accumulate blends delta, moved since the last tick, into the coasting
// velocity v.
func (in *Interaction) accumulate(v *float64, delta float64, now time.Time) {
	dt := in.tick(now)
	if dt <= 0 {
		return
	}
	*v = *v*0.5 + delta/dt*0.5
}

// CancelMomentum stops any in-progress coasting outright - call on a
// fresh press, a mouse wheel tick, or a drag that's left Bounds, any of
// which should kill a fling rather than let it keep decaying.
func (in *Interaction) CancelMomentum() {
	in.momentum = 0
	in.hMomentum = 0
}

// Moving reports whether there's a velocity fast enough for Momentum to
// act on.
func (in *Interaction) Moving() bool {
	return fastEnough(in.momentum) || fastEnough(in.hMomentum)
}

// Animating reports whether frames must keep coming even without input:
// while there's a fling to coast (see Momentum), or a scrollbar fading
// out. A caller whose frames are event-driven (like Gio's) must keep
// requesting frames while it's true, or the animation stops dead.
func (in *Interaction) Animating() bool {
	s := in.hscroll()
	return in.Moving() || s != nil && s.animating(in.View.ctx.Time)
}

func fastEnough(v float64) bool {
	return math.Abs(v) >= interactionMomentumMinVelocity
}

// Momentum applies one tick of decay to any velocity left over from a
// touch drag that's since ended. Call every tick there's no active
// touch/mouse-wheel input, so a released fling keeps coasting.
func (in *Interaction) Momentum(now time.Time) {
	dt := in.tick(now)
	in.momentum = coast(in.momentum, dt, in.View.Scroll)
	in.hMomentum = coast(in.hMomentum, dt, func(d float64) { in.scrollTarget(in.hTarget, d) })
}

// coast applies dt seconds of velocity v through apply, and returns v
// decayed over that time - 0 once it's too slow to act on.
func coast(v, dt float64, apply func(float64)) float64 {
	if !fastEnough(v) {
		// Dropped before applying any of it: a slow velocity left over a
		// long gap between calls would otherwise turn into a jump.
		return 0
	}
	if dt <= 0 {
		return v
	}
	delta := v * dt
	v *= math.Pow(interactionMomentumDecayPerSecond, dt)
	if !fastEnough(v) {
		v = 0
	}
	apply(delta)
	return v
}

// Reset clears hover state that shouldn't carry over from whatever View
// this Interaction was previously driving - call when swapping to a
// different View.
func (in *Interaction) Reset() {
	in.hoverDest = ""
}

// HoverAndClick updates hover state (calling OnLinkHover on change) and
// dispatches OnLinkClick - or ScrollToAnchor, if AnchorScrolling and the
// destination is a "#fragment" - when justPressed lands on a link.
func (in *Interaction) HoverAndClick(cx, cy int, justPressed bool) {
	cursor := image.Pt(cx, cy)
	var dest string
	var hasLink bool
	if cursor.In(in.Bounds) {
		if in.touching {
			// Touch has no hover, only taps on links: a block's
			// scrollbar shows only while panning it sideways (see
			// TouchDrag), not for being under the finger.
			dest, hasLink = in.View.hoverLink(cx-in.Bounds.Min.X, cy-in.Bounds.Min.Y)
			if s := in.hscroll(); s != nil {
				s.unhover()
			}
		} else {
			dest, hasLink = in.View.Hover(cx-in.Bounds.Min.X, cy-in.Bounds.Min.Y)
		}
	} else {
		// (-1, -1) can't land on anything - only ever clears a
		// highlight left over from moving off a link while still
		// inside Bounds.
		in.View.Hover(-1, -1)
	}

	newHoverDest := ""
	if hasLink {
		newHoverDest = dest
	}
	if in.OnLinkHover != nil && newHoverDest != in.hoverDest {
		in.OnLinkHover(newHoverDest)
	}
	in.hoverDest = newHoverDest

	if hasLink && justPressed {
		if in.AnchorScrolling && strings.HasPrefix(dest, "#") {
			in.View.ScrollToAnchor(strings.TrimPrefix(dest, "#"))
		} else if in.OnLinkClick != nil {
			in.OnLinkClick(dest)
		}
	}
}

// ScrollHorizontal scrolls the block at cx, cy sideways by dx, if it's
// wider than the View - see View.ScrollHorizontal.
func (in *Interaction) ScrollHorizontal(cx, cy int, dx float64) {
	if dx != 0 && image.Pt(cx, cy).In(in.Bounds) {
		in.View.ScrollHorizontal(cx-in.Bounds.Min.X, cy-in.Bounds.Min.Y, dx)
	}
}

// DragHorizontalScrollbar handles pressing, dragging and releasing the
// scrollbar of a block that scrolls sideways, reporting whether the
// pointer event is part of such a drag - in which case the caller
// shouldn't also treat it as a hover or click on the document. down is
// whether the pointer is pressed; justPressed whether it was pressed
// this frame.
func (in *Interaction) DragHorizontalScrollbar(cx, cy int, down, justPressed bool) bool {
	s := in.View.hscroll
	if s == nil {
		return false
	}
	p := image.Pt(cx-in.Bounds.Min.X, cy-in.Bounds.Min.Y)
	switch {
	case s.dragging != nil && !down:
		s.endDrag(in.View.ctx.Time)
		s.hover(p, in.View.ctx.Time)
		return true
	case s.dragging != nil:
		s.dragTo(p.X)
		return true
	case justPressed && image.Pt(cx, cy).In(in.Bounds):
		return s.beginDrag(p)
	}
	return false
}
