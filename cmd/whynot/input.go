package main

import (
	"image"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

func (g *game) Update() error {
	_, dy := ebiten.Wheel()
	g.current.view.Scroll(dy * ebiten.Monitor().DeviceScaleFactor() * 2)

	g.hoverX, g.hoverY = ebiten.CursorPosition()
	docY := g.hoverY - g.toolbarHeight

	var dest string
	var hasLink bool
	if docY >= 0 {
		dest, hasLink = g.current.view.Hover(g.hoverX, docY)
	} else {
		// Over the toolbar, not the document - (-1, -1) can't land on
		// anything, so this only ever clears a highlight left over from
		// just having moved off a link.
		g.current.view.Hover(-1, -1)
	}
	g.hoverDest = ""
	if hasLink {
		if resolved, err := g.resolveLink(dest); err == nil {
			g.hoverDest = resolved.String()
		}
	}

	cursor := image.Pt(g.hoverX, g.hoverY)
	mouseDown := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	g.backState = buttonState{hover: cursor.In(g.backButton), pressed: mouseDown && cursor.In(g.backButton)}
	g.forwardState = buttonState{hover: cursor.In(g.forwardButton), pressed: mouseDown && cursor.In(g.forwardButton)}
	g.reloadState = buttonState{hover: cursor.In(g.reloadButton), pressed: mouseDown && cursor.In(g.reloadButton)}
	g.zoomInState = buttonState{hover: cursor.In(g.zoomInButton), pressed: mouseDown && cursor.In(g.zoomInButton)}
	g.zoomOutState = buttonState{hover: cursor.In(g.zoomOutButton), pressed: mouseDown && cursor.In(g.zoomOutButton)}
	g.themeState = buttonState{hover: cursor.In(g.themeButton), pressed: mouseDown && cursor.In(g.themeButton)}

	clicked := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	switch {
	case hasLink && clicked:
		g.follow(dest)
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
			g.current.view.Scroll(page)
		} else {
			g.current.view.Scroll(-page)
		}
	}

	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyL):
		g.setTheme(false)
	case inpututil.IsKeyJustPressed(ebiten.KeyD):
		g.setTheme(true)
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
