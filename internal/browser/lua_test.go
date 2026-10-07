package browser

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// writeScript writes a Lua script to a temporary file and returns its
// lua: URL.
func writeScript(t *testing.T, source string) *url.URL {
	t.Helper()
	path := filepath.Join(t.TempDir(), "script.lua")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return &url.URL{Scheme: "lua", Opaque: path}
}

// loadPage loads ref, relative to base, through a registry with res.
func loadPage(t *testing.T, res *LuaResolver, base *url.URL, ref string) (*url.URL, string) {
	t.Helper()
	u, err := resolveAgainst(base, ref)
	if err != nil {
		t.Fatal(err)
	}
	page, err := LoadDocument(NewRegistry(nil, res), u)
	if err != nil {
		t.Fatal(err)
	}
	return u, string(page)
}

// linkDest returns the destination of the link labelled text in page.
func linkDest(t *testing.T, page, text string) string {
	t.Helper()
	m := regexp.MustCompile(`\[` + regexp.QuoteMeta(text) + `\]\(([^)]*)\)`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no link %q in page %q", text, page)
	}
	return m[1]
}

const counterScript = `
function page(request, state)
  state.n = (state.n or 0) + (tonumber(request.add) or 0)
  out("# Count\n\nn = ", state.n, "\n\n")
  out(link("Add one", {add = 1}), "\n\n", "[Add two](?add=2)\n")
end
`

// TestLuaStateFollowsLinks follows links from page to page, and checks
// each continues from the state of the page it's on, so that a link
// from an earlier page starts a new branch.
func TestLuaStateFollowsLinks(t *testing.T) {
	res := &LuaResolver{}
	start := writeScript(t, counterScript)
	first, page := loadPage(t, res, start, "")
	if !strings.Contains(page, "n = 0") {
		t.Fatalf("first page = %q, want n = 0", page)
	}
	second, page := loadPage(t, res, first, linkDest(t, page, "Add one"))
	if !strings.Contains(page, "n = 1") {
		t.Fatalf("after Add one, page = %q, want n = 1", page)
	}
	_, page = loadPage(t, res, second, linkDest(t, page, "Add two"))
	if !strings.Contains(page, "n = 3") {
		t.Fatalf("after Add two, page = %q, want n = 3", page)
	}

	// Another link from the first page, whose id is 1.
	_, page = loadPage(t, res, first, "?add=2&s=1")
	if !strings.Contains(page, "n = 2") {
		t.Errorf("Add two from the first page = %q, want n = 2: the first page's state, not the latest", page)
	}
}

func TestLuaErrors(t *testing.T) {
	for _, c := range []struct{ name, script, want string }{
		{"no page", `x = 1`, "no page function"},
		{"error", "function page() out('before') error('boom') end", "boom"},
		{"syntax", "function page(", "script.lua"},
		{"cpu", "function page() while true do end end", "CPU"},
		{"function in state", "function page(r, state) state.f = print end", "plain data"},
		{"no io", "function page() out(io == nil and 'no io' or 'io!') end", "no io"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, page := loadPage(t, &LuaResolver{cpuLimit: 1_000_000}, writeScript(t, c.script), "")
			if !strings.Contains(page, c.want) {
				t.Errorf("page = %q, want it to mention %q", page, c.want)
			}
		})
	}
}

// TestLuaFailedPageKeepsItsStartingState checks that links from a page
// whose script fails continue from the state the page started from.
func TestLuaFailedPageKeepsItsStartingState(t *testing.T) {
	res := &LuaResolver{}
	start := writeScript(t, `
function page(request, state)
  state.n = (state.n or 0) + 1
  out("n = ", state.n, " [again](?)\n")
  if request.fail then error("failed") end
end`)
	first, _ := loadPage(t, res, start, "")
	failed, page := loadPage(t, res, first, "?fail=1&s=1")
	if !strings.Contains(page, "failed") {
		t.Fatalf("page = %q, want the error", page)
	}
	_, page = loadPage(t, res, failed, linkDest(t, page, "again"))
	if !strings.Contains(page, "n = 2") {
		t.Errorf("page after a failed one = %q, want n = 2: the failed page's move doesn't count", page)
	}
}

// TestLuaClaude checks that claude{...} writes its text into the page and
// returns it, that claude.ask{...} only returns it, and that a second
// call with the same inputs uses the first's text.
func TestLuaClaude(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			System   string
			Messages []claudeMessage
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, body.Messages[0].Content)
		if !strings.HasPrefix(body.System, luaClaudeSystemPrompt) || !strings.HasSuffix(body.System, "Be gothic.") {
			t.Errorf("system prompt = %q, want the built-in one, then the script's", body.System)
		}
		for _, text := range []string{"The cellar ", "is dark."} {
			delta, _ := json.Marshal(map[string]any{
				"type":  "content_block_delta",
				"delta": map[string]string{"type": "text_delta", "text": text},
			})
			fmt.Fprintf(w, "data: %s\n\n", delta)
		}
		io.WriteString(w, "data: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	res := &LuaResolver{Claude: &ClaudeResolver{Endpoint: server.URL}}
	start := writeScript(t, `
claude.system = "Be gothic."
function page(request, state)
  local text = claude{prompt = "Describe the cellar.", context = {lamp = false, items = {"rope"}}}
  out("\n\n[", #text, "]")
  local verdict = claude.ask{prompt = "Judge."}
  out("\n\nverdict: ", verdict:upper())
end`)
	_, page := loadPage(t, res, start, "")
	if want := "The cellar is dark.\n\n[19]\n\nverdict: THE CELLAR IS DARK."; page != want {
		t.Errorf("page = %q, want %q", page, want)
	}
	if len(requests) != 2 || !strings.Contains(requests[0], `"items": [`) {
		t.Fatalf("requests = %q, want two, the first with its context as JSON", requests)
	}
	loadPage(t, res, start, "")
	if len(requests) != 2 {
		t.Errorf("%d requests after loading the page again, want still 2: the same inputs reuse the text", len(requests))
	}
}

func TestLuaLinks(t *testing.T) {
	var b strings.Builder
	l := &luaLinks{w: &b, id: "7"}
	for _, s := range []string{"[a](?x=1) [b]", "(?y=2) [c](https://example.com) [d](", "?)"} {
		l.Write([]byte(s))
	}
	l.flush()
	if want := "[a](?s=7&x=1) [b](?s=7&y=2) [c](https://example.com) [d](?s=7&)"; b.String() != want {
		t.Errorf("rewritten = %q, want %q", b.String(), want)
	}
}
