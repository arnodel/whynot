//go:build !js

package browser

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arnodel/whynot/browser/internal/whymark/ai"
)

// cannedAI answers every request with the same text.
type cannedAI string

func (c cannedAI) Ask(ctx context.Context, req ai.Request, w io.Writer) (string, error) {
	if w != nil {
		io.WriteString(w, string(c))
	}
	return string(c), nil
}

// TestEnableAI checks that local whymark documents get the AI, and remote
// ones only when allowed.
func TestEnableAI(t *testing.T) {
	const doc = "---\nwhymark:\n  version: 1\n  uses:\n    ai: optional\n---\n# Shop\n\n" +
		"```ai\nDescribe the shop.\n```\n```without ai\nThe shop is quiet.\n```\n"
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, doc)
	}))
	defer remote.Close()

	for _, c := range []struct {
		name          string
		remoteAllowed bool
		link, want    string
	}{
		{"local", false, "shop.md", "Clocks tick everywhere"},
		{"remote", false, remote.URL + "/shop.md", "The shop is quiet"},
		{"remote, allowed", true, remote.URL + "/shop.md", "Clocks tick everywhere"},
	} {
		dir := t.TempDir()
		start := writeTempMD(t, dir, "start.md", "# Start\n")
		writeTempMD(t, dir, "shop.md", doc)
		app := newTestApp(t, dir, start, "# Start\n")
		app.EnableAI(cannedAI("Clocks tick everywhere."), map[string]string{"balanced": "test-model"}, c.remoteAllowed)
		app.Follow(c.link)
		if text := pageText(app); !strings.Contains(text, c.want) {
			t.Errorf("%s: the page shows %q, want %q", c.name, text, c.want)
		}
	}
}
