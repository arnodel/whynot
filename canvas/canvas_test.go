package canvas

import (
	"image"
	"testing"
)

func TestTransform(t *testing.T) {
	half := Transform{Scale: 0.5, X: 10, Y: 20}
	if x, y := half.Apply(100, 40); x != 60 || y != 40 {
		t.Errorf("Apply(100, 40) = %v, %v, want 60, 40", x, y)
	}
	if x, y := half.Invert().Apply(60, 40); x != 100 || y != 40 {
		t.Errorf("Invert().Apply(60, 40) = %v, %v, want 100, 40", x, y)
	}
	if got := half.Then(half.Invert()); got != Identity {
		t.Errorf("t.Then(t.Invert()) = %v, want Identity", got)
	}
	// Then applies t first: scaled by 0.5 and moved by (10, 20), then
	// scaled by 2.
	double := Transform{Scale: 2}
	if x, y := half.Then(double).Apply(100, 40); x != 120 || y != 80 {
		t.Errorf("Then: Apply(100, 40) = %v, %v, want 120, 80", x, y)
	}
}

func TestTransformApplyRect(t *testing.T) {
	third := Transform{Scale: 1.0 / 3}
	if got, want := third.ApplyRect(image.Rect(1, 2, 7, 8)), image.Rect(0, 0, 3, 3); got != want {
		t.Errorf("ApplyRect = %v, want %v (rounded outwards)", got, want)
	}
	if got, want := Identity.ApplyRect(image.Rect(-5, 3, 10, 12)), image.Rect(-5, 3, 10, 12); got != want {
		t.Errorf("Identity.ApplyRect = %v, want %v", got, want)
	}
}
