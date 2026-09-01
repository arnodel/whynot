package whynot

import "testing"

// stackOf builds a StackBox from already-built boxes, as pre-resolved
// slots - for tests that just want a StackBox with known children and
// don't need to exercise lazy building via Block.
func stackOf(boxes ...Box) *StackBox {
	return &StackBox{slots: preResolvedSlots(boxes)}
}

func TestStackBoxAnchorAt(t *testing.T) {
	stack := stackOf(
		NewEmptyBox(0, 10),
		NewEmptyBox(0, 20),
		NewEmptyBox(0, 30),
	)

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
	stack := stackOf(
		NewEmptyBox(0, 10),
		NewEmptyBox(0, 20),
		NewEmptyBox(0, 30),
	)

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
	stack := stackOf(
		NewEmptyBox(0, 10),
		NewEmptyBox(0, 20),
		NewEmptyBox(0, 30),
	)

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

func TestStackBoxResolve(t *testing.T) {
	stack := stackOf(
		NewEmptyBox(0, 10),
		NewEmptyBox(0, 20),
		NewEmptyBox(0, 30),
	)

	cases := []struct {
		index, offset         int
		wantIndex, wantOffset int
	}{
		{index: 0, offset: 0, wantIndex: 0, wantOffset: 0},
		{index: 0, offset: 9, wantIndex: 0, wantOffset: 9},
		{index: 0, offset: 10, wantIndex: 1, wantOffset: 0},
		{index: 1, offset: 5, wantIndex: 1, wantOffset: 5},
		{index: 1, offset: -7, wantIndex: 0, wantOffset: 3},
		{index: 2, offset: -35, wantIndex: 0, wantOffset: 0},
		{index: 0, offset: 17, wantIndex: 1, wantOffset: 7},
		{index: 1, offset: 20, wantIndex: 2, wantOffset: 0},
		{index: 0, offset: 60, wantIndex: 2, wantOffset: 30},
		{index: 0, offset: 1000, wantIndex: 2, wantOffset: 30},
		{index: 2, offset: 30, wantIndex: 2, wantOffset: 30},
	}

	for _, c := range cases {
		gotIndex, gotOffset := stack.resolve(c.index, c.offset)
		if gotIndex != c.wantIndex || gotOffset != c.wantOffset {
			t.Errorf("resolve(%d, %d) = (%d, %d), want (%d, %d)",
				c.index, c.offset, gotIndex, gotOffset, c.wantIndex, c.wantOffset)
		}
	}
}

func TestStackBoxResolveEmpty(t *testing.T) {
	stack := &StackBox{}
	gotIndex, gotOffset := stack.resolve(0, 5)
	if gotIndex != 0 || gotOffset != 0 {
		t.Errorf("resolve on an empty StackBox = (%d, %d), want (0, 0)", gotIndex, gotOffset)
	}
}

// TestStackBoxResolveMatchesAnchorAt checks the claim that resolve(0, y)
// and anchorAt(y) agree: same index, and resolve's pixel offset matches
// anchorAt's ratio once scaled by that index's height. resolve(0, y) never
// takes the backward-walk branch (there's nothing before index 0), so its
// only possible path is the same forward scan anchorAt does.
func TestStackBoxResolveMatchesAnchorAt(t *testing.T) {
	stack := stackOf(
		NewEmptyBox(0, 10),
		NewEmptyBox(0, 20),
		NewEmptyBox(0, 30),
	)

	const epsilon = 1e-9
	for y := -10; y <= 70; y++ {
		wantIndex, wantRatio, ok := stack.anchorAt(y)
		if !ok {
			t.Fatalf("anchorAt(%d) not ok", y)
		}
		gotIndex, gotOffset := stack.resolve(0, y)
		if gotIndex != wantIndex {
			t.Errorf("y=%d: resolve(0, y) index = %d, anchorAt(y) index = %d", y, gotIndex, wantIndex)
			continue
		}
		h := stack.boxAt(gotIndex).Bounds().Dy()
		gotRatio := float64(gotOffset) / float64(h)
		if abs(gotRatio-wantRatio) > epsilon {
			t.Errorf("y=%d: resolve offset/height = %v, anchorAt ratio = %v", y, gotRatio, wantRatio)
		}
	}
}

// TestStackBoxResolveIdempotent checks that resolve leaves an
// already-canonical (index, offset) pair - one already satisfying
// 0 <= offset < height(index) - unchanged.
func TestStackBoxResolveIdempotent(t *testing.T) {
	stack := stackOf(
		NewEmptyBox(0, 10),
		NewEmptyBox(0, 20),
		NewEmptyBox(0, 30),
	)

	for index := 0; index < 3; index++ {
		h := stack.boxAt(index).Bounds().Dy()
		for offset := 0; offset < h; offset++ {
			gotIndex, gotOffset := stack.resolve(index, offset)
			if gotIndex != index || gotOffset != offset {
				t.Errorf("resolve(%d, %d) (already canonical) = (%d, %d), want unchanged",
					index, offset, gotIndex, gotOffset)
			}
		}
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
