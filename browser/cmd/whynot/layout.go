package main

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// toolbarLogicalHeight is the address bar / button row's height, in the
// same DPI-relative logical units as StyleSheet text sizes (i.e. not
// pre-multiplied by scale here - layoutToolbar does that).
const toolbarLogicalHeight = 36

// layoutToolbar recomputes the toolbar's height and button positions
// for the current device scale - deliberately deviceScale, not scale,
// so the toolbar's own size stays fixed regardless of zoom.
func (g *game) layoutToolbar() {
	s := g.deviceScale
	g.toolbarHeight = int(toolbarLogicalHeight * s)
	pad := int(8 * s)
	btn := g.toolbarHeight - 2*pad // square icon buttons
	next := pad
	nextButton := func() image.Rectangle {
		r := image.Rect(next, pad, next+btn, pad+btn)
		next = r.Max.X + pad
		return r
	}
	g.backButton = nextButton()
	g.forwardButton = nextButton()
	g.reloadButton = nextButton()
	g.tocButton = nextButton()

	// The actions button (zoom, the theme and everything else) sits at
	// the toolbar's right corner, as a "more" menu usually does, apart
	// from back/forward/reload since it's not navigation.
	g.actionsButton = image.Rect(g.width-pad-btn, pad, g.width-pad, pad+btn)
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	g.outsideWidth, g.outsideHeight = outsideWidth, outsideHeight
	g.relayout()
	return g.width, g.app.Bounds().Max.Y
}

// applyDeviceScale recomputes the toolbar's own physical width
// (deviceScale alone, no zoom) and lays it out - split out from
// relayout so main can compute a correct initial toolbarHeight before
// app.Relayout runs for the first time.
func (g *game) applyDeviceScale() {
	g.deviceScale = ebiten.Monitor().DeviceScaleFactor()
	g.width = int(float64(g.outsideWidth) * g.deviceScale)
	g.layoutToolbar()
}

// relayout applies applyDeviceScale's result to the toolbar, then
// app.Relayout to the document panel - shared by the ebiten-driven
// Layout callback and setZoom, which needs the same recomputation to
// happen immediately rather than waiting for ebiten's next own Layout
// call.
func (g *game) relayout() {
	g.applyDeviceScale()
	g.app.Relayout(g.outsideWidth, g.outsideHeight, g.deviceScale, g.toolbarHeight)
}
