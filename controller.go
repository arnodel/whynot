package whynot

import (
	"image"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/arnodel/whynot/input"
	"github.com/arnodel/whynot/internal/engine"
)

// controllerMomentumDecayPerSecond/controllerMomentumMinVelocity tune
// post-touch scroll momentum: the fraction of velocity (pixels/second)
// retained after one second, and the speed below which it stops.
const (
	controllerMomentumDecayPerSecond = 0.05
	controllerMomentumMinVelocity    = 30
)

// Controller turns a backend's input events into what they do to a View:
// scrolling, link hovering, sideways-scrolling blocks and their
// scrollbars, touch pans and flings. Feed it each frame's input with
// Frame, which returns the Events that happened, such as a link being
// clicked. It also scrolls on command (ScrollDown, PageDown, ScrollLeft
// and so on), for an app binding keys to them.
type Controller struct {
	view  *View
	scale float64

	// events collects what happens during a Frame, for it to return.
	events []Event

	// lastTick is the time of the last tick; ticked is whether there was
	// one.
	lastTick time.Duration
	ticked   bool

	// pointer is the mouse pointer's last position; hasPointer is false
	// before it's known, and after it leaves.
	pointer    image.Point
	hasPointer bool

	// momentum is the page's coasting velocity after a vertical touch
	// fling; hMomentum that of hTarget, after a sideways one.
	momentum  float64
	hMomentum float64
	hTarget   engine.Block

	// viewMoves is the View's moves count at the end of the last Frame:
	// if it's changed by the next one, the app moved the View, which
	// stops any fling.
	viewMoves uint64

	// lastHScrolled is the sideways-scrolling block last scrolled, pressed
	// or touched: ScrollLeft and ScrollRight's target when the pointer
	// isn't on one.
	lastHScrolled engine.Block

	// The touch in progress: touching is set from its TouchStart to its
	// TouchEnd, even for a touch outside the bounds. Other touches are
	// ignored.
	touching     bool
	touchID      int
	touchPos     image.Point
	touchAxis    touchAxis
	touchTarget  engine.Block // the sideways-scrolling block it started on, if any
	touchPending image.Point
	// touchOnBar is whether the touch is dragging the View's scrollbar.
	touchOnBar bool
}

// touchAxis is what a touch drag scrolls.
type touchAxis int

const (
	touchNone       touchAxis = iota // no touch, or one that started outside the bounds
	touchUndecided                   // on a sideways-scrolling block, not moved far enough to tell
	touchVertical                    // the page
	touchHorizontal                  // touchTarget
)

// touchAxisLockDistance is how far, in logical pixels, a touch starting
// on a sideways-scrolling block moves before its dominant direction
// decides whether it scrolls the block or the page.
const touchAxisLockDistance = 10

// NewController returns a Controller for v, at scale 1. It acts on
// input inside v's Bounds.
func NewController(v *View) *Controller {
	return &Controller{view: v, scale: 1}
}

// View returns the View the Controller drives.
func (c *Controller) View() *View {
	return c.view
}

// SetView makes the Controller drive v instead. Nothing stays hovered in
// the previous View; the next Frame hovers whatever is under the pointer
// in v.
func (c *Controller) SetView(v *View) {
	if c.view != nil {
		c.view.vbar.hovered = false
		c.view.unhover()
	}
	c.view = v
	c.hTarget, c.lastHScrolled = nil, nil
	c.viewMoves = v.moves
	c.cancelMomentum()
}

// bounds is where the View is drawn.
func (c *Controller) bounds() image.Rectangle {
	return c.view.Bounds()
}

// Scale returns the display's scale (see SetScale).
func (c *Controller) Scale() float64 {
	return c.scale
}

// SetScale sets the display's scale: input pixels per logical pixel,
// without any zoom of the document. Command scrolling steps and touch
// thresholds are in logical pixels, so they don't change with the zoom.
func (c *Controller) SetScale(s float64) {
	c.scale = s
}

// Frame applies one frame's input events, in order, at now: elapsed time
// on the clock the View is laid out with (see View.Layout). Call it once
// per frame, with no events if there were none, so flings keep coasting.
//
// It returns what happened that the app may want to react to, such as a
// LinkClick, in order; nil if nothing did. The Controller doesn't act on
// them itself.
//
// A fling stops if the app moves the View itself (with View.ScrollBy,
// ScrollToAnchor and so on) or swaps it with SetView.
func (c *Controller) Frame(events []input.Event, now time.Duration) []Event {
	if c.view.moves != c.viewMoves {
		c.cancelMomentum()
	}
	c.events = nil
	defer func() { c.viewMoves = c.view.moves }()
	var (
		pressed      bool        // a primary press inside the bounds, for a click
		pressedAt    image.Point // where
		wheeled      bool
		touchStarted bool
		touchEnded   bool
		touchDelta   image.Point
	)
	for _, e := range events {
		switch e := e.(type) {
		case input.PointerMove:
			c.pointer, c.hasPointer = image.Pt(e.X, e.Y), true
			if c.view.vbar.dragging {
				c.view.dragScrollbarTo(e.Y)
			} else if s := c.hscroll(); s != nil && s.dragging != nil {
				s.dragTo(e.X)
			}
		case input.PointerLeave:
			c.hasPointer = false
		case input.PointerButton:
			if e.Button != input.ButtonPrimary {
				continue
			}
			p := image.Pt(e.X, e.Y)
			c.pointer, c.hasPointer = p, true
			s := c.hscroll()
			if !e.Down {
				if c.view.vbar.dragging {
					c.view.endScrollbarDrag()
				}
				if s != nil && s.dragging != nil {
					s.endDrag(c.view.ctx.Time)
					s.hover(p, c.view.ctx.Time)
				}
				continue
			}
			c.cancelMomentum()
			if !p.In(c.bounds()) {
				continue
			}
			if c.view.onScrollbar(p) {
				c.view.beginScrollbarDrag(p)
				continue
			}
			if b := c.hscrollBlockAt(p); b != nil {
				c.lastHScrolled = b
			}
			if s != nil && s.beginDrag(p) {
				continue
			}
			pressed, pressedAt = true, p
		case input.Wheel:
			p := image.Pt(e.X, e.Y)
			if !p.In(c.bounds()) {
				continue
			}
			c.cancelMomentum()
			wheeled = true
			dx, dy := e.DX, e.DY
			if e.Mods.Contain(input.ModShift) {
				// Shift+wheel scrolls sideways. Some platforms (macOS)
				// already report it as horizontal, leaving dy 0.
				dx, dy = dx+dy, 0
			}
			c.view.ScrollBy(dy)
			if dx != 0 {
				c.hscrollBy(c.hscrollBlockAt(p), dx)
			}
		case input.TouchStart:
			if c.touching {
				continue
			}
			c.touching, c.touchID, c.touchPos = true, e.ID, image.Pt(e.X, e.Y)
			if c.touchPos.In(c.bounds()) && c.view.onScrollbar(c.touchPos) && c.view.scrollbarOpacity(c.view.ctx.Time) > 0 {
				// A visible scrollbar is dragged by touch too; a hidden one
				// mustn't swallow touches along the edge.
				c.touchOnBar = true
				c.cancelMomentum()
				c.view.beginScrollbarDrag(c.touchPos)
				continue
			}
			touchStarted = true
			c.touchStart(c.touchPos, now)
		case input.TouchMove:
			if c.touching && e.ID == c.touchID {
				p := image.Pt(e.X, e.Y)
				if c.touchOnBar {
					c.view.dragScrollbarTo(p.Y)
				} else {
					touchDelta = touchDelta.Add(p.Sub(c.touchPos))
				}
				c.touchPos = p
			}
		case input.TouchEnd:
			if c.touching && e.ID == c.touchID {
				touchEnded = true
			}
		case input.TouchCancel:
			if c.touching && e.ID == c.touchID {
				touchEnded = true
			}
		}
	}

	switch {
	case c.touchOnBar:
	case c.touching:
		// Also while the finger is held still (no movement), so the
		// velocity decays before release rather than flinging.
		c.touchDrag(touchDelta.X, touchDelta.Y, now)
	case !pressed && !wheeled:
		c.coastMomentum(now)
	}

	if touchStarted && c.touchPos.In(c.bounds()) {
		// A touch only taps: it highlights and clicks the link under it,
		// but hovers nothing else (a block's scrollbar shows only while
		// panning it).
		dest, ok := c.view.hoverLink(c.touchPos.X, c.touchPos.Y)
		if s := c.hscroll(); s != nil {
			s.unhover()
		}
		if ok {
			c.click(dest)
		}
	} else if !c.touching {
		c.hover()
		if pressed {
			if dest, ok := c.view.LinkAt(pressedAt.X, pressedAt.Y); ok {
				c.click(dest)
			}
		}
	}

	if touchEnded {
		c.touchEnd()
	}
	return c.events
}

// hover updates the hover from the pointer's position, unless a
// scrollbar drag owns the pointer.
func (c *Controller) hover() {
	if s := c.hscroll(); c.view.vbar.dragging || s != nil && s.dragging != nil {
		return
	}
	if !c.hasPointer || !c.pointer.In(c.bounds()) {
		c.view.vbar.hovered = false
		c.view.unhover()
		return
	}
	c.view.vbar.hovered = c.view.onScrollbar(c.pointer)
	c.view.hover(c.pointer.X, c.pointer.Y)
}

// click reports a click or tap on the link to dest.
func (c *Controller) click(dest string) {
	if id, ok := strings.CutPrefix(dest, "#"); ok {
		// Ids are matched decoded, as browsers do: "#caf%C3%A9" is the
		// heading "café".
		if decoded, err := url.PathUnescape(id); err == nil {
			id = decoded
		}
		c.events = append(c.events, AnchorClick{ID: id})
		return
	}
	c.events = append(c.events, LinkClick{Destination: dest})
}

// tick returns the seconds elapsed since its last call (0 the first
// time).
func (c *Controller) tick(now time.Duration) float64 {
	dt := 0.0
	if c.ticked {
		dt = (now - c.lastTick).Seconds()
	}
	c.lastTick, c.ticked = now, true
	return dt
}

// touchStart begins a touch drag at p, stopping any fling. A drag
// starting on a block that scrolls sideways locks to whichever direction
// it first clearly moves in, scrolling the block or the page; any other
// drag inside the bounds scrolls the page, and one outside nothing.
func (c *Controller) touchStart(p image.Point, now time.Duration) {
	c.cancelMomentum()
	c.tick(now)
	c.touchPending = image.Point{}
	c.touchTarget = c.hscrollBlockAt(p)
	switch {
	case !p.In(c.bounds()):
		c.touchAxis = touchNone
	case c.touchTarget != nil:
		c.lastHScrolled = c.touchTarget
		c.touchAxis = touchUndecided
	default:
		c.touchAxis = touchVertical
	}
}

// touchDrag applies one frame of a touch drag: dx, dy is how far the
// finger moved since the last frame ("content follows the finger").
func (c *Controller) touchDrag(dx, dy int, now time.Duration) {
	if c.touchAxis == touchUndecided {
		c.touchPending = c.touchPending.Add(image.Pt(dx, dy))
		d := c.touchPending
		if math.Hypot(float64(d.X), float64(d.Y)) < touchAxisLockDistance*c.scale {
			c.tick(now)
			return
		}
		// Apply everything moved so far, along the locked axis.
		dx, dy = d.X, d.Y
		if abs(dx) > abs(dy) {
			c.touchAxis = touchHorizontal
		} else {
			c.touchAxis = touchVertical
		}
	}
	switch c.touchAxis {
	case touchVertical:
		c.view.ScrollBy(-float64(dy))
		c.accumulate(&c.momentum, float64(dy), now)
	case touchHorizontal:
		// Content follows the finger: moving right scrolls towards the
		// start.
		c.hscrollBy(c.touchTarget, -float64(dx))
		c.hTarget = c.touchTarget
		c.accumulate(&c.hMomentum, float64(dx), now)
	}
}

// touchEnd ends the touch drag; a fling keeps coasting. After a sideways
// pan, the block's scrollbar fades out from now. It also clears hover:
// whatever the finger was on mustn't stay hovered after it lifts.
func (c *Controller) touchEnd() {
	if c.touchOnBar {
		c.touchOnBar = false
		c.view.endScrollbarDrag()
	}
	if s := c.hscroll(); s != nil && c.touchAxis == touchHorizontal {
		s.reveal(c.touchTarget, c.view.ctx.Time)
	}
	c.touching = false
	c.touchAxis = touchNone
	c.view.unhover()
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (c *Controller) hscroll() *hscrollState {
	return c.view.hscroll
}

// hscrollBlockAt returns the sideways-scrolling block at p, in the
// input's coordinates, or nil if there's none.
func (c *Controller) hscrollBlockAt(p image.Point) engine.Block {
	s := c.hscroll()
	if s == nil || !p.In(c.bounds()) {
		return nil
	}
	r, ok := s.regionAt(p)
	if !ok {
		return nil
	}
	return r.Source
}

// hscrollTarget is the block ScrollLeft and ScrollRight scroll: the one
// under the pointer, else the one last scrolled, pressed or touched.
func (c *Controller) hscrollTarget() engine.Block {
	if c.hasPointer {
		if b := c.hscrollBlockAt(c.pointer); b != nil {
			return b
		}
	}
	return c.lastHScrolled
}

// hscrollBy scrolls block dx pixels towards its end, showing its
// scrollbar, and remembers it as the last block scrolled. It reports
// whether block is on screen (and so scrolled).
func (c *Controller) hscrollBy(block engine.Block, dx float64) bool {
	s := c.hscroll()
	if s == nil || block == nil || !s.scrollBy(block, dx, c.view.ctx.Time) {
		return false
	}
	c.lastHScrolled = block
	return true
}

// vscrollBy scrolls the View dy pixels towards the end, stopping any
// fling.
func (c *Controller) vscrollBy(dy float64) {
	c.cancelMomentum()
	c.view.ScrollBy(dy)
}

// commandStep is how far ScrollDown, ScrollUp, ScrollLeft and ScrollRight
// move, in logical pixels.
const commandStep = 40

// pageOverlap is the fraction of the page PageDown and PageUp keep in
// view, for continuity.
const pageOverlap = 0.1

// ScrollDown scrolls a step towards the end, as for an arrow key.
func (c *Controller) ScrollDown() { c.vscrollBy(commandStep * c.scale) }

// ScrollUp scrolls a step towards the start, as for an arrow key.
func (c *Controller) ScrollUp() { c.vscrollBy(-commandStep * c.scale) }

// PageDown scrolls a page towards the end.
func (c *Controller) PageDown() { c.vscrollBy(float64(c.bounds().Dy()) * (1 - pageOverlap)) }

// PageUp scrolls a page towards the start.
func (c *Controller) PageUp() { c.vscrollBy(-float64(c.bounds().Dy()) * (1 - pageOverlap)) }

// ScrollLeft scrolls a block wider than the View (a code block or table,
// say) a step towards its start: the block under the pointer, else the
// one last scrolled, clicked or touched, if it's still on screen. It
// reports whether there was such a block.
func (c *Controller) ScrollLeft() bool {
	c.cancelMomentum()
	return c.hscrollBy(c.hscrollTarget(), -commandStep*c.scale)
}

// ScrollRight scrolls a block a step towards its end, like ScrollLeft.
func (c *Controller) ScrollRight() bool {
	c.cancelMomentum()
	return c.hscrollBy(c.hscrollTarget(), commandStep*c.scale)
}

// accumulate blends delta, moved since the last tick, into the coasting
// velocity v.
func (c *Controller) accumulate(v *float64, delta float64, now time.Duration) {
	dt := c.tick(now)
	if dt <= 0 {
		return
	}
	*v = *v*0.5 + delta/dt*0.5
}

// cancelMomentum stops any fling outright.
func (c *Controller) cancelMomentum() {
	c.momentum = 0
	c.hMomentum = 0
}

// moving reports whether there's a velocity fast enough to coast.
func (c *Controller) moving() bool {
	return fastEnough(c.momentum) || fastEnough(c.hMomentum)
}

// Animating reports whether frames must keep coming even without input:
// while a fling coasts, or a scrollbar fades out. A backend whose frames
// are event-driven (like Gio's) must keep requesting frames while it's
// true, or the animation stops dead.
func (c *Controller) Animating() bool {
	s := c.hscroll()
	return c.moving() || s != nil && s.animating(c.view.ctx.Time) || c.view.scrollbarAnimating(c.view.ctx.Time)
}

func fastEnough(v float64) bool {
	return math.Abs(v) >= controllerMomentumMinVelocity
}

// coastMomentum applies one frame of decay to any velocity left over
// from a touch drag that's since ended.
func (c *Controller) coastMomentum(now time.Duration) {
	dt := c.tick(now)
	c.momentum = coast(c.momentum, dt, func(d float64) { c.view.ScrollBy(-d) })
	c.hMomentum = coast(c.hMomentum, dt, func(d float64) { c.hscrollBy(c.hTarget, -d) })
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
	v *= math.Pow(controllerMomentumDecayPerSecond, dt)
	if !fastEnough(v) {
		v = 0
	}
	apply(delta)
	return v
}
