// Command panel is a runnable example of a whynot.Panel embedded in a
// larger Ebitengine game window: it fills the whole window with a solid
// color standing in for "other game content," then draws a Panel into
// an inset rectangle, showing its drawing stays within its bounds on
// all four edges.
package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/ebitenrenderer"
	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/styles/simpletheme"
)

const (
	windowWidth  = 800
	windowHeight = 600
	panelMargin  = 60 // inset on all sides, so the background shows around the panel
)

var backgroundColor = color.RGBA{0x20, 0x60, 0x20, 0xFF} // green, to make it distinct from the panel contents

func exampleDoc() string {
	var b strings.Builder
	b.WriteString("# Panel example\n\n")
	b.WriteString("This document exists to prove that `whynot.Panel` clips its drawing to\n")
	b.WriteString("its own bounds even when embedded inside a larger window that draws other\n")
	b.WriteString("content around it.\n\n")
	for i := 1; i <= 8; i++ {
		fmt.Fprintf(&b, "## Section %d\n\n", i)
		b.WriteString("Some text, quite a bit of it actually, more than one line's worth, so the\n")
		b.WriteString("document is tall enough to need scrolling within its panel.\n\n")
	}
	return b.String()
}

func main() {
	view := whynot.NewView(whynot.Parse([]byte(exampleDoc())), fonts.NewGoSelector(), simpletheme.DarkStyleSheet)
	bounds := image.Rect(panelMargin, panelMargin, windowWidth-panelMargin, windowHeight-panelMargin)
	panel := whynot.NewPanel(view, bounds)
	panel.SetScrollbar(true)

	ebiten.SetWindowSize(windowWidth, windowHeight)
	ebiten.SetWindowTitle("whynot panel example")
	g := &game{panel: panel, renderer: ebitenrenderer.New(), start: time.Now()}
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}

type game struct {
	panel    *whynot.Panel
	renderer *ebitenrenderer.Renderer
	input    ebitenrenderer.Input
	start    time.Time
}

func (g *game) Update() error {
	g.panel.Frame(g.input.Events(), time.Since(g.start))
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	screen.Fill(backgroundColor)
	g.panel.Draw(g.renderer.NewCanvas(screen), time.Since(g.start))
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return windowWidth, windowHeight
}
