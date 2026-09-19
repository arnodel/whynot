// Command panelexample is a headless-verifiable demo of
// ebitenrenderer.Panel embedded in a larger game window: it fills the
// whole window with a solid color standing in for "other game
// content," then draws a Panel into an inset rectangle so a
// screenshot can show whether Panel.Draw's SubImage clipping actually
// holds on all four edges - something cmd/whynot never has to prove,
// since its View occupies basically the whole window.
package main

import (
	"image"
	"image/color"
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/ebitenrenderer"
)

const (
	windowWidth  = 800
	windowHeight = 600
	panelMargin  = 60 // inset on all sides, so the background shows around the panel
)

// backgroundColor is deliberately not black/white/gray - distinct
// from anything the document itself draws - so a screenshot makes it
// obvious whether Panel content ever leaks outside its bounds.
var backgroundColor = color.RGBA{0x20, 0x60, 0x20, 0xFF}

const longDoc = `# Panel example

This document exists to prove that ` + "`ebitenrenderer.Panel`" + ` clips its drawing to its
own bounds even when embedded inside a larger window that draws other content
around it.

## Section one

Some text, quite a bit of it actually, more than one line's worth, so the
document is tall enough to need scrolling within its panel.

## Section two

Some text, quite a bit of it actually, more than one line's worth, so the
document is tall enough to need scrolling within its panel.

## Section three

Some text, quite a bit of it actually, more than one line's worth, so the
document is tall enough to need scrolling within its panel.

## Section four

Some text, quite a bit of it actually, more than one line's worth, so the
document is tall enough to need scrolling within its panel.

## Section five

Some text, quite a bit of it actually, more than one line's worth, so the
document is tall enough to need scrolling within its panel.

## Section six

Some text, quite a bit of it actually, more than one line's worth, so the
document is tall enough to need scrolling within its panel.

## Section seven

Some text, quite a bit of it actually, more than one line's worth, so the
document is tall enough to need scrolling within its panel.

## Section eight

Some text, quite a bit of it actually, more than one line's worth, so the
document is tall enough to need scrolling within its panel.
`

func main() {
	view := whynot.NewView([]byte(longDoc), whynot.NewGoFontFaceSelector(72), whynot.WithStyleSheet(whynot.NewDarkStyleSheet()))
	bounds := image.Rect(panelMargin, panelMargin, windowWidth-panelMargin, windowHeight-panelMargin)
	panel := ebitenrenderer.NewPanel(view, ebitenrenderer.New(), bounds, ebitenrenderer.WithScrollbar())

	ebiten.SetWindowSize(windowWidth, windowHeight)
	ebiten.SetWindowTitle("panelexample")
	if err := ebiten.RunGame(&game{panel: panel}); err != nil {
		log.Fatal(err)
	}
}

type game struct {
	panel *ebitenrenderer.Panel
}

func (g *game) Update() error {
	g.panel.Update()
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	screen.Fill(backgroundColor)
	g.panel.Draw(screen)
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return windowWidth, windowHeight
}
