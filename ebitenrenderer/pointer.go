package ebitenrenderer

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// PointerState reports this tick's effective single pointer - a touch
// when one is active, taking priority over mouse state exactly the way
// Panel.Update does (see its own doc comment: a browser can synthesize
// compatibility mouse events from a touch, so treating both as
// separate, simultaneously-active pointers would double up), falling
// back to real mouse state otherwise.
//
// Exported so a host with its own touch-sensitive UI outside any Panel
// - cmd/whynot's toolbar, for instance - can hit-test it the same way
// Panel hit-tests the document, without reimplementing this touch/
// mouse priority itself. Deliberately stateless and simpler than
// Panel's own touch tracking (touchInput): a plain hit-test only needs
// "is a pointer down right now, and where," not a consistently-tracked
// touch ID to diff a smooth scroll delta against, so unlike Panel it
// doesn't try to keep following the same touch across ticks - whichever
// touch ebiten.AppendTouchIDs reports first this tick is "the" pointer,
// full stop.
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
