package main

import (
	"flag"
	"image/color"
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/ebitenrenderer"
)

func main() {
	flag.Parse()
	f := "test.md"
	if flag.NArg() != 0 {
		f = flag.Arg(0)
	}
	source, err := os.ReadFile(f)
	if err != nil {
		panic(err)
	}

	ebiten.SetWindowSize(1024, 768)
	ebiten.SetWindowTitle("Why Not?")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	scale := ebiten.Monitor().DeviceScaleFactor()
	game := &game{
		view:     whynot.NewView(source, whynot.NewGoFontFaceSelector(72*scale)),
		renderer: ebitenrenderer.New(),
	}
	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

// game adapts a whynot.View to ebiten's Game interface: it owns window/input
// plumbing only, all rendering behavior lives in the library.
type game struct {
	view     *whynot.View
	renderer *ebitenrenderer.Renderer
}

func (g *game) Update() error {
	_, dy := ebiten.Wheel()
	g.view.Scroll(dy * ebiten.Monitor().DeviceScaleFactor() * 2)
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	// Short-term fix: explicit, rather than relying on ebiten's default per-frame clear (which is
	// transparent, not black - a real window just happens to show that as black since it ignores
	// alpha). Whynot itself never fills a background. Long term, background color should presumably
	// be a themeable/configurable property (e.g. for a future light mode) rather than hardcoded here -
	// where that lives (app vs library, and how it'd reach the document's own styling) isn't decided.
	screen.Fill(color.Black)
	g.view.Draw(g.renderer.NewCanvas(screen), 0, 0)
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	s := ebiten.Monitor().DeviceScaleFactor()
	width := int(float64(outsideWidth) * s)
	height := int(float64(outsideHeight) * s)
	g.view.Layout(width, s)
	return width, height
}
