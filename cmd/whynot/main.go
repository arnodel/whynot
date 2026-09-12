package main

import (
	"flag"
	"image"
	"image/color"
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/ebitenrenderer"
)

func main() {
	light := flag.Bool("light", false, "use whynot's light theme instead of the default dark one")
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

	var opts []whynot.ViewOption
	if *light {
		opts = append(opts, whynot.WithStyleSheet(whynot.NewLightStyleSheet()))
	}

	scale := ebiten.Monitor().DeviceScaleFactor()
	game := &game{
		view:     whynot.NewView(source, whynot.NewGoFontFaceSelector(72*scale), opts...),
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

	hoverX, hoverY int
}

func (g *game) Update() error {
	_, dy := ebiten.Wheel()
	g.view.Scroll(dy * ebiten.Monitor().DeviceScaleFactor() * 2)

	g.hoverX, g.hoverY = ebiten.CursorPosition()
	g.view.Hover(g.hoverX, g.hoverY)

	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyL):
		g.view.SetStyleSheet(whynot.NewLightStyleSheet())
	case inpututil.IsKeyJustPressed(ebiten.KeyD):
		g.view.SetStyleSheet(whynot.NewDarkStyleSheet())
	}
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	canvas := g.renderer.NewCanvas(screen)

	// View.Draw fills the background itself, from the View's StyleSheet -
	// no separate clear step needed here.
	g.view.Draw(canvas, 0, 0)

	if hit, offset := g.view.HitTest(g.hoverX, g.hoverY); hit != nil {
		drawOutline(canvas, hit.Bounds().Add(offset), color.RGBA{255, 0, 0, 255})
	}
}

// drawOutline draws a thin border around r - Canvas has no dedicated
// outline primitive, so this is 4 filled rects along the edges.
func drawOutline(dst whynot.Canvas, r image.Rectangle, clr color.Color) {
	const thickness = 2
	dst.DrawRect(r.Min.X, r.Min.Y, r.Dx(), thickness, clr)
	dst.DrawRect(r.Min.X, r.Max.Y-thickness, r.Dx(), thickness, clr)
	dst.DrawRect(r.Min.X, r.Min.Y, thickness, r.Dy(), clr)
	dst.DrawRect(r.Max.X-thickness, r.Min.Y, thickness, r.Dy(), clr)
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	s := ebiten.Monitor().DeviceScaleFactor()
	width := int(float64(outsideWidth) * s)
	height := int(float64(outsideHeight) * s)
	g.view.Layout(width, s)
	return width, height
}
