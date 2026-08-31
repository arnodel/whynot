package whynot

import "testing"

func TestStackBoxAnchorAt(t *testing.T) {
	stack := &StackBox{boxes: []Box{
		NewEmptyBox(0, 10),
		NewEmptyBox(0, 20),
		NewEmptyBox(0, 30),
	}}

	cases := []struct {
		y         int
		wantIndex int
		wantRatio float64
		wantOK    bool
	}{
		{y: -5, wantIndex: 0, wantRatio: 0, wantOK: true},
		{y: 0, wantIndex: 0, wantRatio: 0, wantOK: true},
		{y: 5, wantIndex: 0, wantRatio: 0.5, wantOK: true},
		{y: 9, wantIndex: 0, wantRatio: 0.9, wantOK: true},
		{y: 10, wantIndex: 1, wantRatio: 0, wantOK: true},
		{y: 25, wantIndex: 1, wantRatio: 0.75, wantOK: true},
		{y: 29, wantIndex: 1, wantRatio: 0.95, wantOK: true},
		{y: 30, wantIndex: 2, wantRatio: 0, wantOK: true},
		{y: 59, wantIndex: 2, wantRatio: 29.0 / 30.0, wantOK: true},
		{y: 60, wantIndex: 2, wantRatio: 1, wantOK: true},
		{y: 1000, wantIndex: 2, wantRatio: 1, wantOK: true},
	}

	const epsilon = 1e-9
	for _, c := range cases {
		index, ratio, ok := stack.anchorAt(c.y)
		if ok != c.wantOK || index != c.wantIndex || abs(ratio-c.wantRatio) > epsilon {
			t.Errorf("anchorAt(%d) = (%d, %v, %v), want (%d, %v, %v)",
				c.y, index, ratio, ok, c.wantIndex, c.wantRatio, c.wantOK)
		}
	}
}

func TestStackBoxAnchorAtEmpty(t *testing.T) {
	stack := &StackBox{}
	if _, _, ok := stack.anchorAt(0); ok {
		t.Error("anchorAt on an empty StackBox should return ok=false")
	}
}

func TestStackBoxPositionOf(t *testing.T) {
	stack := &StackBox{boxes: []Box{
		NewEmptyBox(0, 10),
		NewEmptyBox(0, 20),
		NewEmptyBox(0, 30),
	}}

	cases := []struct {
		index  int
		ratio  float64
		wantY  int
		wantOK bool
	}{
		{index: 0, ratio: 0, wantY: 0, wantOK: true},
		{index: 0, ratio: 0.5, wantY: 5, wantOK: true},
		{index: 1, ratio: 0, wantY: 10, wantOK: true},
		{index: 1, ratio: 0.75, wantY: 25, wantOK: true},
		{index: 2, ratio: 0, wantY: 30, wantOK: true},
		{index: 2, ratio: 1, wantY: 60, wantOK: true},
		{index: -1, ratio: 0, wantOK: false},
		{index: 3, ratio: 0, wantOK: false},
	}

	for _, c := range cases {
		y, ok := stack.positionOf(c.index, c.ratio)
		if ok != c.wantOK || (ok && y != c.wantY) {
			t.Errorf("positionOf(%d, %v) = (%d, %v), want (%d, %v)",
				c.index, c.ratio, y, ok, c.wantY, c.wantOK)
		}
	}
}

// TestStackBoxAnchorRoundTrip checks that anchoring at a position and
// re-deriving it from the same (unchanged) tree returns the same position -
// the property the resize-anchor feature in Layout() depends on.
func TestStackBoxAnchorRoundTrip(t *testing.T) {
	stack := &StackBox{boxes: []Box{
		NewEmptyBox(0, 10),
		NewEmptyBox(0, 20),
		NewEmptyBox(0, 30),
	}}

	for y := 0; y < 60; y++ {
		index, ratio, ok := stack.anchorAt(y)
		if !ok {
			t.Fatalf("anchorAt(%d) not ok", y)
		}
		got, ok := stack.positionOf(index, ratio)
		if !ok {
			t.Fatalf("positionOf(%d, %v) not ok", index, ratio)
		}
		if got != y {
			t.Errorf("round trip for y=%d: anchorAt -> positionOf = %d", y, got)
		}
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
