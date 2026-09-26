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

	// zoomOut/zoomIn/theme sit on the toolbar's right edge instead,
	// grouped apart from back/forward/reload since they're not
	// navigation. nextButtonFromRight fills from the right edge inward,
	// so calling it for theme first puts the theme toggle at the very
	// corner, then + then - just to its left, reading left-to-right as
	// "- + [theme]" (the zoom pair matching e.g. Chrome's own
	// "- 100% +" zoom control, with the theme toggle as the outermost,
	// app-level setting next to it).
	right := g.width - pad
	nextButtonFromRight := func() image.Rectangle {
		r := image.Rect(right-btn, pad, right, pad+btn)
		right = r.Min.X - pad
		return r
	}
	g.themeButton = nextButtonFromRight()
	g.zoomInButton = nextButtonFromRight()
	g.zoomOutButton = nextButtonFromRight()
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	g.outsideWidth, g.outsideHeight = outsideWidth, outsideHeight
	g.relayout()
	return g.width, g.panel.Bounds().Max.Y
}

// applyDeviceScale recomputes the toolbar's own physical width
// (deviceScale alone, no zoom) and lays it out - split out from
// relayout so main can compute a correct initial toolbarHeight before
// app.Relayout runs for the first time.
func (g *game) applyDeviceScale() {
	g.deviceScale = ebiten.Monitor().DeviceScaleFactor()
	g.width = int(float64(g.outsideWidth) * g.deviceScale)
	g.toolbarFaceSelector.SetDPI(g.deviceScale * 72)
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
