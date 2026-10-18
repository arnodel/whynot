//go:build !js

package whymark

import (
	"path/filepath"
	"testing"
)

func newSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "whymark.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// newEngineOn returns an engine on store, as in the program's next run.
func newEngineOn(t *testing.T, store Store) *Engine {
	e, _ := testEngine(t, nil, WithStore(store))
	return e
}

// TestEnginePages checks the list of a document's pages: every page, with
// its parent, passage and title, by when it was made, from the Store and
// from a new engine on it alike.
func TestEnginePages(t *testing.T) {
	files := map[string]string{"cave.md": "---\nwhymark:\n  version: 1\n---\n# Entrance\n\n[In](#hall)\n\n# Hall\n\n[Out](#entrance)\n"}
	store := newSQLiteStore(t)
	e, file := testEngine(t, files, WithStore(store))
	start := take(t, e, Request{Location: file("cave.md")})
	hall := follow(t, e, start, "#hall")
	again := follow(t, e, start, "#hall") // a sibling of hall

	document := DocumentOf(file("cave.md"))
	for _, engine := range []*Engine{e, newEngineOn(t, store)} {
		pages, err := engine.Pages(document)
		if err != nil {
			t.Fatal(err)
		}
		if len(pages) != 3 {
			t.Fatalf("%d pages, want 3: %+v", len(pages), pages)
		}
		want := []struct{ id, parent, passage, title string }{
			{start.PageID, "", "entrance", "Entrance"},
			{hall.PageID, start.PageID, "hall", "Hall"},
			{again.PageID, start.PageID, "hall", "Hall"},
		}
		for i, w := range want {
			p := pages[i]
			if p.ID != w.id || p.Parent != w.parent || p.Passage != w.passage || p.Title != w.title {
				t.Errorf("page %d is %+v, want %+v", i, p, w)
			}
		}
	}
	if pages, _ := e.Pages("file:///elsewhere.md"); len(pages) != 0 {
		t.Errorf("another document has pages: %+v", pages)
	}
}
