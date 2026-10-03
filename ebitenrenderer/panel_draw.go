package ebitenrenderer

import (
	"github.com/hajimehoshi/ebiten/v2"
)

// Draw renders the panel - the document, clipped to Bounds, with its
// scrollbar if enabled - onto dst.
func (p *Panel) Draw(dst *ebiten.Image) {
	// Cheap when width/scale are unchanged; called unconditionally so
	// an animated GIF gets fresh timing every rendered frame regardless
	// of whether/how many times Update ran this tick.
	p.relayout()

	// SubImage is a real clipping destination in ebiten: drawing onto
	// it is scissored to bounds, but coordinates stay in dst's own
	// absolute space, not re-based to zero - so View.Draw's x, y below
	// must be bounds' own absolute position, exactly like cmd/whynot's
	// existing view.Draw(canvas, 0, g.toolbarHeight) call, not (0, 0).
	sub := dst.SubImage(p.bounds).(*ebiten.Image)
	canvas := p.Renderer.NewCanvas(sub)
	p.view.Draw(canvas, p.bounds.Min.X, p.bounds.Min.Y)
}
