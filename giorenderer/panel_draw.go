package giorenderer

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget/material"

	"github.com/arnodel/whynot"
)

// Draw renders the panel - the document, clipped to Bounds, and its
// scrollbar if enabled - into gtx.Ops. Call once per frame, after
// Update.
func (p *Panel) Draw(gtx layout.Context) {
	// Cheap when width/scale are unchanged; called unconditionally so
	// an animated GIF gets fresh timing every rendered frame - same
	// reasoning as ebitenrenderer.Panel.Draw.
	p.relayout()

	canvas := p.Renderer.NewCanvas(gtx.Ops, p.bounds)
	p.view.Draw(canvas, p.bounds.Min.X, p.bounds.Min.Y)

	if p.scrollbarEnabled {
		p.drawScrollbar(gtx)
	}
}

// drawScrollbar lays out Gio's own material.Scrollbar in a strip along
// Bounds' right edge - its width comes from the widget itself
// (ScrollbarStyle.Width), not a share of the document's own margin the
// way ebitenrenderer.Panel's hand-drawn scrollbar borrows. Laying it out
// here (registering its own input area) is what makes Update's routing
// comment true: it must happen after Update's own clip.Rect(p.bounds)/
// event.Op so the scrollbar's area is foremost over the overlapping
// strip.
func (p *Panel) drawScrollbar(gtx layout.Context) {
	doc := p.view.DocumentBounds()
	if doc.Dy() == 0 {
		return
	}
	visible := p.view.VisibleViewBounds(image.Pt(p.bounds.Dx(), p.bounds.Dy()))
	viewportStart := float32(visible.Min.Y) / float32(doc.Dy())
	viewportEnd := float32(visible.Max.Y) / float32(doc.Dy())

	sb := material.Scrollbar(p.theme, &p.scrollbar)
	if sh, ok := p.styleSheet.(whynot.ScrollbarStyleSheet); ok {
		// Matches ebitenrenderer.Panel's own scrollbarColor: read the
		// thumb's color from the current StyleSheet rather than material's
		// default theme, which is a light-mode palette (black Fg) that's
		// invisible against a dark StyleSheet's own background - material's
		// two-color-slot model (idle/hover, no separate pressed) means
		// Dragging() is folded into the hover slot.
		sb.Indicator.Color = toNRGBA(sh.ScrollbarColor(false, false))
		sb.Indicator.HoverColor = toNRGBA(sh.ScrollbarColor(true, p.scrollbar.Dragging()))
	}
	width := gtx.Dp(sb.Width())
	strip := image.Rect(p.bounds.Max.X-width, p.bounds.Min.Y, p.bounds.Max.X, p.bounds.Max.Y)

	stack := op.Offset(strip.Min).Push(gtx.Ops)
	stripGtx := gtx
	stripGtx.Constraints = layout.Exact(strip.Size())
	sb.Layout(stripGtx, layout.Vertical, viewportStart, viewportEnd)
	stack.Pop()

	// ScrollDistance is normalized ([-1,1], fraction of the document),
	// not pixels - and positive same as pointer.Event.Scroll (moving
	// the viewport toward the document's end), so negated for
	// View.Scroll the same reason Update negates wheel scroll. Applied
	// directly, not through Interaction.Scroll's bounds gate: a
	// scrollbar drag isn't gated on cursor-over-document the way wheel/
	// touch scroll is - the user is dragging the scrollbar itself, not
	// scrolling the document surface.
	if d := p.scrollbar.ScrollDistance(); d != 0 {
		p.view.Scroll(-float64(d) * float64(doc.Dy()))
	}
}
