package giobackend

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/arnodel/whynot"
)

// NativeScrollbar is Gio's material scrollbar, scrolling a View (or any
// ScrollTarget): for an app that wants its scrollbar to match the rest of
// its Gio UI rather than use the View's own ([whynot.WithScrollbar]). Its
// colors come from the View's StyleSheet; its width from Theme.
type NativeScrollbar struct {
	// Theme sets the scrollbar's size; nil means material.NewTheme().
	Theme *material.Theme

	scrollbar widget.Scrollbar
}

// ScrollTarget is what a NativeScrollbar scrolls: a [whynot.View], or
// anything else that scrolls the same way, such as several Views shown
// as one.
type ScrollTarget interface {
	// Bounds is where it's drawn: the scrollbar runs along its right edge.
	Bounds() image.Rectangle
	// VisibleRange is the part of its content in view, as fractions of
	// the content's height, as for [whynot.View.VisibleRange].
	VisibleRange() (start, end float64)
	// ScrollToRatio scrolls it to put the top of its bounds at ratio of
	// its content's height.
	ScrollToRatio(ratio float64)
	// ScrollbarColor is the color of the scrollbar's thumb, as for
	// [whynot.View.ScrollbarColor].
	ScrollbarColor(hover, pressed bool) color.Color
}

var _ ScrollTarget = (*whynot.View)(nil)

// Layout lays the scrollbar out along the right edge of view's Bounds,
// and scrolls view when it's dragged. Call it after registering the
// View's own input area for the frame (see Input.Source), so the
// scrollbar's area takes its input first.
func (s *NativeScrollbar) Layout(gtx layout.Context, view ScrollTarget) {
	start, end := view.VisibleRange()
	bounds := view.Bounds()

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
	sb.Layout(stripGtx, layout.Vertical, float32(start), float32(end))
	stack.Pop()

	// ScrollDistance is a fraction of the document, like VisibleRange.
	if d := s.scrollbar.ScrollDistance(); d != 0 {
		view.ScrollToRatio(start + float64(d))
	}
}
