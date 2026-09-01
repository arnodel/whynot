package whynot

import "testing"

// stackOf builds a StackBox from already-built boxes, as pre-resolved
// slots - for tests that just want a StackBox with known children and
// don't need to exercise lazy building via Block.
func stackOf(boxes ...Box) *StackBox {
	return &StackBox{slots: preResolvedSlots(boxes)}
}

func TestStackBoxResolve(t *testing.T) {
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
		got := stack.resolve(tc.c)
		if got != tc.want {
			t.Errorf("resolve(%+v) = %+v, want %+v", tc.c, got, tc.want)
		}
	}
}

func TestStackBoxResolveEmpty(t *testing.T) {
	stack := &StackBox{}
	got := stack.resolve(stackCursor{0, 5})
	if got != (stackCursor{0, 0}) {
		t.Errorf("resolve on an empty StackBox = %+v, want {0, 0}", got)
	}
}

// TestStackBoxResolveIdempotent checks that resolve leaves an
// already-canonical cursor - one already satisfying
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
			c := stackCursor{index: index, offset: float64(offset)}
			got := stack.resolve(c)
			if got != c {
				t.Errorf("resolve(%+v) (already canonical) = %+v, want unchanged", c, got)
			}
		}
	}
}
