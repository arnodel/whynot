package ebitenrenderer

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestCanvasClipBounds(t *testing.T) {
	c := New().NewCanvas(ebiten.NewImage(100, 100))
	clipped := c.Clip(image.Rect(50, -10, 200, 40))
	if want := image.Rect(50, 0, 100, 40); clipped.Bounds() != want {
		t.Errorf("Clip bounds = %v, want %v (intersected with the canvas)", clipped.Bounds(), want)
	}
	if again := clipped.Clip(image.Rect(0, 0, 60, 60)).Bounds(); again != image.Rect(50, 0, 60, 40) {
		t.Errorf("nested Clip bounds = %v, want %v", again, image.Rect(50, 0, 60, 40))
	}
}
