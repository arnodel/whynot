package whynot

import "image"

// Panel is what the backends' panels have in common, so app code can
// drive one without depending on a backend. The link callbacks aren't
// part of it: they're plain fields, set by whoever creates the panel.
type Panel interface {
	View() *View
	SetView(v *View)
	Bounds() image.Rectangle
	SetBounds(r image.Rectangle)
	Scale() float64
	SetScale(s float64)
	SetStyleSheet(s StyleSheet)
	ScrollDown()
	ScrollUp()
	PageDown()
	PageUp()
}
