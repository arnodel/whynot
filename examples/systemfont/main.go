// Command systemfont is a runnable example of systemfont.SystemFontFaceSelector.
// By default it doesn't name a font at all: RegisterPreferredFont picks
// this platform's most likely UI font (a short curated, GOOS-aware
// candidate list - see systemfont.RegisterPreferredFont's doc comment)
// for both regular proportional text and monospace text, so nobody has to
// guess or hardcode a specific installed font name. Pass -family or
// -monospace-family to try a specific name instead (e.g. "Arial" on
// Windows) via RegisterSystemFont directly. Either way, what actually
// gets used depends on what's installed and how sysfont's fuzzy matching
// scores it - a requested (or candidate) family that isn't found at all
// logs a warning and falls back transparently to the bundled Go fonts for
// that slot; watch stderr to see exactly what got resolved.
package main

import (
	"flag"
	"log"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/ebitenrenderer"
	"github.com/arnodel/whynot/systemfont"
)

func exampleDoc() string {
	var b strings.Builder
	b.WriteString("# System font example\n\n")
	b.WriteString("Regular proportional text and `monospace text` here should both come\n")
	b.WriteString("from real installed fonts, if this machine has any - check stderr for\n")
	b.WriteString("exactly what got resolved (and why, if nothing did).\n\n")
	b.WriteString("**Bold** and *italic* proportional text use the matched font's own\n")
	b.WriteString("bold/italic face too, when it has one - falling back to the bundled Go\n")
	b.WriteString("fonts for that slot instead when it doesn't. Some fonts bundle every\n")
	b.WriteString("style into one file `AddFontCollection` reads in one go:\n")
	b.WriteString("**`bold monospace`** and *`italic monospace`* should both come from a\n")
	b.WriteString("real installed monospace font too, not the Go fonts, if it has those\n")
	b.WriteString("styles.\n")
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
	family := flag.String("family", "", "installed font family for regular proportional text (default: this platform's preferred font, no name needed)")
	monospaceFamily := flag.String("monospace-family", "", "installed font family for monospace text (default: this platform's preferred monospace font)")
	flag.Parse()

	selector := systemfont.NewSystemFontFaceSelector(72)
	if *family != "" {
		selector.RegisterSystemFont(whynot.Proportional, *family)
	} else {
		selector.RegisterPreferredFont(whynot.Proportional)
	}
	if *monospaceFamily != "" {
		selector.RegisterSystemFont(whynot.Monospace, *monospaceFamily)
	} else {
		selector.RegisterPreferredFont(whynot.Monospace)
	}

	g := &game{
		view:     whynot.NewView([]byte(exampleDoc()), selector),
		renderer: ebitenrenderer.New(),
		start:    time.Now(),
	}
	ebiten.SetWindowSize(800, 600)
	ebiten.SetWindowTitle("whynot system font example")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
