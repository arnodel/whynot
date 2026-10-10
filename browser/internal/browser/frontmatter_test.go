package browser

import "testing"

func TestFrontMatterTitle(t *testing.T) {
	for _, c := range []struct{ raw, want string }{
		{"title: A Page\n", "A Page"},
		{"title: \"Quoted: with a colon\"\nauthor: Someone\n", "Quoted: with a colon"},
		{"author: Someone\n", ""},
		{"title: [not, a, string]\n", ""},
		{"not: valid: yaml\n", ""},
	} {
		if got := frontMatterTitle(c.raw); got != c.want {
			t.Errorf("frontMatterTitle(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}
