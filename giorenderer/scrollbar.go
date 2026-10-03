package giorenderer

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/arnodel/whynot"
)

// NativeScrollbar is Gio's material scrollbar, scrolling a View: for an
// app that wants its scrollbar to match the rest of its Gio UI rather
// than use the View's own (whynot.WithScrollbar). Its colors come from
// the View's StyleSheet; its width from Theme.
type NativeScrollbar struct {
	// Theme sets the scrollbar's size; nil means material.NewTheme().
	Theme *material.Theme

	scrollbar widget.Scrollbar
}

// Layout lays the scrollbar out along the right edge of bounds, where
// view is drawn, and scrolls view when it's dragged. Call it after
// registering the View's own input area for the frame (see
// Input.Source), so the scrollbar's area takes its input first.
func (s *NativeScrollbar) Layout(gtx layout.Context, view *whynot.View, bounds image.Rectangle) {
	doc := view.DocumentBounds()
	if doc.Dy() == 0 {
		return
	}
	visible := view.VisibleViewBounds(bounds.Size())
	start := float32(visible.Min.Y) / float32(doc.Dy())
	end := float32(visible.Max.Y) / float32(doc.Dy())

	if s.Theme == nil {
		s.Theme = material.NewTheme()
	}
	sb := material.Scrollbar(s.Theme, &s.scrollbar)
	// material has no separate pressed color, so dragging shows as hover.
	sb.Indicator.Color = toNRGBA(view.ScrollbarColor(false, false))
	sb.Indicator.HoverColor = toNRGBA(view.ScrollbarColor(true, s.scrollbar.Dragging()))
	width := gtx.Dp(sb.Width())
	strip := image.Rect(bounds.Max.X-width, bounds.Min.Y, bounds.Max.X, bounds.Max.Y)

	stack := op.Offset(strip.Min).Push(gtx.Ops)
	stripGtx := gtx
	stripGtx.Constraints = layout.Exact(strip.Size())
	sb.Layout(stripGtx, layout.Vertical, start, end)
	stack.Pop()

	// ScrollDistance is a fraction of the document.
	if d := s.scrollbar.ScrollDistance(); d != 0 {
		view.ScrollBy(float64(d) * float64(doc.Dy()))
	}
}
