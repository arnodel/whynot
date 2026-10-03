package whynot

import (
	"image"
	"time"

	"github.com/arnodel/whynot/canvas"
	"github.com/arnodel/whynot/input"
)

// Panel shows a View in a rectangle of a window and makes it
// interactive: it lays the View out to fit, draws it there, and turns
// input into scrolling, hovering and clicking (through a Controller).
// It keeps its settings when the View is swapped, so an app can show
// one document after another in the same place.
//
// A Panel doesn't depend on a graphics library. Each frame, the app
// passes it the frame's input events (from a backend's input.Source)
// and a canvas.Canvas to draw on, along with the time.
type Panel struct {
	// OnLinkClick is called with a link's destination when it's clicked
	// or tapped; nil makes links inert. See also
	// AnchorScrolling.
	OnLinkClick func(destination string)
	// OnLinkHover is called when the hovered link changes, with "" when
	// none is. The View already highlights it; this is for the app's own
	// reactions, like a status bar.
	OnLinkHover func(destination string)
	// AnchorScrolling makes a clicked "#fragment" link scroll the View to
	// its heading instead of calling OnLinkClick: for a document with no
	// location to resolve a fragment against.
	AnchorScrolling bool

	view       *View
	bounds     image.Rectangle
	scale      float64
	controller *Controller

	// Re-applied to each View the Panel shows.
	styleSheet StyleSheet
	scrollbar  bool

	// now is the time of the last Frame or Draw, so a change between
	// frames can lay the View out on the same clock.
	now time.Duration
}

// NewPanel returns a Panel showing view in bounds, at scale 1.
//
// bounds is in the coordinates of the Canvas the Panel draws on and of
// the input events it's given (see package input).
func NewPanel(view *View, bounds image.Rectangle) *Panel {
	p := &Panel{bounds: bounds, scale: 1, controller: NewController(view, bounds)}
	p.SetView(view)
	return p
}

// View returns the View the Panel shows.
func (p *Panel) View() *View {
	return p.view
}

// SetView shows v instead, with the Panel's StyleSheet and scrollbar,
// if set, and lays it out, so it can be scrolled straight away.
func (p *Panel) SetView(v *View) {
	p.view = v
	if p.styleSheet != nil {
		v.SetStyleSheet(p.styleSheet)
	}
	if p.scrollbar {
		v.SetScrollbar(true)
	}
	p.controller.SetView(v)
	p.relayout()
}

// Bounds returns where the Panel is drawn.
func (p *Panel) Bounds() image.Rectangle {
	return p.bounds
}

// SetBounds moves or resizes the Panel.
func (p *Panel) SetBounds(r image.Rectangle) {
	p.bounds = r
	p.controller.SetBounds(r)
	p.relayout()
}

// Scale returns the scale the View is laid out at.
func (p *Panel) Scale() float64 {
	return p.scale
}

// SetScale sets the scale the View is laid out at: the display's scale
// times any zoom.
func (p *Panel) SetScale(s float64) {
	p.scale = s
	p.relayout()
}

// SetScrollbar shows or hides the View's own scrollbar (see
// WithScrollbar), in the current View and, when on, in every View the
// Panel shows after it.
func (p *Panel) SetScrollbar(on bool) {
	p.scrollbar = on
	p.view.SetScrollbar(on)
}

// SetStyleSheet sets the StyleSheet of the current View and of every
// View the Panel shows after it.
func (p *Panel) SetStyleSheet(s StyleSheet) {
	p.styleSheet = s
	p.view.SetStyleSheet(s)
}

// Frame applies one frame's input events at now, elapsed time on the
// app's clock. Call it once per frame, before Draw, with no events if
// there were none, so flings keep coasting.
func (p *Panel) Frame(events []input.Event, now time.Duration) {
	p.now = now
	p.controller.OnLinkClick = p.OnLinkClick
	p.controller.OnLinkHover = p.OnLinkHover
	p.controller.AnchorScrolling = p.AnchorScrolling
	p.controller.Frame(events, now)
}

// Draw draws the View onto dst, within the Panel's bounds, as it is at
// now.
func (p *Panel) Draw(dst canvas.Canvas, now time.Duration) {
	p.now = now
	// Layout is cheap when nothing changed, and gives animated images
	// the time.
	p.relayout()
	p.view.Draw(dst.Clip(p.bounds), p.bounds.Min.X, p.bounds.Min.Y)
}

// Animating reports whether the Panel needs more frames even without
// input: a fling is coasting, or a scrollbar is fading.
func (p *Panel) Animating() bool {
	return p.controller.Animating()
}

const (
	// arrowScrollLines is how far ScrollDown and ScrollUp move, in
	// logical pixels.
	arrowScrollLines = 40
	// pageOverlapFrac is how much of the page PageDown and PageUp keep
	// in view, for continuity.
	pageOverlapFrac = 0.1
)

// ScrollDown scrolls a little towards the end, as for an arrow key.
func (p *Panel) ScrollDown() { p.view.Scroll(-arrowScrollLines * p.scale) }

// ScrollUp scrolls a little towards the start, as for an arrow key.
func (p *Panel) ScrollUp() { p.view.Scroll(arrowScrollLines * p.scale) }

// ScrollLeft scrolls the block under the mouse pointer a little towards
// its start, if it's wider than the page (a code block or table, say).
// It reports whether there was such a block.
func (p *Panel) ScrollLeft() bool { return p.controller.scrollAtPointer(arrowScrollLines * p.scale) }

// ScrollRight scrolls the block under the mouse pointer a little towards
// its end, like ScrollLeft.
func (p *Panel) ScrollRight() bool { return p.controller.scrollAtPointer(-arrowScrollLines * p.scale) }

// PageDown scrolls a page towards the end.
func (p *Panel) PageDown() { p.view.Scroll(-float64(p.bounds.Dy()) * (1 - pageOverlapFrac)) }

// PageUp scrolls a page towards the start.
func (p *Panel) PageUp() { p.view.Scroll(float64(p.bounds.Dy()) * (1 - pageOverlapFrac)) }

func (p *Panel) relayout() {
	p.view.Layout(p.bounds.Dx(), p.bounds.Dy(), p.scale, p.now)
}
