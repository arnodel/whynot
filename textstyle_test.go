package whynot

import "testing"

func TestFontFamilyString(t *testing.T) {
	cases := []struct {
		family FontFamily
		want   string
	}{
		{Proportional, "Proportional"},
		{Monospace, "Monospace"},
		{SmallCaps, "SmallCaps"},
		{FontFamily(99), "FontFamily(99)"},
	}
	for _, c := range cases {
		if got := c.family.String(); got != c.want {
			t.Errorf("FontFamily(%d).String() = %q, want %q", c.family, got, c.want)
		}
	}
}
