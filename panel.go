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
	controller *Controller

	// Re-applied to each View the Panel shows.
	bounds     image.Rectangle
	scale      float64
	zoom       float64
	styleSheet StyleSheet
	scrollbar  bool
}

// NewPanel returns a Panel showing view in bounds, at scale 1 and zoom 1.
// bounds is in canvas coordinates (see Coordinates in the package
// documentation).
func NewPanel(view *View, bounds image.Rectangle) *Panel {
	p := &Panel{bounds: bounds, scale: 1, zoom: 1, controller: NewController(view)}
	p.SetView(view)
	return p
}

// View returns the View the Panel shows.
func (p *Panel) View() *View {
	return p.view
}

// SetView shows v instead, with the Panel's bounds, scale, zoom,
// StyleSheet and scrollbar, if set, so it can be scrolled straight away.
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

// Bounds returns where the Panel is drawn on the canvas.
func (p *Panel) Bounds() image.Rectangle {
	return p.bounds
}

// SetBounds moves or resizes the Panel.
func (p *Panel) SetBounds(r image.Rectangle) {
	p.bounds = r
	p.relayout()
}

// Scale returns the display's scale (see SetScale).
func (p *Panel) Scale() float64 {
	return p.scale
}

// SetScale sets the display's scale: canvas pixels per logical pixel.
func (p *Panel) SetScale(s float64) {
	p.scale = s
	p.relayout()
}

// Zoom returns the document's zoom (see SetZoom).
func (p *Panel) Zoom() float64 {
	return p.zoom
}

// SetZoom magnifies the document by zoom (1 is 100%): the View is laid
// out at Scale times Zoom. Scrolling steps don't change with the zoom.
func (p *Panel) SetZoom(zoom float64) {
	p.zoom = zoom
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
	p.controller.OnLinkClick = p.OnLinkClick
	p.controller.OnLinkHover = p.OnLinkHover
	p.controller.AnchorScrolling = p.AnchorScrolling
	p.controller.Frame(events, now)
}

// Draw draws the View onto dst, within the Panel's bounds, as it is at
// now.
func (p *Panel) Draw(dst canvas.Canvas, now time.Duration) {
	p.view.Draw(dst, now)
}

// Animating reports whether the Panel needs more frames even without
// input: a fling is coasting, or a scrollbar is fading.
func (p *Panel) Animating() bool {
	return p.controller.Animating()
}

// ScrollDown scrolls a step towards the end, as for an arrow key.
func (p *Panel) ScrollDown() { p.controller.ScrollDown() }

// ScrollUp scrolls a step towards the start, as for an arrow key.
func (p *Panel) ScrollUp() { p.controller.ScrollUp() }

// ScrollLeft scrolls a block wider than the View a step towards its
// start (see Controller.ScrollLeft), reporting whether there was one.
func (p *Panel) ScrollLeft() bool { return p.controller.ScrollLeft() }

// ScrollRight scrolls a block wider than the View a step towards its
// end, like ScrollLeft.
func (p *Panel) ScrollRight() bool { return p.controller.ScrollRight() }

// PageDown scrolls a page towards the end.
func (p *Panel) PageDown() { p.controller.PageDown() }

// PageUp scrolls a page towards the start.
func (p *Panel) PageUp() { p.controller.PageUp() }

// relayout applies the Panel's geometry to the View and Controller.
func (p *Panel) relayout() {
	p.controller.SetScale(p.scale)
	p.view.SetScale(p.scale * p.zoom)
	p.view.SetBounds(p.bounds)
}
