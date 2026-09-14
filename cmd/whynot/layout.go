package main

import (
	"image"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// toolbarLogicalHeight is the address bar / button row's height, in the
// same DPI-relative logical units as StyleSheet text sizes (i.e. not
// pre-multiplied by scale here - layoutToolbar does that).
const toolbarLogicalHeight = 36

// layoutToolbar recomputes the toolbar's height and button positions
// for the current device scale - deliberately deviceScale, not scale,
// so the toolbar's own size stays fixed regardless of zoom (see
// deviceScale's own doc comment on the game struct).
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
	return g.width, g.height
}

// relayout recomputes physical width/height (from deviceScale alone)
// and scale (deviceScale*zoom) from the last known logical window size
// (outsideWidth/outsideHeight), and applies them to the toolbar and
// the current View - shared by the ebiten-driven Layout callback and
// setZoom, which needs the same recomputation to happen immediately
// rather than waiting for ebiten's next own Layout call (same reason
// follow/back/reload/paste each lay out their View immediately instead
// of leaving it for next frame).
func (g *game) relayout() {
	g.deviceScale = ebiten.Monitor().DeviceScaleFactor()
	g.scale = g.deviceScale * g.zoom
	g.width = int(float64(g.outsideWidth) * g.deviceScale)
	g.height = int(float64(g.outsideHeight) * g.deviceScale)
	g.toolbarFaceSelector.SetDPI(g.deviceScale * 72)
	g.layoutToolbar()
	g.current.view.Layout(g.width, g.scale, g.elapsed())
}

// zoomStep is a fixed step of the original (100%) size, not of the
// current zoom - so +/- (keyboard or button) goes 100%, 110%, 120%,
// ... rather than steps shrinking as you zoom out or growing as you
// zoom in.
const zoomStep = 0.1

// setZoom changes the zoom level (1.0 = 100%), clamped to a sane
// range, and re-lays-out immediately at the new scale - the same idea
// as a window resize, just user-triggered instead.
func (g *game) setZoom(zoom float64) {
	const minZoom, maxZoom = 0.5, 3.0
	g.zoom = math.Max(minZoom, math.Min(maxZoom, zoom))
	g.relayout()
	g.zoomIndicatorUntil = time.Now().Add(zoomIndicatorDuration)
}
