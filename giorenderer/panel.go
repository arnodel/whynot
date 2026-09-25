package giorenderer

import (
	"image"
	"time"

	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/arnodel/whynot"
)

// Panel adapts a whynot.View for embedding in a larger Gio UI - the Gio
// counterpart to ebitenrenderer.Panel, sharing its scroll/hover/click/
// momentum logic via whynot.Interaction, but using Gio's own native
// widget.Scrollbar for the scrollbar rather than a hand-drawn one.
var _ whynot.Panel = (*Panel)(nil)

type Panel struct {
	Renderer *Renderer

	// OnLinkClick/OnLinkHover - see ebitenrenderer.Panel's identical
	// fields.
	OnLinkClick func(destination string)
	OnLinkHover func(destination string)

	view   *whynot.View
	bounds image.Rectangle
	scale  float64

	styleSheet whynot.StyleSheet

	interaction whynot.Interaction

	scrollbarEnabled bool
	anchorScrolling  bool
	scrollbar        widget.Scrollbar
	theme            *material.Theme

	// Touch-drag state - see Update. At most one drag tracked at a time.
	dragging    bool
	lastDragPos image.Point

	start time.Time
}

// PanelOption customizes a Panel at construction, via NewPanel's opts
// parameter.
type PanelOption func(*Panel)

// WithScrollbar enables Panel's scrollbar - Gio's own widget.Scrollbar,
// drawn along Bounds' right edge.
func WithScrollbar() PanelOption {
	return func(p *Panel) { p.scrollbarEnabled = true }
}

// WithAnchorScrolling - see ebitenrenderer.Panel's identical option.
func WithAnchorScrolling() PanelOption {
	return func(p *Panel) { p.anchorScrolling = true }
}

// WithStyleSheet is equivalent to calling SetStyleSheet right after
// NewPanel.
func WithStyleSheet(s whynot.StyleSheet) PanelOption {
	return func(p *Panel) { p.SetStyleSheet(s) }
}

// WithTheme sets the material.Theme used for the scrollbar's own chrome
// (unrelated to the document's own font rendering). Defaults to
// material.NewTheme(), whose Palette.Fg is black - pass a theme with a
// brighter Fg when the document's StyleSheet uses a dark background, or
// the scrollbar indicator will be indistinguishable from it.
func WithTheme(theme *material.Theme) PanelOption {
	return func(p *Panel) { p.theme = theme }
}

// NewPanel constructs a Panel drawing view into bounds via renderer.
// scale starts at 1; call SetScale to change it.
func NewPanel(view *whynot.View, renderer *Renderer, bounds image.Rectangle, opts ...PanelOption) *Panel {
	p := &Panel{
		Renderer: renderer,
		view:     view,
		bounds:   bounds,
		scale:    1,
		theme:    material.NewTheme(),
		start:    time.Now(),
	}
	for _, opt := range opts {
		opt(p)
	}
	p.relayout()
	return p
}

// View returns the View this Panel currently draws/scrolls.
func (p *Panel) View() *whynot.View {
	return p.view
}

// SetView swaps the View this Panel draws/scrolls.
func (p *Panel) SetView(v *whynot.View) {
	p.view = v
	if p.styleSheet != nil {
		p.view.SetStyleSheet(p.styleSheet)
	}
	p.interaction.Reset()
	p.relayout()
}

// Bounds returns this panel's current screen-space rectangle.
func (p *Panel) Bounds() image.Rectangle {
	return p.bounds
}

// SetBounds resizes/repositions the panel.
func (p *Panel) SetBounds(bounds image.Rectangle) {
	p.bounds = bounds
	p.relayout()
}

// Scale returns the zoom/DPI scale currently passed to View.Layout.
func (p *Panel) Scale() float64 {
	return p.scale
}

// SetScale changes the zoom/DPI scale.
func (p *Panel) SetScale(scale float64) {
	p.scale = scale
	p.relayout()
}

// SetStyleSheet sets the document's StyleSheet on the current View, and
// remembers it so a later SetView can re-apply it - see
// ebitenrenderer.Panel's identical method.
func (p *Panel) SetStyleSheet(s whynot.StyleSheet) {
	p.styleSheet = s
	p.view.SetStyleSheet(s)
}

// ScrollDown/ScrollUp/PageDown/PageUp - see ebitenrenderer.Panel's
// identical methods.
const arrowScrollLines = 40
const pageOverlapFrac = 0.1

func (p *Panel) ScrollDown() { p.view.Scroll(-arrowScrollLines * p.scale) }
func (p *Panel) ScrollUp()   { p.view.Scroll(arrowScrollLines * p.scale) }
func (p *Panel) PageDown()   { p.view.Scroll(-float64(p.bounds.Dy()) * (1 - pageOverlapFrac)) }
func (p *Panel) PageUp()     { p.view.Scroll(float64(p.bounds.Dy()) * (1 - pageOverlapFrac)) }

func (p *Panel) elapsed() time.Duration {
	return time.Since(p.start)
}

func (p *Panel) relayout() {
	p.view.Layout(p.bounds.Dx(), p.bounds.Dy(), p.scale, p.elapsed())
}
