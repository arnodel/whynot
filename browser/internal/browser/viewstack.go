package browser

import (
	"image"
	"image/color"
	"math"
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
// link clicked: Event, from the child at index Child.
type StackEvent struct {
	Child int
	Event whynot.Event
}

// A StackController turns input into what a ViewStack does, as a
// whynot.Controller does for a View. The stack scrolls itself, by the
// wheel's vertical part; everything else in a child, such as hovering and
// clicking links and scrolling code blocks sideways, is its own
// Controller's, which only acts within its child's region.
type StackController struct {
	stack       *ViewStack
	controllers []*whynot.Controller
}

// NewStackController returns a StackController for s.
func NewStackController(s *ViewStack) *StackController {
	c := &StackController{stack: s}
	for _, v := range s.views {
		c.controllers = append(c.controllers, whynot.NewController(v))
	}
	return c
}

// Frame handles a frame's input, at now, and returns what the children
// reported. Touch isn't handled yet: it's dropped.
func (c *StackController) Frame(events []input.Event, now time.Duration) []StackEvent {
	var forChildren []input.Event
	for _, e := range events {
		switch e := e.(type) {
		case input.Wheel:
			if !image.Pt(e.X, e.Y).In(c.stack.bounds) {
				continue
			}
			if !e.Mods.Contain(input.ModShift) {
				// Wheel deltas are logical pixels, as a Controller takes
				// them; the vertical part is the stack's.
				c.stack.ScrollBy(e.DY * c.stack.scale())
				e.DY = 0
			}
			forChildren = append(forChildren, e)
		case input.TouchStart, input.TouchMove, input.TouchEnd, input.TouchCancel:
			// Not yet: a drag could be meant for the stack or for a code
			// block, which a Controller decides for one View.
		default:
			forChildren = append(forChildren, e)
		}
	}
	var out []StackEvent
	for i, r := range c.stack.regions {
		if r.Empty() {
			continue
		}
		for _, e := range c.controllers[i].Frame(forChildren, now) {
			out = append(out, StackEvent{Child: i, Event: e})
		}
	}
	return out
}

// Animating reports whether a child is animating, as a scrollbar fading
// out does: a program that draws only when something happens should draw
// another frame.
func (c *StackController) Animating() bool {
	for i, r := range c.stack.regions {
		if !r.Empty() && c.controllers[i].Animating() {
			return true
		}
	}
	return false
}
