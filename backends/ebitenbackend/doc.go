// Package ebitenbackend is whynot's backend for Ebitengine
// (github.com/hajimehoshi/ebiten/v2). It provides both ends a whynot
// View or Panel needs from a game:
//
//   - drawing: a [Renderer] makes a [Canvas] (a [canvas.Canvas]) onto
//     an *ebiten.Image, typically the screen in the game's Draw;
//   - input: an [Input] reads Ebitengine's mouse, wheel and touch state
//     as [input.Event] values, once per tick in the game's Update.
//
// The core whynot packages don't depend on Ebitengine; only this one
// does.
//
// # A game showing a document
//
// A whynot.Panel ties these to Ebitengine's three game methods. It draws
// at the device's resolution, so text stays sharp on a high-density
// display:
//
//	type game struct {
//		panel    *whynot.Panel
//		renderer *ebitenbackend.Renderer
//		input    ebitenbackend.Input
//		start    time.Time
//	}
//
//	func (g *game) Update() error {
//		for _, e := range g.panel.Frame(g.input.Events(), time.Since(g.start)) {
//			if link, ok := e.(whynot.LinkClick); ok {
//				log.Println("clicked", link.Destination)
//			}
//		}
//		return nil
//	}
//
//	func (g *game) Draw(screen *ebiten.Image) {
//		g.panel.Draw(g.renderer.NewCanvas(screen), time.Since(g.start))
//	}
//
//	func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
//		scale := ebiten.Monitor().DeviceScaleFactor()
//		w, h := int(float64(outsideWidth)*scale), int(float64(outsideHeight)*scale)
//		g.panel.SetBounds(image.Rect(0, 0, w, h))
//		g.panel.SetScale(scale)
//		g.input.Scale = scale // wheel units to pixels
//		return w, h
//	}
//
// The coordinates of the canvas and of the input events are those of the
// game's screen, as whynot expects (see Coordinates in package whynot's
// documentation).
package ebitenbackend
