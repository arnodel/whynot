// Command systemfont is a runnable example of systemfont.SystemFontFaceSelector.
// By default it tries to resolve "Helvetica" for regular proportional text
// and "Courier" for monospace text via RegisterSystemFont - real installed
// families with distinct bold/italic faces on macOS; pass -family or
// -monospace-family to try something else (e.g. "Arial"/"Courier New" on
// Windows). What a query actually resolves to depends entirely on what's
// installed and how sysfont's fuzzy matching scores it - e.g. "Arial" on a
// Mac without a real Arial installed can resolve to an unrelated, styleless
// substitute instead of failing outright, which won't look distinct from
// the bundled Go fonts. If a requested family isn't found at all,
// RegisterSystemFont logs a warning and this example falls back
// transparently to the bundled Go fonts for that slot - watch stderr to
// see what actually got resolved either way.
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

func exampleDoc(family, monospaceFamily string) string {
	var b strings.Builder
	b.WriteString("# System font example\n\n")
	fmt.Fprintf(&b, "Regular proportional text here should be **%s**, and `monospace text`\n", family)
	fmt.Fprintf(&b, "should be **%s**, if installed on this machine - check stderr for\n", monospaceFamily)
	b.WriteString("whether each one actually resolved.\n\n")
	b.WriteString("**Bold** and *italic* proportional text use the matched font's own\n")
	b.WriteString("bold/italic face too, when it has one - falling back to the bundled Go\n")
	b.WriteString("fonts for that slot instead when it doesn't. Some fonts bundle every\n")
	b.WriteString("style into one file `AddFontCollection` reads in one go:\n")
	b.WriteString("**`bold monospace`** and *`italic monospace`* should both come from\n")
	fmt.Fprintf(&b, "%s too, not the Go fonts, if it has those styles.\n", monospaceFamily)
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
	family := flag.String("family", "Helvetica", "installed font family to try for regular proportional text")
	monospaceFamily := flag.String("monospace-family", "Courier", "installed font family to try for monospace text")
	flag.Parse()

	selector := systemfont.NewSystemFontFaceSelector(72)
	selector.RegisterSystemFont(whynot.Proportional, *family)
	selector.RegisterSystemFont(whynot.Monospace, *monospaceFamily)

	g := &game{
		view:     whynot.NewView([]byte(exampleDoc(*family, *monospaceFamily)), selector),
		renderer: ebitenrenderer.New(),
		start:    time.Now(),
	}
	ebiten.SetWindowSize(800, 600)
	ebiten.SetWindowTitle("whynot system font example")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
