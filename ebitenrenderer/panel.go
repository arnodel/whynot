package ebitenrenderer

import (
	"image"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
)

// Panel adapts a whynot.View for embedding in a larger ebiten game: it
// owns what cmd/whynot otherwise hand-rolls in its own game loop -
// coordinate translation, hover/click detection, wheel scroll gated on
// its own bounds, optional scrollbar drag - so any ebiten.Game can drop
// a scrollable Markdown panel into part of its own window without
// reimplementing any of it. Deliberately ebiten-only: View itself stays
// engine-agnostic, but a game embedding a Panel is never going to swap
// ebiten out from under it, so another abstraction layer here buys
// nothing.
//
// view, bounds, and scale are private, each with a paired getter/setter
// - every one of them has a real side effect when it changes (re-layout,
// resetting stale per-view UI state, keeping the scrollbar's remembered
// StyleSheet in sync), so a plain public field would let a caller
// bypass that by direct mutation.
type Panel struct {
	Renderer *Renderer

	// OnLinkClick is called with a clicked link's raw destination text -
	// not resolved against anything, since Panel has no "current
	// location" to resolve a relative link against (unlike cmd/whynot's
	// navigate.go). nil means links are inert. Not called for a
	// "#fragment" destination when WithAnchorScrolling handles it
	// instead.
	OnLinkClick func(destination string)
	// OnLinkHover fires whenever the hovered link changes ("" when
	// nothing's hovered). In-document hover styling already happens
	// automatically via View.Hover - this is only for a host's own
	// reactions outside the document (status bar text, cursor shape).
	OnLinkHover func(destination string)

	view   *whynot.View
	bounds image.Rectangle // screen-space rect, same absolute space as ebiten.CursorPosition()/Draw's dst
	scale  float64         // zoom/DPI scale passed to View.Layout

	// styleSheet is the last StyleSheet given to SetStyleSheet (or nil,
	// if never called) - remembered so SetView can re-apply it to a
	// newly-swapped-in View, and so scrollbarColor has something to
	// read (View has no way to report its current StyleSheet back out).
	styleSheet whynot.StyleSheet

	scrollbarEnabled bool
	anchorScrolling  bool
	start            time.Time // Panel's own elapsed-time clock, see elapsed()

	// interaction is the backend-agnostic scroll/hover/click/momentum
	// core (see whynot.Interaction) - its View/Bounds/OnLinkClick/
	// OnLinkHover/AnchorScrolling fields are kept in sync with this
	// Panel's own at the top of every Update, since those can change
	// (SetView, SetBounds, or direct assignment to OnLinkClick/
	// OnLinkHover) between calls.
	interaction whynot.Interaction

	// Scrollbar drag state, mirroring cmd/whynot's own (grabRatio is a
	// fraction of thumb height, not an absolute offset, since the
	// thumb's height can itself shrink mid-drag). Unused unless
	// WithScrollbar is set.
	draggingScrollbar  bool
	scrollbarGrabRatio float64
	scrollbarState     buttonState

	// Touch state - see touchInput. At most one touch tracked at a time.
	trackingTouch bool
	activeTouch   ebiten.TouchID
}

// buttonState is the scrollbar thumb's per-frame hover/pressed state,
// driving its drawn color - mirrors cmd/whynot's own type of the same
// name and shape.
type buttonState struct{ hover, pressed bool }

// PanelOption customizes a Panel at construction, via NewPanel's opts
// parameter.
type PanelOption func(*Panel)

// WithScrollbar enables Panel's own scrollbar thumb along its bounds'
// right edge, draggable - omitted by default so a panel that never
// needs one pays no cost tracking drag state or hit-testing a rect
// that was never drawn. Colour/width aren't configured here - they come
// from the View's own StyleSheet (ScrollbarStyleSheet) and
// ScaledViewMargins, exactly as cmd/whynot's scrollbar already does.
func WithScrollbar() PanelOption {
	return func(p *Panel) { p.scrollbarEnabled = true }
}

// WithAnchorScrolling makes a clicked "#fragment" link scroll there
// directly (View.ScrollToAnchor) instead of invoking OnLinkClick - for
// a self-contained document (e.g. a table of contents) where the host
// has no fuller URL to resolve a fragment against.
func WithAnchorScrolling() PanelOption {
	return func(p *Panel) { p.anchorScrolling = true }
}

// WithStyleSheet is equivalent to calling SetStyleSheet right after
// NewPanel - a convenience for a host that knows its StyleSheet upfront.
func WithStyleSheet(s whynot.StyleSheet) PanelOption {
	return func(p *Panel) { p.SetStyleSheet(s) }
}

// NewPanel constructs a Panel drawing view into bounds (screen-space,
// see Bounds) via renderer. scale starts at 1; call SetScale to change
// it (e.g. to match a host's own zoom/DPI).
func NewPanel(view *whynot.View, renderer *Renderer, bounds image.Rectangle, opts ...PanelOption) *Panel {
	p := &Panel{
		Renderer: renderer,
		view:     view,
		bounds:   bounds,
		scale:    1,
		start:    time.Now(),
	}
	for _, opt := range opts {
		opt(p)
	}
	p.relayout()
	return p
}

// View returns the View this Panel currently draws/scrolls - e.g. for a
// host implementing its own back/forward history (as cmd/whynot does)
// to capture/restore ScrollPosition, or to call a View method Panel
// doesn't wrap directly (Title, ...).
func (p *Panel) View() *whynot.View {
	return p.view
}

// SetView swaps the View this Panel draws/scrolls - e.g. navigating to
// a different document (cmd/whynot's own cross-document jump keeps a
// separate *View per document and swaps which one's current). Resets
// scrollbar-drag and hover state (meaningless carried over from the old
// View), re-applies the last StyleSheet given via SetStyleSheet/
// WithStyleSheet (if any) so switching documents doesn't silently drop
// back to the new View's own default styling, and lays the new View
// out immediately, the same as NewPanel does.
func (p *Panel) SetView(v *whynot.View) {
	p.view = v
	if p.styleSheet != nil {
		p.view.SetStyleSheet(p.styleSheet)
	}
	p.interaction.Reset()
	p.draggingScrollbar = false
	p.scrollbarState = buttonState{}
	p.relayout()
}

// Bounds returns this panel's current screen-space rectangle.
func (p *Panel) Bounds() image.Rectangle {
	return p.bounds
}

// SetBounds resizes/repositions the panel - call from the host's own
// Layout callback whenever its window size changes. Re-lays-out the
// View immediately (see View.Layout) so DocumentBounds/etc. reflect the
// new size before the next Draw, rather than lagging a frame behind.
func (p *Panel) SetBounds(bounds image.Rectangle) {
	p.bounds = bounds
	p.relayout()
}

// Scale returns the zoom/DPI scale currently passed to View.Layout.
func (p *Panel) Scale() float64 {
	return p.scale
}

// SetScale changes the zoom/DPI scale - see Bounds/SetBounds; same
// immediate re-layout.
func (p *Panel) SetScale(scale float64) {
	p.scale = scale
	p.relayout()
}

// SetStyleSheet sets the document's StyleSheet on the current View, and
// remembers it (see View, above) so a later SetView can re-apply it to
// the new View too, and so the scrollbar - which has no way to read a
// View's StyleSheet back out - knows what to draw with. Use this
// instead of calling p.View().SetStyleSheet directly, or both of those
// go stale.
func (p *Panel) SetStyleSheet(s whynot.StyleSheet) {
	p.styleSheet = s
	p.view.SetStyleSheet(s)
}

// ScrollDown/ScrollUp move by a small amount (a few lines) - the same
// increment cmd/whynot's own arrow keys use. PageDown/PageUp move by
// one panel-height, minus a small overlap so the last line read stays
// visible as a landmark - same idea as cmd/whynot's Space/Shift+Space.
// Panel doesn't read keyboard input itself (would risk conflicting with
// a host game's own controls) - these exist so a host binding its own
// keys/gamepad/buttons doesn't have to invent a sensible amount.
//
// Deliberate simplification: unlike cmd/whynot's own arrow keys (which
// use device scale alone, staying zoom-independent), ScrollDown/ScrollUp
// scale with the panel's single combined scale - Panel has no separate
// device-scale-alone concept to keep them zoom-independent with.
const arrowScrollLines = 40
const pageOverlapFrac = 0.1

func (p *Panel) ScrollDown() { p.view.Scroll(-arrowScrollLines * p.scale) }
func (p *Panel) ScrollUp()   { p.view.Scroll(arrowScrollLines * p.scale) }

// PageDown/PageUp don't multiply by scale again - bounds is already in
// the same physical-pixel space scale would otherwise apply, mirroring
// cmd/whynot's own Space handling (g.height-g.toolbarHeight, never
// re-multiplied by deviceScale).
func (p *Panel) PageDown() { p.view.Scroll(-float64(p.bounds.Dy()) * (1 - pageOverlapFrac)) }
func (p *Panel) PageUp()   { p.view.Scroll(float64(p.bounds.Dy()) * (1 - pageOverlapFrac)) }

// elapsed is how long this Panel has existed - see start. Only ever
// compared to itself via View.Layout's now argument (animated-GIF frame
// selection), never to a wall-clock timestamp, so multiple panels don't
// need synchronized clocks, only each one's own monotonically increasing.
func (p *Panel) elapsed() time.Duration {
	return time.Since(p.start)
}

// relayout re-lays-out the current View at the panel's own bounds/scale
// - called by SetBounds, SetScale, SetView, and NewPanel, mirroring
// cmd/whynot's own relayout (layout.go).
func (p *Panel) relayout() {
	p.view.Layout(p.bounds.Dx(), p.bounds.Dy(), p.scale, p.elapsed())
}
