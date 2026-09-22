package main

import (
	"image"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/arnodel/whynot/ebitenrenderer"
)

func (g *game) Update() error {
	start := time.Now()
	defer func() { g.updateDuration = time.Since(start) }()

	g.panel.Update()

	// PointerState so these buttons work with a touch too, not just a mouse.
	var pointerDown, clicked bool
	g.hoverX, g.hoverY, pointerDown, clicked = ebitenrenderer.PointerState()
	cursor := image.Pt(g.hoverX, g.hoverY)
	g.backState = buttonState{hover: cursor.In(g.backButton), pressed: pointerDown && cursor.In(g.backButton)}
	g.forwardState = buttonState{hover: cursor.In(g.forwardButton), pressed: pointerDown && cursor.In(g.forwardButton)}
	g.reloadState = buttonState{hover: cursor.In(g.reloadButton), pressed: pointerDown && cursor.In(g.reloadButton)}
	g.zoomInState = buttonState{hover: cursor.In(g.zoomInButton), pressed: pointerDown && cursor.In(g.zoomInButton)}
	g.zoomOutState = buttonState{hover: cursor.In(g.zoomOutButton), pressed: pointerDown && cursor.In(g.zoomOutButton)}
	g.themeState = buttonState{hover: cursor.In(g.themeButton), pressed: pointerDown && cursor.In(g.themeButton)}

	switch {
	case clicked && g.backState.hover:
		g.back()
	case clicked && g.forwardState.hover:
		g.forward()
	case clicked && g.reloadState.hover:
		g.reload()
	case clicked && g.zoomInState.hover:
		g.setZoom(g.zoom + zoomStep)
	case clicked && g.zoomOutState.hover:
		g.setZoom(g.zoom - zoomStep)
	case clicked && g.themeState.hover:
		g.setTheme(!g.darkTheme)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			g.forward()
		} else {
			g.back()
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		// A little overlap with the previous page - not the full
		// viewport height - so the last line you were reading is still
		// there as a landmark, same idea as a pager's own page-down.
		const pageOverlapFrac = 0.1
		page := float64(g.height-g.toolbarHeight) * (1 - pageOverlapFrac)
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			g.panel.View().Scroll(page)
		} else {
			g.panel.View().Scroll(-page)
		}
	}
	// A few lines at a time, repeating while held (see keyRepeat) -
	// finer-grained than Space's page jump, for nudging up/down a
	// short way without overshooting. Calls View.Scroll directly rather
	// than panel.ScrollDown/Up, which scale by panel's own combined
	// scale (deviceScale*zoom) - this stays deviceScale-alone,
	// deliberately zoom-independent like Space above.
	const arrowScrollLines = 40
	switch {
	case keyRepeat(ebiten.KeyDown):
		g.panel.View().Scroll(-arrowScrollLines * g.deviceScale)
	case keyRepeat(ebiten.KeyUp):
		g.panel.View().Scroll(arrowScrollLines * g.deviceScale)
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyF) {
		g.debugStats = !g.debugStats
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyV) && (ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)) {
		g.paste()
	}
	switch {
	case keyRepeat(ebiten.KeyEqual):
		g.setZoom(g.zoom + zoomStep)
	case keyRepeat(ebiten.KeyMinus):
		g.setZoom(g.zoom - zoomStep)
	}
	return nil
}

// keyRepeat reports whether a held key should fire again this tick -
// once immediately on press, then after an initial delay, repeatedly
// at a steady interval for as long as it's held (the same shape as OS
// keyboard repeat) - so +/- can be held down to zoom continuously
// instead of needing repeated individual presses.
func keyRepeat(key ebiten.Key) bool {
	tps := ebiten.TPS()
	const initialDelay = 2 * time.Second / 5 // ~0.4s
	const interval = time.Second / 10        // ~0.1s
	initialDelayTicks := int(initialDelay.Seconds() * float64(tps))
	intervalTicks := max(int(interval.Seconds()*float64(tps)), 1)

	d := inpututil.KeyPressDuration(key)
	if d == 1 {
		return true
	}
	if d <= initialDelayTicks {
		return false
	}
	return (d-initialDelayTicks)%intervalTicks == 0
}
