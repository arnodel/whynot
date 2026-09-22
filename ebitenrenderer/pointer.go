package ebitenrenderer

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// PointerState reports this tick's single effective pointer: a touch
// when one is active, otherwise the mouse. For a host with its own
// touch-sensitive UI outside a Panel, e.g. a toolbar.
func PointerState() (x, y int, down, justPressed bool) {
	if ids := ebiten.AppendTouchIDs(nil); len(ids) > 0 {
		id := ids[0]
		x, y := ebiten.TouchPosition(id)
		for _, j := range inpututil.AppendJustPressedTouchIDs(nil) {
			if j == id {
				return x, y, true, true
			}
		}
		return x, y, true, false
	}
	x, y = ebiten.CursorPosition()
	return x, y, ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft), inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
}
