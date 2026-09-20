// Command customfont is a runnable example of whynot.CustomFontFaceSelector
// - pass -font path/to/font.ttf to load a real font file from disk for
// regular proportional text; every other slot (bold, italic, monospace,
// small caps) has nothing registered, so it falls back transparently to
// the bundled Go fonts via whynot.NewGoFontFaceSelector. Run with no -font
// to see everything served by the fallback alone.
package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/ebitenrenderer"
)

func exampleDoc() string {
	var b strings.Builder
	b.WriteString("# Custom font example\n\n")
	b.WriteString("Regular proportional text here is loaded from `-font`, if given.\n")
	b.WriteString("Some **bold** text, some *italic* text, and some `monospace` text -\n")
	b.WriteString("all three still come from the bundled Go fonts, since only the\n")
	b.WriteString("regular/proportional slot is registered by this example.\n")
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
	g.view.Layout(outsideWidth, outsideHeight, 1, time.Since(g.start))
	return outsideWidth, outsideHeight
}

func buildFaceSelector(fontPath string) whynot.FaceSelector {
	fallback := whynot.NewGoFontFaceSelector(72)
	selector := whynot.NewCustomFontFaceSelector(72, fallback)
	if fontPath == "" {
		fmt.Println("no -font given: every slot falls back to the bundled Go fonts")
		return selector
	}
	if err := selector.AddFontFile(whynot.Proportional, font.WeightNormal, font.StyleNormal, fontPath); err != nil {
		log.Fatalf("loading %s: %v", fontPath, err)
	}
	return selector
}

func main() {
	fontPath := flag.String("font", "", "path to a TTF/OTF file for regular proportional text (optional)")
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
