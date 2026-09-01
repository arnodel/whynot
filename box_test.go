package whynot

import "testing"

// stackOf builds a StackBox from already-built boxes, as pre-resolved
// slots - for tests that just want a StackBox with known children and
// don't need to exercise lazy building via Block.
func stackOf(boxes ...Box) *StackBox {
	return &StackBox{slots: preResolvedSlots(boxes)}
}

func TestStackBoxNormalizeCursor(t *testing.T) {
	stack := stackOf(
		NewEmptyBox(0, 10),
		NewEmptyBox(0, 20),
		NewEmptyBox(0, 30),
	)

	cases := []struct {
		c    stackCursor
		want stackCursor
	}{
		{c: stackCursor{0, 0}, want: stackCursor{0, 0}},
		{c: stackCursor{0, 9}, want: stackCursor{0, 9}},
		{c: stackCursor{0, 10}, want: stackCursor{1, 0}},
		{c: stackCursor{1, 5}, want: stackCursor{1, 5}},
		{c: stackCursor{1, -7}, want: stackCursor{0, 3}},
		{c: stackCursor{2, -35}, want: stackCursor{0, 0}},
		{c: stackCursor{0, 17}, want: stackCursor{1, 7}},
		{c: stackCursor{1, 20}, want: stackCursor{2, 0}},
		{c: stackCursor{0, 60}, want: stackCursor{2, 30}},
		{c: stackCursor{0, 1000}, want: stackCursor{2, 30}},
		{c: stackCursor{2, 30}, want: stackCursor{2, 30}},
	}

	for _, tc := range cases {
		got := stack.normalizeCursor(tc.c)
		if got != tc.want {
			t.Errorf("normalizeCursor(%+v) = %+v, want %+v", tc.c, got, tc.want)
		}
	}
}

func TestStackBoxNormalizeCursorEmpty(t *testing.T) {
	stack := &StackBox{}
	got := stack.normalizeCursor(stackCursor{0, 5})
	if got != (stackCursor{0, 0}) {
		t.Errorf("normalizeCursor on an empty StackBox = %+v, want {0, 0}", got)
	}
}

// TestStackBoxNormalizeCursorIdempotent checks that normalizeCursor leaves an
// already-canonical cursor - one already satisfying
// 0 <= offset < height(index) - unchanged.
func TestStackBoxNormalizeCursorIdempotent(t *testing.T) {
	stack := stackOf(
		NewEmptyBox(0, 10),
		NewEmptyBox(0, 20),
		NewEmptyBox(0, 30),
	)

	for index := 0; index < 3; index++ {
		h := stack.boxAt(index).Bounds().Dy()
		for offset := 0; offset < h; offset++ {
			c := stackCursor{index: index, offset: float64(offset)}
			got := stack.normalizeCursor(c)
			if got != c {
				t.Errorf("normalizeCursor(%+v) (already canonical) = %+v, want unchanged", c, got)
			}
		}
	}
}

// TestStackBoxMoveCursor checks that moveCursor shifts by dy and
// normalizes the result, matching what an equivalent normalizeCursor call
// would give.
func TestStackBoxMoveCursor(t *testing.T) {
	stack := stackOf(
		NewEmptyBox(0, 10),
		NewEmptyBox(0, 20),
		NewEmptyBox(0, 30),
	)

	cases := []struct {
		c    stackCursor
		dy   float64
		want stackCursor
	}{
		{c: stackCursor{0, 5}, dy: 10, want: stackCursor{1, 5}},
		{c: stackCursor{1, 5}, dy: -12, want: stackCursor{0, 3}},
		{c: stackCursor{2, 30}, dy: 100, want: stackCursor{2, 30}},
	}

	for _, tc := range cases {
		got := stack.moveCursor(tc.c, tc.dy)
		if got != tc.want {
			t.Errorf("moveCursor(%+v, %v) = %+v, want %+v", tc.c, tc.dy, got, tc.want)
		}
	}
}
