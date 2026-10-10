package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// pointerState reports this tick's single effective pointer, for the
// toolbar: a touch when one is active, otherwise the mouse.
func pointerState() (x, y int, down, justPressed bool) {
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
