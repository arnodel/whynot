package main

import (
	"flag"
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	flag.Parse()
	f := "test.md"
	if flag.NArg() != 0 {
		f = flag.Arg(0)
	}
	source, err := os.ReadFile(f)
	if err != nil {
		panic(err)
	}
	block := parseMarkdown(source)

	ebiten.SetWindowSize(1024, 768)
	ebiten.SetWindowTitle("Why Not?")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	var scale = ebiten.DeviceScaleFactor()

	game := &whynotController{
		ctx: RenderingContext{
			Scale:        scale,
			FaceSelector: NewGoFontFaceSelector(72 * scale),
		},
		block: block,
	}
	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

type whynotController struct {
	ctx     RenderingContext
	block   Block
	offsetY float64

	box      Box
	boxWidth int
	boxScale float64
}

func (c *whynotController) Update() error {
	_, dy := ebiten.Wheel()
	c.offsetY += dy * ebiten.DeviceScaleFactor() * 2
	return nil
}

func (c *whynotController) Draw(screen *ebiten.Image) {
	DrawBox(c.box, screen, 0, int(c.offsetY))
}

func (c *whynotController) Layout(outsideWidth, outsideHeight int) (int, int) {
	s := ebiten.DeviceScaleFactor()
	c.ctx.SetDPI(s * 72)
	c.ctx.Scale = s

	width := int(float64(outsideWidth) * s)
	height := int(float64(outsideHeight) * s)

	if width != c.boxWidth || s != c.boxScale {
		// Reflowing at a new width changes every block's height, so the raw
		// pixel offsetY (measured against the old tree) would point at
		// different content in the new one. Anchor on which outer-stack
		// entry is at the top of the viewport now, and how far through it,
		// then re-derive offsetY from the same (index, ratio) against the
		// rebuilt tree, so the same content stays at the top.
		var anchorIndex int
		var anchorRatio float64
		anchored := false
		if oldStack, ok := c.box.(*StackBox); ok {
			anchorIndex, anchorRatio, anchored = oldStack.anchorAt(-int(c.offsetY))
		}

		c.box = c.block.GetBox(c.ctx, width)
		c.boxWidth = width
		c.boxScale = s

		if anchored {
			if newStack, ok := c.box.(*StackBox); ok {
				if y, ok := newStack.positionOf(anchorIndex, anchorRatio); ok {
					c.offsetY = -float64(y)
				}
			}
		}
	}

	return width, height
}
