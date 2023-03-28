package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {

	block := parseMarkdown("test.md")

	log.Printf("block found")
	ebiten.SetWindowSize(1024, 768)
	ebiten.SetWindowTitle("Why Not?")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	var dpi = 72 * ebiten.DeviceScaleFactor()

	game := &whynotController{
		faceSelector: NewGoFontFaceSelector(dpi),
		block:        block,
	}
	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

type whynotController struct {
	faceSelector FaceSelector
	block        Block
	offsetY      float64
}

func (c *whynotController) Update() error {
	_, dy := ebiten.Wheel()
	c.offsetY += dy * ebiten.DeviceScaleFactor()
	return nil
}

func (c *whynotController) Draw(screen *ebiten.Image) {
	box := c.block.GetBox(c.faceSelector, screen.Bounds().Dx())
	box.Draw(screen, 0, int(c.offsetY))
}

func (c *whynotController) Layout(outsideWidth, outsideHeight int) (int, int) {
	s := ebiten.DeviceScaleFactor()
	c.faceSelector.SetDPI(s * 72)
	return int(float64(outsideWidth) * s), int(float64(outsideHeight) * s)
}