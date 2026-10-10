package browser

import (
	"image"
	"image/color"
	"math"
	"slices"
	"time"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/canvas"
	"github.com/arnodel/whynot/input"
)

// A ViewStack shows Views one above the other, scrolled as one: the pages
// of journal mode, each in its own View, with its own document, state and
// links.
//
// Each child is laid out at the stack's width. Each frame, the stack gives
// each visible child the part of its bounds the child occupies, and
// scrolls it so that the right part of its content shows there.
//
// The stack's position is the child at its top, and that child's own
// scroll position. Scrolling moves it, carrying on into the next or the
// previous child past an end, so it walks real, laid-out content, as a
// View's own scrolling does: it never depends on estimated heights. A
// child above the top can grow, as a page still arriving does, without
// moving what's shown.
//
// A line across the stack's full width separates each child from the
// next, midway between their own margins, so that it's part of neither.
//
// It uses only the Views' public API; input goes through a StackController.
type ViewStack struct {
	views  []*whynot.View
	bounds image.Rectangle
	// top is the child at the top of the stack, scrolled to the stack's
	// position; the children below it are shown from their start.
	top int
	// regions is where each child was drawn by the last Draw: empty for
	// one that wasn't.
	regions []image.Rectangle
	// background fills the stack below its last child; separator is the
	// color of the lines between children.
	background, separator color.Color
}

// NewViewStack returns a ViewStack of views, at its start, drawn with the
// given colors (see SetColors). It shows nothing until SetBounds is
// called.
func NewViewStack(background, separator color.Color, views ...*whynot.View) *ViewStack {
	return &ViewStack{views: views, regions: make([]image.Rectangle, len(views)), background: background, separator: separator}
}

// SetColors sets the colors the stack draws with itself: background,
// below its last child, and separator, for the lines between children.
func (s *ViewStack) SetColors(background, separator color.Color) {
	s.background, s.separator = background, separator
}

// Views returns the stack's children.
func (s *ViewStack) Views() []*whynot.View { return s.views }

// Regions returns where each child was drawn by the last Draw: empty for
// one that wasn't.
func (s *ViewStack) Regions() []image.Rectangle { return s.regions }

// SetBounds sets where the stack is drawn on the canvas. Its children are
// laid out at its width.
func (s *ViewStack) SetBounds(r image.Rectangle) {
	s.bounds = r
	for _, v := range s.views {
		v.SetBounds(r) // lays out at r's width; Draw gives each its part
	}
	s.clampEnd()
}

// toStart and toEnd scroll v to its very start, or its very end, at its
// top.
func toStart(v *whynot.View) { v.ScrollToRatio(0) }

func toEnd(v *whynot.View) {
	v.ScrollToEnd()             // the end at the bottom of v's bounds
	v.ScrollWithin(math.Inf(1)) // and on, to its top
}

// pixels is limit as DocumentBounds takes it.
func pixels(limit float64) int {
	if limit >= math.MaxInt32 {
		return math.MaxInt32
	}
	return max(0, int(math.Ceil(limit)))
}

// below returns how much of v's document is below v's top, exact up to
// limit.
func below(v *whynot.View, limit float64) float64 {
	b := v.Bounds()
	return float64(v.DocumentBounds(pixels(limit-float64(b.Dy()))).Max.Y - b.Min.Y)
}

// above returns how much of v's document is above v's top, exact up to
// limit.
func above(v *whynot.View, limit float64) float64 {
	b := v.Bounds()
	return float64(b.Min.Y - v.DocumentBounds(pixels(limit)).Min.Y)
}

// ScrollBy moves the stack's position dy pixels towards its end (towards
// its start if dy is negative), stopping when its end reaches the bottom
// of the stack, or its start the top.
func (s *ViewStack) ScrollBy(dy float64) {
	if len(s.views) == 0 {
		return
	}
	s.move(dy)
	s.clampEnd()
}

// move moves the position by dy, carrying what's left past a child's end
// on into the next or previous child, and stopping at the stack's very
// start or end.
func (s *ViewStack) move(dy float64) {
	for {
		dy = s.views[s.top].ScrollWithin(dy)
		switch {
		case dy >= 0 && s.top < len(s.views)-1 && below(s.views[s.top], 0) <= 0:
			// At the very end of a child, or past it: the next one's
			// start is the same place.
			s.top++
			toStart(s.views[s.top])
		case dy < 0 && s.top > 0:
			s.top--
			toEnd(s.views[s.top])
		default:
			return
		}
	}
}

// clampEnd keeps the stack's end from rising above its bottom.
func (s *ViewStack) clampEnd() {
	if len(s.views) == 0 {
		return
	}
	h := float64(s.bounds.Dy())
	filled := 0.0
	for i := s.top; i < len(s.views) && filled < h; i++ {
		if i != s.top {
			toStart(s.views[i])
		}
		filled += below(s.views[i], h-filled)
	}
	if filled < h {
		s.ScrollToEnd()
	}
}

// ScrollToEnd puts the end of the last child at the bottom of the stack,
// or the start of the first at the top if they're shorter than it.
func (s *ViewStack) ScrollToEnd() {
	if len(s.views) == 0 {
		return
	}
	s.top = len(s.views) - 1
	toEnd(s.views[s.top])
	s.move(-float64(s.bounds.Dy()))
}

// Position returns how far the stack is scrolled: its top child, and how
// much of that child is above the top of the stack.
func (s *ViewStack) Position() (child int, offset float64) {
	if len(s.views) == 0 {
		return 0, 0
	}
	return s.top, above(s.views[s.top], math.Inf(1))
}

// offsetOf returns how far below the top of the stack child i starts:
// negative if it starts above.
func (s *ViewStack) offsetOf(i int) float64 {
	top := s.views[s.top]
	if i <= s.top {
		y := -above(top, math.Inf(1))
		for j := s.top - 1; j >= i; j-- {
			toStart(s.views[j])
			y -= below(s.views[j], math.Inf(1))
		}
		return y
	}
	y := below(top, math.Inf(1))
	for j := s.top + 1; j < i; j++ {
		toStart(s.views[j])
		y += below(s.views[j], math.Inf(1))
	}
	return y
}

// Reveal scrolls the least it takes to show child i: down until its end
// is at the bottom of the stack, but never so far that its start goes
// above the top, or up to its start if that's above the top.
func (s *ViewStack) Reveal(i int) {
	if i < 0 || i >= len(s.views) {
		return
	}
	h := float64(s.bounds.Dy())
	top := s.offsetOf(i)
	if top < 0 {
		s.ScrollBy(top)
		return
	}
	// The scroll is at most top, so the child's height matters only up to
	// the stack's.
	if i != s.top {
		toStart(s.views[i])
	}
	if bottom := top + below(s.views[i], h); bottom > h {
		s.ScrollBy(min(bottom-h, top))
	}
}

// Draw draws the stack: the top child from its scroll position, then the
// next ones from their start, each in the part of the stack it occupies.
func (s *ViewStack) Draw(dst canvas.Canvas, now time.Duration) {
	for i := range s.regions {
		s.regions[i] = image.Rectangle{}
	}
	y := s.bounds.Min.Y
	for i := s.top; i < len(s.views) && y < s.bounds.Max.Y; i++ {
		v := s.views[i]
		if i != s.top {
			toStart(v)
		}
		h := below(v, float64(s.bounds.Max.Y-y))
		bottom := min(s.bounds.Max.Y, y+int(math.Ceil(h)))
		r := image.Rect(s.bounds.Min.X, y, s.bounds.Max.X, bottom)
		v.SetBounds(r) // the same width: a change of height is cheap
		v.Draw(dst, now)
		s.regions[i] = r
		y = bottom
	}
	clip := dst.Clip(s.bounds)
	if y < s.bounds.Max.Y {
		clip.DrawRect(s.bounds.Min.X, y, s.bounds.Dx(), s.bounds.Max.Y-y, s.background)
	}
	// Drawn last, over the children's backgrounds.
	thickness := max(1, int(math.Round(s.scale())))
	for i := 1; i < len(s.regions); i++ {
		if above, below := s.regions[i-1], s.regions[i]; !above.Empty() && !below.Empty() {
			clip.DrawRect(s.bounds.Min.X, below.Min.Y-thickness/2, s.bounds.Dx(), thickness, s.separator)
		}
	}
}

// scale is the display's scale: the children's.
func (s *ViewStack) scale() float64 {
	if len(s.views) == 0 {
		return 1
	}
	return s.views[0].Scale()
}

// A StackEvent is something a child of a ViewStack reported, such as a
// link clicked: Event, from the child at index Child. A whynot.Scroll,
// the reader scrolling the stack, has Child -1.
type StackEvent struct {
	Child int
	Event whynot.Event
}

// Touch flings, as a whynot.Controller does them: the fraction of a
// fling's velocity (pixels a second) left after a second, and the speed
// below which it stops.
const (
	stackMomentumDecayPerSecond = 0.05
	stackMomentumMinVelocity    = 30
)

// stackAxisLockDistance is how far, in logical pixels, a touch moves
// before its main direction decides whether it scrolls the stack or a
// child sideways.
const stackAxisLockDistance = 10

// A StackController turns input into what a ViewStack does, as a
// whynot.Controller does for a View. The stack scrolls itself: by the
// wheel's vertical part, and by touch drags that are mostly vertical,
// with a fling when the finger lifts. Everything else in a child, such
// as hovering and clicking links, and scrolling code blocks sideways, is
// its own Controller's, which only acts within its child's region.
//
// A touch drag waits until it has moved stackAxisLockDistance to decide:
// a View's Controller only waits on a block that scrolls sideways, but
// the stack can't tell where those are.
type StackController struct {
	stack       *ViewStack
	controllers []*whynot.Controller

	// The touch in progress, from its TouchStart to its TouchEnd: its
	// id, where it started and last was, the child it started on (-1 if
	// none), and what it scrolls (axis), once it has moved far enough to
	// tell.
	touching   bool
	touchID    int
	touchStart image.Point
	touchPos   image.Point
	touchChild int
	axis       touchAxis

	// momentum is the stack's fling velocity, in pixels a second; tick
	// is the time of the last frame that moved or coasted it.
	momentum float64
	tick     time.Duration
	ticked   bool

	// position is the stack's position at the end of the last Frame: if
	// it's changed by the next one, the app moved the stack, which stops
	// a fling.
	position stackPosition
}

// touchAxis is what a touch drag scrolls.
type touchAxis int

const (
	axisUndecided touchAxis = iota // not moved far enough to tell
	axisVertical                   // the stack
	axisSideways                   // the child it started on
)

// stackPosition is a cheap snapshot of a stack's position, to tell
// whether it moved.
type stackPosition struct {
	top    int
	scroll whynot.ScrollPosition
}

func (s *ViewStack) position() stackPosition {
	if len(s.views) == 0 {
		return stackPosition{}
	}
	return stackPosition{s.top, s.views[s.top].ScrollPosition()}
}

// NewStackController returns a StackController for s.
func NewStackController(s *ViewStack) *StackController {
	c := &StackController{stack: s, touchChild: -1}
	for _, v := range s.views {
		c.controllers = append(c.controllers, whynot.NewController(v))
	}
	c.position = s.position()
	return c
}

// Frame handles a frame's input, at now, and returns what happened: what
// the children reported, and a whynot.Scroll if the input scrolled the
// stack. Call it once a frame, with no events if there were none, so
// flings keep coasting. A fling stops if the app moves the stack.
func (c *StackController) Frame(events []input.Event, now time.Duration) []StackEvent {
	if c.stack.position() != c.position {
		c.momentum = 0
	}
	start := c.stack.position()
	scale := c.stack.scale()
	// forTouched is what the child the touch started on gets of it, on top
	// of forChildren.
	var forChildren, forTouched []input.Event
	touchEnded := false
	dragY := 0
	for _, e := range events {
		switch e := e.(type) {
		case input.Wheel:
			if !image.Pt(e.X, e.Y).In(c.stack.bounds) {
				continue
			}
			c.momentum = 0
			if !e.Mods.Contain(input.ModShift) {
				// Wheel deltas are logical pixels, as a Controller takes
				// them; the vertical part is the stack's.
				c.stack.ScrollBy(e.DY * scale)
				e.DY = 0
			}
			forChildren = append(forChildren, e)
		case input.PointerButton:
			if e.Down {
				c.momentum = 0
			}
			forChildren = append(forChildren, e)
		case input.TouchStart:
			p := image.Pt(e.X, e.Y)
			if c.touching || !p.In(c.stack.bounds) {
				continue
			}
			c.touching, c.touchID, c.touchStart, c.touchPos = true, e.ID, p, p
			c.axis, c.momentum = axisUndecided, 0
			c.touchChild = c.childAt(p)
			c.advance(now)
			forTouched = append(forTouched, e)
		case input.TouchMove:
			if !c.touching || e.ID != c.touchID {
				continue
			}
			p := image.Pt(e.X, e.Y)
			if c.axis == axisUndecided {
				d := p.Sub(c.touchStart)
				if math.Hypot(float64(d.X), float64(d.Y)) < stackAxisLockDistance*scale {
					continue
				}
				if absInt(d.X) > absInt(d.Y) {
					c.axis = axisSideways
				} else {
					c.axis = axisVertical
				}
			}
			switch c.axis {
			case axisVertical:
				// Content follows the finger, including what it moved
				// before the lock.
				dy := p.Y - c.touchPos.Y
				c.stack.ScrollBy(-float64(dy))
				dragY += dy
			case axisSideways:
				// The child sees a purely sideways drag.
				forTouched = append(forTouched, input.TouchMove{ID: e.ID, X: e.X, Y: c.touchStart.Y})
			}
			c.touchPos = p
		case input.TouchEnd:
			if c.touching && e.ID == c.touchID {
				forTouched, touchEnded = append(forTouched, e), true
			}
		case input.TouchCancel:
			if c.touching && e.ID == c.touchID {
				forTouched, touchEnded = append(forTouched, e), true
			}
		default:
			forChildren = append(forChildren, e)
		}
	}

	switch {
	case c.touching && c.axis == axisVertical:
		// Also while the finger is held still, so the velocity decays
		// before it lifts rather than flinging.
		c.accumulate(float64(dragY), now)
	case c.touching:
		c.advance(now)
	default:
		c.coast(now)
	}
	if touchEnded {
		c.touching = false
	}

	var out []StackEvent
	for i, r := range c.stack.regions {
		in := forChildren
		switch {
		case i == c.touchChild:
			// Even off screen, until its touch ends.
			in = append(slices.Clone(forChildren), forTouched...)
		case r.Empty():
			continue
		}
		for _, e := range c.controllers[i].Frame(in, now) {
			if _, ok := e.(whynot.Scroll); ok {
				continue // a child doesn't scroll the stack: only the stack does
			}
			out = append(out, StackEvent{Child: i, Event: e})
		}
	}
	if touchEnded {
		c.touchChild = -1
	}
	if c.position = c.stack.position(); c.position != start {
		out = append(out, StackEvent{Child: -1, Event: whynot.Scroll{}})
	}
	return out
}

// childAt returns the index of the child drawn at p by the last Draw, or
// -1 if none was.
func (c *StackController) childAt(p image.Point) int {
	for i, r := range c.stack.regions {
		if p.In(r) {
			return i
		}
	}
	return -1
}

// advance returns the seconds since the last frame it was called in (0
// the first time).
func (c *StackController) advance(now time.Duration) float64 {
	dt := 0.0
	if c.ticked {
		dt = (now - c.tick).Seconds()
	}
	c.tick, c.ticked = now, true
	return dt
}

// accumulate blends delta, how far the finger moved since the last frame,
// into the fling's velocity.
func (c *StackController) accumulate(delta float64, now time.Duration) {
	if dt := c.advance(now); dt > 0 {
		c.momentum = c.momentum*0.5 + delta/dt*0.5
	}
}

// coast applies a frame of the fling, if there's one, decaying it.
func (c *StackController) coast(now time.Duration) {
	dt := c.advance(now)
	if !c.flinging() {
		// Dropped before applying any of it: a slow velocity left over a
		// long gap between frames would otherwise turn into a jump.
		c.momentum = 0
		return
	}
	if dt <= 0 {
		return
	}
	d := c.momentum * dt
	c.momentum *= math.Pow(stackMomentumDecayPerSecond, dt)
	if !c.flinging() {
		c.momentum = 0
	}
	c.stack.ScrollBy(-d)
}

// flinging reports whether the fling is fast enough to coast.
func (c *StackController) flinging() bool {
	return math.Abs(c.momentum) >= stackMomentumMinVelocity
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Animating reports whether frames must keep coming even without input:
// while a fling coasts, or a child animates, as a scrollbar fading out
// does. A program that draws only when something happens should draw
// another frame.
func (c *StackController) Animating() bool {
	if c.flinging() {
		return true
	}
	for i, r := range c.stack.regions {
		if (!r.Empty() || i == c.touchChild) && c.controllers[i].Animating() {
			return true
		}
	}
	return false
}
