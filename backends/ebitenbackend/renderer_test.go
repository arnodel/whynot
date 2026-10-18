package ebitenbackend

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot/canvas"
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

func TestCanvasTransform(t *testing.T) {
	c := New().NewCanvas(ebiten.NewImage(100, 100))
	thumb := c.Clip(image.Rect(20, 20, 70, 70)).Transform(canvas.Transform{Scale: 0.5, X: 20, Y: 20})
	if want := image.Rect(0, 0, 100, 100); thumb.Bounds() != want {
		t.Errorf("Transform bounds = %v, want %v (the clip, mapped back)", thumb.Bounds(), want)
	}
	left := thumb.Clip(image.Rect(0, 0, 51, 100)).(*Canvas)
	if want := image.Rect(0, 0, 51, 100); left.Bounds() != want {
		t.Errorf("Clip bounds = %v, want %v (exact, in its own coordinates)", left.Bounds(), want)
	}
	if want := image.Rect(20, 20, 46, 70); left.dst.Bounds() != want {
		t.Errorf("Clip device bounds = %v, want %v (rounded to the nearest pixel)", left.dst.Bounds(), want)
	}
}
