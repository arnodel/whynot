// Command systemfont is a runnable example of systemfont.SystemFontFaceSelector.
// By default it tries to resolve "Arial" for regular proportional text via
// RegisterSystemFont; pass -family "Some Other Font" to try something
// else. If the requested family isn't installed (or isn't found for any
// other reason), RegisterSystemFont logs a warning and this example falls
// back transparently to the bundled Go fonts - watch stderr to see what
// actually got resolved.
package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/ebitenrenderer"
	"github.com/arnodel/whynot/systemfont"
)

func exampleDoc(family string) string {
	var b strings.Builder
	b.WriteString("# System font example\n\n")
	fmt.Fprintf(&b, "Regular proportional text here should be **%s**, if it's installed on\n", family)
	b.WriteString("this machine - check stderr for whether it actually resolved. Some\n")
	b.WriteString("**bold** text, some *italic* text, and some `monospace` text - those\n")
	b.WriteString("still come from the bundled Go fonts, since only the regular/\n")
	b.WriteString("proportional slot is registered by this example.\n")
	return b.String()
}

type game struct {
	view     *whynot.View
	renderer *ebitenrenderer.Renderer
	start    time.Time
}

func (g *game) Update() error {
	_, dy := ebiten.Wheel()
	g.view.Scroll(dy * 2)
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	g.view.Draw(g.renderer.NewCanvas(screen), 0, 0)
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	scale := ebiten.Monitor().DeviceScaleFactor()
	width := int(float64(outsideWidth) * scale)
	height := int(float64(outsideHeight) * scale)
	g.view.Layout(width, height, scale, time.Since(g.start))
	return width, height
}

func main() {
	family := flag.String("family", "Arial", "installed font family to try for regular proportional text")
	flag.Parse()

	selector := systemfont.NewSystemFontFaceSelector(72)
	selector.RegisterSystemFont(whynot.Proportional, *family)

	g := &game{
		view:     whynot.NewView([]byte(exampleDoc(*family)), selector),
		renderer: ebitenrenderer.New(),
		start:    time.Now(),
	}
	ebiten.SetWindowSize(800, 600)
	ebiten.SetWindowTitle("whynot system font example")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
