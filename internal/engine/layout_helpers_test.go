package engine

import (
	"image"
	"testing"
)

func TestResolveColumnWidths(t *testing.T) {
	cases := []struct {
		name      string
		natural   []int
		available int
		want      []int
	}{
		{
			name:      "fits naturally",
			natural:   []int{10, 20, 30},
			available: 100,
			want:      []int{10, 20, 30},
		},
		{
			// Worked by hand: sorted [10,20,100], K=1 (10 < 60/3=20;
			// 10+20=30 is not < 60/2=30), so column 0 stays narrow at
			// 10, and columns 1/2 share the remaining 50 proportionally
			// to their natural width (20:100).
			name:      "one narrow, two wide, sharing proportionally",
			natural:   []int{10, 20, 100},
			available: 60,
			want:      []int{10, 8, 41},
		},
		{
			// Same values, shuffled column order, checking the result
			// maps back to the original (not sorted) positions.
			name:      "shuffled order maps back correctly",
			natural:   []int{100, 10, 20},
			available: 60,
			want:      []int{41, 10, 8},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveColumnWidths(tc.natural, tc.available)
			if len(got) != len(tc.want) {
				t.Fatalf("resolveColumnWidths(%v, %d) = %v, want %v", tc.natural, tc.available, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("resolveColumnWidths(%v, %d) = %v, want %v", tc.natural, tc.available, got, tc.want)
					break
				}
			}
		})
	}
}

func TestFitWidth(t *testing.T) {
	for _, tc := range []struct {
		name  string
		r     image.Rectangle
		width int
		want  image.Rectangle
	}{
		{"already fits", image.Rect(0, 0, 100, 50), 200, image.Rect(0, 0, 100, 50)},
		{"exactly fits", image.Rect(0, 0, 200, 50), 200, image.Rect(0, 0, 200, 50)},
		{"too wide, scaled down", image.Rect(0, 0, 400, 600), 200, image.Rect(0, 0, 200, 300)},
		{"unbounded width", image.Rect(0, 0, 400, 600), 0, image.Rect(0, 0, 400, 600)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := fitWidth(tc.r, tc.width); got != tc.want {
				t.Errorf("fitWidth(%v, %d) = %v, want %v", tc.r, tc.width, got, tc.want)
			}
		})
	}
}
