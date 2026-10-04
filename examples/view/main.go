// Command view is a runnable example of whynot.View wired up by hand -
// full control over input handling, at the cost of doing it yourself.
// See examples/panel for the turnkey alternative, whynot.Panel.
package main

import (
	"fmt"
	"image"
	"log"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/backends/ebitenbackend"
	"github.com/arnodel/whynot/input"
)

func exampleDoc() string {
	var b strings.Builder
	b.WriteString("# View example\n\n")
	b.WriteString("This document is rendered by wiring `whynot.View` up directly - full control\n")
	b.WriteString("over input handling, at the cost of doing it yourself (see `whynot.Panel`\n")
	b.WriteString("for the turnkey alternative).\n\n")
	for i := 1; i <= 8; i++ {
		fmt.Fprintf(&b, "## Section %d\n\n", i)
		b.WriteString("Some text, quite a bit of it actually, more than one line's worth, so the\n")
		b.WriteString("document is tall enough to need scrolling.\n\n")
	}
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
	// View.Draw fills its own background (from the View's StyleSheet) -
	// no separate clear step needed here.
	g.view.Draw(g.renderer.NewCanvas(screen), time.Since(g.start))
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	g.view.SetBounds(image.Rect(0, 0, outsideWidth, outsideHeight))
	return outsideWidth, outsideHeight
}

func main() {
	g := &game{
		view:     whynot.NewView(whynot.Parse([]byte(exampleDoc()))),
		renderer: ebitenbackend.New(),
		start:    time.Now(),
	}
	ebiten.SetWindowSize(800, 600)
	ebiten.SetWindowTitle("whynot view example")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
