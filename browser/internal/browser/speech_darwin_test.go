//go:build darwin && !ios

package browser

import "testing"

func TestJoinSentences(t *testing.T) {
	for _, c := range []struct{ a, b, want string }{
		{"Add milk", "And eggs", "Add milk. And eggs"},
		{"Add milk.", "And eggs", "Add milk. And eggs"},
		{"Is that all?", "Yes", "Is that all? Yes"},
		{"Add milk,", "eggs", "Add milk, eggs"},
		{"", "Hello", "Hello"},
		{"Hello ", "", "Hello"},
	} {
		if got := joinSentences(c.a, c.b); got != c.want {
			t.Errorf("joinSentences(%q, %q) = %q, want %q", c.a, c.b, got, c.want)
		}
	}
}
