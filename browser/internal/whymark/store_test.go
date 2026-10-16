package whymark

import (
	"bufio"
	"io"
	"math"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// TestSavedValues checks that state saved as JSON comes back with its
// types: integers and floats told apart, infinities, booleans, nested
// tables, and keys that aren't strings.
func TestSavedValues(t *testing.T) {
	inner := &table{keys: []any{int64(1), int64(2)}, values: []any{"a", math.Inf(1)}}
	value := &table{
		keys:   []any{"n", "x", true, int64(7), "list", 1.5},
		values: []any{int64(3), 3.0, "yes", nil, inner, false},
	}
	store := FileStore{Dir: t.TempDir()}
	snap := &snapshot{document: "doc", state: value, request: Request{Location: mustParse(t, "file:///doc.md?q=1#p")}}
	if err := store.SavePage("abc", snap.saved()); err != nil {
		t.Fatal(err)
	}
	saved, ok, err := store.LoadPage("abc")
	if err != nil || !ok {
		t.Fatalf("LoadPage: %v, %v", ok, err)
	}
	back, err := restored(saved)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back.state, value) {
		t.Errorf("the state came back as %#v, want %#v", back.state, value)
	}
	if back.request.Location.String() != "file:///doc.md?q=1#p" {
		t.Errorf("the location came back as %s", back.request.Location)
	}
	if _, ok, _ := store.LoadPage("../etc/passwd"); ok {
		t.Error("a page id that isn't one loaded something")
	}
}

// TestEngineResumesFromStore checks that pages kept in a Store outlive
// their engine: a new one, as in the program's next run, continues from
// them, with their state and their trail.
func TestEngineResumesFromStore(t *testing.T) {
	files := map[string]string{"count.md": "---\nwhymark:\n  version: 1\n---\n```lua\nfunction init(turn) return {n = 0} end\n```\n# (Count)\n\n" +
		"`turn.state.n = turn.state.n + 1`\ncount `= turn.state.n`, before `= turn.previous and turn.previous.state.n or \"none\"`\n"}
	store := FileStore{Dir: t.TempDir()}
	e, file := testEngine(t, files, WithStore(store))
	page := take(t, e, Request{Location: file("count.md")})
	page = follow(t, e, page, "?")
	wants(t, "the first run", page, []string{"count 2, before 1"}, nil)

	again, _ := testEngine(t, nil, WithStore(store))
	// The same files: the new engine's registry reads them from the first
	// engine's directory.
	again.registry = e.registry
	// The page as it was written, header and all, from the store.
	r, req, err := again.Saved(t.Context(), page.PageID)
	if err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(r)
	h, err := ReadHeader(br)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(br)
	if h.PageID != page.PageID || !strings.Contains(string(body), "count 2, before 1") || req.FromPageID == "" {
		t.Errorf("the saved page is %+v, %q, answering %+v", h, body, req)
	}
	resumed := take(t, again, Request{Location: page.Location, FromPageID: page.PageID})
	wants(t, "the next run, from the last page", resumed, []string{"count 3, before 2"}, []string{"The page stopped"})
	if _, err := again.Turn(t.Context(), Request{Location: page.Location, FromPageID: "nosuchpage"}); err == nil {
		t.Error("a page id the store doesn't have was continued from")
	}
}

func mustParse(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
