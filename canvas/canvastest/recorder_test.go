package canvastest

import (
	"image"
	"image/color"
	"testing"

	"github.com/arnodel/whynot/canvas"
)

func TestRecorderTransform(t *testing.T) {
	rec := &Recorder{Area: image.Rect(0, 0, 100, 100)}
	half := canvas.Transform{Scale: 0.5, X: 50}
	scaled := rec.Transform(half)
	if want := image.Rect(-100, 0, 100, 200); scaled.Bounds() != want {
		t.Errorf("Bounds = %v, want %v", scaled.Bounds(), want)
	}
	scaled.Clip(image.Rect(0, 0, 10, 10)).DrawRect(1, 2, 3, 4, color.White)
	rec.DrawRect(5, 6, 7, 8, color.White)
	want := []Rect{
		{X: 1, Y: 2, W: 3, H: 4, Color: color.White, Transform: half},
		{X: 5, Y: 6, W: 7, H: 8, Color: color.White},
	}
	if len(rec.Rects) != len(want) || rec.Rects[0] != want[0] || rec.Rects[1] != want[1] {
		t.Errorf("Rects = %v, want %v", rec.Rects, want)
	}
	nested := scaled.Transform(half).(*Recorder)
	if want := half.Then(half); nested.transform != want {
		t.Errorf("nested transform = %v, want %v", nested.transform, want)
	}
}
