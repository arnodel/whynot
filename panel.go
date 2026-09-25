package whynot

import "image"

// Panel is the common shape ebitenrenderer.Panel and giorenderer.Panel
// both already have - enough for backend-agnostic app code (see
// browser.App) to drive a panel without depending on either backend.
// OnLinkClick/OnLinkHover are deliberately not part of this interface:
// both concrete types expose them as plain fields, wired directly by
// whichever backend constructs the panel.
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
