package browser

import (
	"image"
	"math"
	"time"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/input"
)

// A selection in the journal can run across pages: a drag from one page
// into another selects, in the page it started in, from where it started
// to that page's end (or start, dragging up), the pages in between whole,
// and in the page the pointer is over, from its start (or end) to the
// pointer. Each page keeps its own selection, which copying joins.

const (
	// stackSelectSlop is how far, in logical pixels, the pointer moves
	// from a press before it's a drag.
	stackSelectSlop = 4
	// stackSelectScrollSpeed is how fast a drag past the stack's top or
	// bottom scrolls it, in logical pixels a second per pixel past it.
	stackSelectScrollSpeed = 12.0
)

// stackPress is the press of the primary button in progress.
type stackPress struct {
	down     bool
	child    int // the child pressed, -1 if none
	at       image.Point
	moved    bool
	pointer  image.Point
	scrolled time.Duration
}

// beginPress starts a press at p.
func (c *StackController) beginPress(p image.Point, now time.Duration) {
	c.press = stackPress{down: true, child: c.childAt(p), at: p, pointer: p, scrolled: now}
}

// movePointer notes the pointer at p.
func (c *StackController) movePointer(p image.Point) {
	c.press.pointer = p
	if c.press.down && math.Hypot(float64(p.X-c.press.at.X), float64(p.Y-c.press.at.Y)) > stackSelectSlop*c.stack.scale() {
		c.press.moved = true
	}
}

// pressedChildEvents returns what the pressed child gets of events: the
// pointer kept within its region, so that its own Controller selects up
// to its edge and doesn't scroll it, the stack doing the scrolling. A
// child off screen gets only the button's release.
func (c *StackController) pressedChildEvents(events []input.Event, r image.Rectangle) []input.Event {
	clamp := func(x, y int) (int, int) {
		return max(r.Min.X, min(x, r.Max.X-1)), max(r.Min.Y, min(y, r.Max.Y-1))
	}
	var out []input.Event
	for _, e := range events {
		switch e := e.(type) {
		case input.PointerMove:
			if r.Empty() {
				continue
			}
			e.X, e.Y = clamp(e.X, e.Y)
			out = append(out, e)
		case input.PointerButton:
			if !r.Empty() {
				e.X, e.Y = clamp(e.X, e.Y)
			}
			out = append(out, e)
		default:
			out = append(out, e)
		}
	}
	return out
}

// carrySelection carries a drag that has left the page it started in on
// into the pages it's over, and clears the selections of pages outside
// it.
func (c *StackController) carrySelection(now time.Duration) {
	dt := (now - c.press.scrolled).Seconds()
	c.press.scrolled = now
	i := c.press.child
	if !c.press.down || !c.press.moved || i < 0 {
		return
	}
	b := c.stack.bounds
	p := c.press.pointer
	if past := pastBounds(p.Y, b); past != 0 {
		c.stack.ScrollBy(float64(past) * stackSelectScrollSpeed * dt)
	}
	q := image.Pt(max(b.Min.X, min(p.X, b.Max.X-1)), max(b.Min.Y, min(p.Y, b.Max.Y-1)))
	j := c.childAt(q)
	if j < 0 {
		return
	}
	views := c.stack.views
	switch {
	case j > i:
		views[i].SelectToEnd()
		for k := i + 1; k < j; k++ {
			views[k].SelectAll()
		}
		views[j].SelectFromStartTo(q.X, q.Y)
	case j < i:
		views[i].SelectToStart()
		for k := j + 1; k < i; k++ {
			views[k].SelectAll()
		}
		views[j].SelectFromEndTo(q.X, q.Y)
	}
	for k, v := range views {
		if k < min(i, j) || k > max(i, j) {
			v.ClearSelection()
		}
	}
}

// selectionScrolling reports whether a drag past the stack's top or bottom
// is scrolling it.
func (c *StackController) selectionScrolling() bool {
	return c.press.down && c.press.moved && c.press.child >= 0 && pastBounds(c.press.pointer.Y, c.stack.bounds) != 0
}

// pastBounds returns how far y is above (negative) or below b, or 0.
func pastBounds(y int, b image.Rectangle) int {
	switch {
	case y < b.Min.Y:
		return y - b.Min.Y
	case y >= b.Max.Y:
		return y - (b.Max.Y - 1)
	}
	return 0
}

// keepOneSelection clears the other pages' selections when a page reports
// a new one of its own, as from a drag within it: unless a drag across
// pages, there's one selection at a time.
func (c *StackController) keepOneSelection(events []StackEvent) {
	if c.press.down && c.press.moved && c.press.child >= 0 && c.childAt(c.press.pointer) != c.press.child {
		return
	}
	for _, e := range events {
		if _, ok := e.Event.(whynot.SelectionChange); ok && e.Child >= 0 && c.stack.views[e.Child].HasSelection() {
			for k, v := range c.stack.views {
				if k != e.Child {
					v.ClearSelection()
				}
			}
		}
	}
}
