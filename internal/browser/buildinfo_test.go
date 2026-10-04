package browser

import (
	"strings"
	"testing"
	"time"
)

func TestVCSBuildLine(t *testing.T) {
	b := vcsBuild{
		revision: "67109f6aaaabbbbccccddddeeeeffff000011112",
		time:     time.Date(2026, 10, 4, 10, 12, 0, 0, time.UTC),
	}
	want := "*Built from commit [67109f6](https://github.com/arnodel/whynot/commit/67109f6aaaabbbbccccddddeeeeffff000011112), of 4 Oct 2026, 10:12 UTC.*\n\n"
	if got := b.line(); got != want {
		t.Errorf("line() = %q, want %q", got, want)
	}
	b.modified = true
	if got := b.line(); !strings.Contains(got, ", with local changes.*") {
		t.Errorf("line() with local changes = %q, want it to say so", got)
	}
}

// TestWelcomeHasNoPlaceholders checks every placeholder in the welcome
// page is filled in, whether or not the build's commit is known.
func TestWelcomeHasNoPlaceholders(t *testing.T) {
	if md := string(renderWelcome()); strings.Contains(md, "{{") {
		t.Errorf("rendered welcome page still has a placeholder:\n%s", md)
	}
}
