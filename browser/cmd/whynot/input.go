package main

import (
	"image"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

func (g *game) Update() error {
	start := time.Now()
	defer func() { g.updateDuration = time.Since(start) }()

	// While the input field is open, it has the keyboard and the pointer.
	if g.field.open {
		g.updateField()
		g.app.Update()
		return nil
	}

	g.app.Frame(g.input.Events(), time.Since(g.start))

	// pointerState so these buttons work with a touch too, not just a mouse.
	var pointerDown, clicked bool
	g.hoverX, g.hoverY, pointerDown, clicked = pointerState()
	cursor := image.Pt(g.hoverX, g.hoverY)
	g.updateStrip(cursor, clicked)
	g.backState = buttonState{hover: cursor.In(g.backButton), pressed: pointerDown && cursor.In(g.backButton)}
	g.forwardState = buttonState{hover: cursor.In(g.forwardButton), pressed: pointerDown && cursor.In(g.forwardButton)}
	g.reloadState = buttonState{hover: cursor.In(g.reloadButton), pressed: pointerDown && cursor.In(g.reloadButton)}
	// pressed also stays true while the TOC is showing, so the button
	// reads as "ON" the whole time, not just while physically held down.
	g.tocState = buttonState{hover: cursor.In(g.tocButton), pressed: (pointerDown && cursor.In(g.tocButton)) || g.app.TOCShowing()}
	g.actionsState = buttonState{hover: cursor.In(g.actionsButton), pressed: (pointerDown && cursor.In(g.actionsButton)) || g.app.ActionsShowing()}

	switch {
	case clicked && g.backState.hover:
		g.app.Back()
	case clicked && g.forwardState.hover:
		g.app.Forward()
	case clicked && g.reloadState.hover:
		g.app.Reload()
	case clicked && g.tocState.hover:
		g.app.ToggleTOC()
	case clicked && g.actionsState.hover:
		g.app.ToggleActions()
	}
	shortcut := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	shift := ebiten.IsKeyPressed(ebiten.KeyShift)
	alt := ebiten.IsKeyPressed(ebiten.KeyAlt)
	for _, k := range g.app.Keys() {
		key, ok := ebitenKeys[k.Name]
		if !ok || k.Shortcut != shortcut || k.Shift != shift || k.Alt != alt {
			continue
		}
		// A key that repeats fires again while held (see keyRepeat).
		if k.Repeats && keyRepeat(key) || !k.Repeats && inpututil.IsKeyJustPressed(key) {
			g.app.PressKey(k)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF) && !shortcut {
		g.debugStats = !g.debugStats
	}
	g.app.Update()
	return nil
}

// ebitenKeys are the ebiten keys of browser.Key names.
var ebitenKeys = map[string]ebiten.Key{
	"C": ebiten.KeyC, "J": ebiten.KeyJ, "L": ebiten.KeyL, "R": ebiten.KeyR, "T": ebiten.KeyT, "V": ebiten.KeyV,
	"0": ebiten.KeyDigit0, "[": ebiten.KeyBracketLeft, "]": ebiten.KeyBracketRight,
	"=": ebiten.KeyEqual, "-": ebiten.KeyMinus, "/": ebiten.KeySlash,
	"Space": ebiten.KeySpace, "Backspace": ebiten.KeyBackspace, "Escape": ebiten.KeyEscape,
	"Up": ebiten.KeyArrowUp, "Down": ebiten.KeyArrowDown, "Left": ebiten.KeyArrowLeft, "Right": ebiten.KeyArrowRight,
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
