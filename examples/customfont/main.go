// Command customfont is a runnable example of fonts.CustomSelector.
// By default it registers the bundled Pacifico-Regular.ttf (SIL Open Font
// License, see OFL.txt) for regular proportional text; pass
// -font path/to/font.ttf to use a different font instead. Either way, every
// other slot (bold, italic, monospace, small caps) has nothing registered,
// so it falls back transparently to the bundled Go fonts via
// fonts.GoSelector.
package main

import (
	_ "embed"
	"flag"
	"image"
	"log"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/backends/ebitenbackend"
	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/input"
	"github.com/arnodel/whynot/styles/simpletheme"
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
	renderer *ebitenbackend.Renderer
	input    ebitenbackend.Input
	start    time.Time
}

func (g *game) Update() error {
	// Without a Controller, interpreting input is up to the program: here,
	// only the wheel scrolls. Its deltas are in logical pixels, so they're
	// scaled to the view's pixels.
	for _, e := range g.input.Events() {
		if w, ok := e.(input.Wheel); ok {
			g.view.ScrollBy(w.DY * g.view.Scale())
		}
	}
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	g.view.Draw(g.renderer.NewCanvas(screen), time.Since(g.start))
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	// Render at the display's real device scale, not just outsideWidth/
	// outsideHeight (logical points) - otherwise ebiten renders at 1x and
	// upscales to fit a HiDPI/Retina screen, blurring every glyph.
	scale := ebiten.Monitor().DeviceScaleFactor()
	width := int(float64(outsideWidth) * scale)
	height := int(float64(outsideHeight) * scale)
	g.view.SetScale(scale)
	g.view.SetBounds(image.Rect(0, 0, width, height))
	return width, height
}

func buildFaceSelector(fontPath string) fonts.FaceSelector {
	selector := fonts.NewCustomSelector()
	if fontPath == "" {
		if err := selector.AddFont(fonts.Proportional, font.WeightNormal, font.StyleNormal, defaultFontData, 0); err != nil {
			log.Fatalf("loading the bundled default font: %v", err)
		}
		return selector
	}
	if err := selector.AddFontFile(fonts.Proportional, font.WeightNormal, font.StyleNormal, fontPath, 0); err != nil {
		log.Fatalf("loading %s: %v", fontPath, err)
	}
	return selector
}

func main() {
	fontPath := flag.String("font", "", "path to a TTF/OTF file for regular proportional text (defaults to the bundled Pacifico)")
	flag.Parse()

	g := &game{
		view:     whynot.NewView(whynot.Parse([]byte(exampleDoc())), buildFaceSelector(*fontPath), simpletheme.DarkStyleSheet),
		renderer: ebitenbackend.New(),
		start:    time.Now(),
	}
	ebiten.SetWindowSize(800, 600)
	ebiten.SetWindowTitle("whynot custom font example")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
