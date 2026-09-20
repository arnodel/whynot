package systemfont

import (
	"testing"

	"github.com/arnodel/whynot"
)

func TestPreferredFontCandidatesNonEmpty(t *testing.T) {
	cases := []struct {
		goos   string
		family whynot.FontFamily
	}{
		{"darwin", whynot.Proportional},
		{"darwin", whynot.Monospace},
		{"windows", whynot.Proportional},
		{"windows", whynot.Monospace},
		{"linux", whynot.Proportional},
		{"linux", whynot.Monospace},
	}
	for _, c := range cases {
		if got := preferredFontCandidates(c.goos, c.family); len(got) == 0 {
			t.Errorf("preferredFontCandidates(%q, %v) = empty, want at least one candidate", c.goos, c.family)
		}
	}
}

func TestPreferredFontCandidatesSmallCapsEmpty(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux", "freebsd"} {
		if got := preferredFontCandidates(goos, whynot.SmallCaps); len(got) != 0 {
			t.Errorf("preferredFontCandidates(%q, SmallCaps) = %v, want empty", goos, got)
		}
	}
}

func TestPreferredFontCandidatesUnknownGOOSFallsBackToLinux(t *testing.T) {
	for _, family := range []whynot.FontFamily{whynot.Proportional, whynot.Monospace} {
		got := preferredFontCandidates("freebsd", family)
		want := preferredFontCandidates("linux", family)
		if len(got) != len(want) {
			t.Fatalf("unknown GOOS candidates(%v) = %v, want same as linux %v", family, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("unknown GOOS candidates(%v)[%d] = %q, want %q", family, i, got[i], want[i])
			}
		}
	}
}
