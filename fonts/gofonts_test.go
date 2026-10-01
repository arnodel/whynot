package fonts

import (
	"testing"

	"golang.org/x/image/font"
)

func TestFamilyString(t *testing.T) {
	cases := []struct {
		family Family
		want   string
	}{
		{Proportional, "Proportional"},
		{Monospace, "Monospace"},
		{SmallCaps, "SmallCaps"},
		{Family(99), "Family(99)"},
	}
	for _, c := range cases {
		if got := c.family.String(); got != c.want {
			t.Errorf("Family(%d).String() = %q, want %q", c.family, got, c.want)
		}
	}
}

// TestGoSelectorCachesFacePerDPI checks that one GoSelector serves several
// dpis at once, caching each face.
func TestGoSelectorCachesFacePerDPI(t *testing.T) {
	s := NewGoSelector()
	style := TextStyle{Size: 16}
	at72, err := s.SelectFace(style, 72)
	if err != nil {
		t.Fatalf("SelectFace: %v", err)
	}
	at144, _ := s.SelectFace(style, 144)
	if at72 == at144 {
		t.Error("SelectFace at two dpis returned the same face")
	}
	if again, _ := s.SelectFace(style, 72); again != at72 {
		t.Error("SelectFace back at the first dpi returned a new face, want the cached one")
	}
	if got, want := at144.Metrics().Height, 2*at72.Metrics().Height; got < want-64 || got > want+64 {
		t.Errorf("face height at 144 dpi = %v, want about twice the 72 dpi height (%v)", got, want)
	}
}

func TestGoSelectorNoBoldSmallCaps(t *testing.T) {
	if _, err := NewGoSelector().SelectFace(TextStyle{Size: 16, Weight: font.WeightBold, Family: SmallCaps}, 72); err == nil {
		t.Error("SelectFace for bold small caps returned no error; the Go fonts have none")
	}
}
