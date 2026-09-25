package whynot

import (
	"image"
	"math"
	"strings"
	"time"
)

// interactionMomentumDecayPerSecond/interactionMomentumMinVelocity tune
// post-touch scroll momentum (see Interaction.Momentum): the fraction of
// velocity (pixels/second) retained after one second, and the speed
// below which it stops.
const (
	interactionMomentumDecayPerSecond = 0.05
	interactionMomentumMinVelocity    = 30
)

// Interaction drives a View from generic pointer input - the backend-
// agnostic core of what a "panel" embedding a View needs (see
// ebitenrenderer.Panel, giorenderer.Panel), so scroll gating, hover/click
// dispatch, and touch-momentum decay are implemented once rather than
// per rendering backend. It deliberately knows nothing about a
// scrollbar - that stays backend-specific, since each backend represents
// one so differently (a hand-drawn rect vs a native widget).
//
// View/Bounds/OnLinkClick/OnLinkHover/AnchorScrolling are plain fields a
// caller keeps in sync with its own equivalents (e.g. every tick, or
// whenever they change) - Interaction has no setter methods to keep
// this a lightweight, low-level helper rather than another layer of
// encapsulation on top of Panel's own.
type Interaction struct {
	View   *View
	Bounds image.Rectangle

	// OnLinkClick/OnLinkHover/AnchorScrolling - see ebitenrenderer.Panel's
	// identical fields/option for what these mean.
	OnLinkClick     func(destination string)
	OnLinkHover     func(destination string)
	AnchorScrolling bool

	hoverDest string
	momentum  float64
	lastTick  time.Time
}

// tick returns real elapsed seconds since the last call (0 on the very
// first), updating the internal clock - shared by AccumulateMomentum and
// Momentum, since both need real time, not tick count, to stay correct
// regardless of frame rate.
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

// AccumulateMomentum blends scrollDelta - this tick's touch-drag amount,
// already in View.Scroll's units - into the coasting velocity Momentum
// applies once the touch ends. Call every tick an active touch drag is
// actually scrolling the document (not, say, dragging a scrollbar
// instead - that's the caller's call, via whether it calls this at all).
func (in *Interaction) AccumulateMomentum(scrollDelta float64, now time.Time) {
	dt := in.tick(now)
	if dt <= 0 {
		return
	}
	velocity := scrollDelta / dt
	in.momentum = in.momentum*0.5 + velocity*0.5
}

// CancelMomentum stops any in-progress coasting outright - call on a
// fresh press, a mouse wheel tick, or a drag that's left Bounds, any of
// which should kill a fling rather than let it keep decaying.
func (in *Interaction) CancelMomentum() {
	in.momentum = 0
}

// Momentum applies one tick of decay to any velocity left over from a
// touch drag that's since ended. Call every tick there's no active
// touch/mouse-wheel input, so a released fling keeps coasting.
func (in *Interaction) Momentum(now time.Time) {
	dt := in.tick(now)
	if in.momentum == 0 || dt <= 0 {
		return
	}
	delta := in.momentum * dt
	in.momentum *= math.Pow(interactionMomentumDecayPerSecond, dt)
	if math.Abs(in.momentum) < interactionMomentumMinVelocity {
		in.momentum = 0
	}
	in.View.Scroll(delta)
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
		dest, hasLink = in.View.Hover(cx-in.Bounds.Min.X, cy-in.Bounds.Min.Y)
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
