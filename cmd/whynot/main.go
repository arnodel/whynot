package main

import (
	"flag"
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
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

	scale := ebiten.DeviceScaleFactor()
	game := &game{
		view: whynot.NewView(source, whynot.NewGoFontFaceSelector(72*scale)),
	}
	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

// game adapts a whynot.View to ebiten's Game interface: it owns window/input
// plumbing only, all rendering behavior lives in the library.
type game struct {
	view *whynot.View
}

func (g *game) Update() error {
	_, dy := ebiten.Wheel()
	g.view.Scroll(dy * ebiten.DeviceScaleFactor() * 2)
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	g.view.Draw(screen, 0, 0)
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	s := ebiten.DeviceScaleFactor()
	width := int(float64(outsideWidth) * s)
	height := int(float64(outsideHeight) * s)
	g.view.Layout(width, s)
	return width, height
}
