package browser

import (
	"runtime/debug"
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

// TestVersionFrom checks which source of the version wins, for each kind
// of build.
func TestVersionFrom(t *testing.T) {
	checkout := &debug.BuildInfo{
		Main:     debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "67109f6aaaabbbb"}},
	}
	installed := &debug.BuildInfo{Main: debug.Module{Version: "v0.6.0"}}
	for _, c := range []struct {
		name    string
		stamped string
		info    *debug.BuildInfo
		want    string
	}{
		{"release build", "0.6.0", checkout, "v0.6.0"},
		{"go install pkg@version", "dev", installed, "v0.6.0"},
		{"build from a checkout", "dev", checkout, "dev 67109f6"},
		{"no build info", "dev", nil, "dev"},
	} {
		if got := versionFrom(c.stamped, c.info); got != c.want {
			t.Errorf("%s: versionFrom = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestWelcomeHasNoPlaceholders checks every placeholder in the welcome
// page is filled in, whether or not the build's commit is known.
func TestWelcomeHasNoPlaceholders(t *testing.T) {
	if md := string(renderWelcome()); strings.Contains(md, "{{") {
		t.Errorf("rendered welcome page still has a placeholder:\n%s", md)
	}
}
