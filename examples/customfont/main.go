// Command customfont is a runnable example of whynot.CustomFontFaceSelector.
// By default it registers the bundled Pacifico-Regular.ttf (SIL Open Font
// License, see OFL.txt) for regular proportional text; pass
// -font path/to/font.ttf to use a different font instead. Either way, every
// other slot (bold, italic, monospace, small caps) has nothing registered,
// so it falls back transparently to the bundled Go fonts via
// whynot.NewGoFontFaceSelector.
package main

import (
	_ "embed"
	"flag"
	"log"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/ebitenrenderer"
)

//go:embed Pacifico-Regular.ttf
var defaultFontData []byte

func exampleDoc() string {
	var b strings.Builder
	b.WriteString("# Custom font example\n\n")
	b.WriteString("Regular proportional text here is Pacifico, a handwriting font, unless\n")
	b.WriteString("`-font` points at something else. Some **bold** text, some *italic* text,\n")
	b.WriteString("and some `monospace` text - all three still come from the bundled Go\n")
	b.WriteString("fonts, since only the regular/proportional slot is registered by this\n")
	b.WriteString("example.\n")
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
	// Render at the display's real device scale, not just outsideWidth/
	// outsideHeight (logical points) - otherwise ebiten renders at 1x and
	// upscales to fit a HiDPI/Retina screen, blurring every glyph.
	scale := ebiten.Monitor().DeviceScaleFactor()
	width := int(float64(outsideWidth) * scale)
	height := int(float64(outsideHeight) * scale)
	g.view.Layout(width, height, scale, time.Since(g.start))
	return width, height
}

func buildFaceSelector(fontPath string) whynot.FaceSelector {
	selector := whynot.NewCustomFontFaceSelector(72)
	if fontPath == "" {
		if err := selector.AddFont(whynot.Proportional, font.WeightNormal, font.StyleNormal, defaultFontData, 0); err != nil {
			log.Fatalf("loading the bundled default font: %v", err)
		}
		return selector
	}
	if err := selector.AddFontFile(whynot.Proportional, font.WeightNormal, font.StyleNormal, fontPath, 0); err != nil {
		log.Fatalf("loading %s: %v", fontPath, err)
	}
	return selector
}

func main() {
	fontPath := flag.String("font", "", "path to a TTF/OTF file for regular proportional text (defaults to the bundled Pacifico)")
	flag.Parse()

	g := &game{
		view:     whynot.NewView([]byte(exampleDoc()), buildFaceSelector(*fontPath)),
		renderer: ebitenrenderer.New(),
		start:    time.Now(),
	}
	ebiten.SetWindowSize(800, 600)
	ebiten.SetWindowTitle("whynot custom font example")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
