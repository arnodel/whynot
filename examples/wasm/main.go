// Command wasm is a runnable example of whynot compiled to WebAssembly
// and run in a browser, via Ebitengine's own js/wasm backend - see
// README.md in this directory for how to build and serve it. It's
// ordinary Go otherwise (no build tags), so `go run .` also works as a
// normal desktop app, useful for checking the document/layout itself
// before doing the GOOS=js GOARCH=wasm dance.
//
// Deliberately minimal, a first step rather than a port of cmd/whynot:
// one embedded document (demo.md, a copy of testdata/demo.md - go:embed
// can't reach outside this package's own directory, so keep the two in
// sync by hand if one changes), scroll and window-resize reflow via
// ebitenrenderer.Panel, no toolbar, no file loading (a local path or
// http(s) URL argument wouldn't mean the same thing in a browser
// sandbox - see the README), and no link-click handling (Panel's
// OnLinkClick is left unset, so a link's hover-highlight still shows but
// clicking one does nothing yet). demo.md's own local images/links
// (cat.jpeg, test-large.md, ...) don't resolve here since only the
// Markdown text itself is embedded - whynot's existing graceful
// degradation shows their alt text/does nothing on click instead of
// erroring.
package main

import (
	_ "embed"
	"image"
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/ebitenrenderer"
)

// initialWidth/Height only matter for the very first frame - Layout
// overwrites Panel's real bounds/scale before anything is drawn (see
// game.Layout), the same reasoning cmd/whynot's own initialWindowWidth/
// Height comment gives.
const initialWidth, initialHeight = 1024, 768

//go:embed demo.md
var demoDoc []byte

func main() {
	view := whynot.NewView(demoDoc, whynot.NewGoFontFaceSelector(72), whynot.WithStyleSheet(whynot.NewDarkStyleSheet()))
	bounds := image.Rect(0, 0, initialWidth, initialHeight)
	panel := ebitenrenderer.NewPanel(view, ebitenrenderer.New(), bounds, ebitenrenderer.WithScrollbar())

	ebiten.SetWindowTitle("whynot wasm example")
	if err := ebiten.RunGame(&game{panel: panel}); err != nil {
		log.Fatal(err)
	}
}

type game struct {
	panel *ebitenrenderer.Panel
}

func (g *game) Update() error {
	g.panel.Update()
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	g.panel.Draw(screen)
}

// Layout fills the whole window/canvas with the Panel, tracking its
// physical (device-scaled) size on every call - the same reasoning as
// cmd/whynot's own relayout - so a browser window resize reflows the
// document instead of leaving it at its initial size, and so the panel
// itself sizes correctly on the very first Layout call too (bounds are
// only ever set here, not guessed at construction).
func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	scale := ebiten.Monitor().DeviceScaleFactor()
	width := int(float64(outsideWidth) * scale)
	height := int(float64(outsideHeight) * scale)
	g.panel.SetBounds(image.Rect(0, 0, width, height))
	g.panel.SetScale(scale)
	return width, height
}
