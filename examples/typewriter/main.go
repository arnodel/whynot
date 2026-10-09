// Command typewriter is a runnable example of a Document that grows as
// its Markdown is written: an Ebitengine game reveals a story a character
// at a time, through a stream (see whynot.Parser.Stream). Each frame, the
// View shows what was written since the last one, laying out only what's
// new. Press any key, or click, to reveal the rest at once.
package main

import (
	"image"
	"io"
	"log"
	"time"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/backends/ebitenbackend"
)

const story = `# The vault

The door groans open, and for a moment nothing moves.

Dust hangs in the lamplight. Along the far wall, a row of iron boxes
waits, each with a brass plate, each plate *blank*.

> Whoever sealed this room didn't mean for it to be opened.

You step inside. Behind you, very quietly, the door begins to close.
`

// charsPerTick is how fast the story is revealed: Ebitengine runs 60
// ticks a second.
const charsPerTick = 1

type game struct {
	panel    *whynot.Panel
	renderer *ebitenbackend.Renderer
	input    ebitenbackend.Input
	start    time.Time

	w       io.WriteCloser // the story's Document is written through w
	pending string         // what's still to reveal
}

func main() {
	doc, w := whynot.NewParser().Stream()
	panel := whynot.NewPanel(whynot.NewView(doc), image.Rectangle{})
	g := &game{panel: panel, renderer: ebitenbackend.New(), start: time.Now(), w: w, pending: story}

	ebiten.SetWindowSize(640, 480)
	ebiten.SetWindowTitle("whynot typewriter example")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}

func (g *game) Update() error {
	if g.pending != "" {
		n := 0
		for range charsPerTick {
			_, size := utf8.DecodeRuneInString(g.pending[n:])
			n += size
		}
		if len(inpututil.AppendJustPressedKeys(nil)) > 0 || inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			n = len(g.pending) // skip: reveal the rest at once
		}
		// Writing only stores the text: the View compiles and lays it
		// out when it next draws.
		io.WriteString(g.w, g.pending[:n])
		if g.pending = g.pending[n:]; g.pending == "" {
			g.w.Close() // the Document is complete
		}
	}
	g.panel.Frame(g.input.Events(), time.Since(g.start))
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	g.panel.Draw(g.renderer.NewCanvas(screen), time.Since(g.start))
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	scale := ebiten.Monitor().DeviceScaleFactor()
	w, h := int(float64(outsideWidth)*scale), int(float64(outsideHeight)*scale)
	g.panel.SetBounds(image.Rect(0, 0, w, h))
	g.panel.SetScale(scale)
	return w, h
}
