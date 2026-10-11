package compile

import (
	"strings"
	"testing"
)

// TestCompile checks what a program holds: its settings, its passages in
// order, quiet ones marked, its Lua and golua's code for it.
func TestCompile(t *testing.T) {
	doc := "---\ntitle: The Cellar\nwhymark:\n  version: 1\n  journal: true\n---\n\nSome prose.\n\n" +
		"```lua\nfunction init(turn) return {} end\n```\n\n" +
		"# Hall\n\nA hall.\n\n# (Take the lamp)\n\nYou take it.\n"
	p, err := Compile("cellar.md", []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "The Cellar" || !p.Journal || p.Version != 1 {
		t.Errorf("settings = %+v", p.Settings)
	}
	want := []Passage{{ID: "hall", Title: "Hall"}, {ID: "take-the-lamp", Title: "Take the lamp", Quiet: true}}
	if len(p.Passages) != 2 || p.Passages[0] != want[0] || p.Passages[1] != want[1] {
		t.Errorf("passages = %+v, want %+v", p.Passages, want)
	}
	if p.StartPassage() != "hall" {
		t.Errorf("StartPassage() = %q, want hall", p.StartPassage())
	}
	if !strings.Contains(p.Lua, "function init(turn)") || !strings.Contains(p.Lua, "function page(turn)") || p.Unit == nil {
		t.Error("the program lacks its prelude, its page(turn), or its code")
	}
	if n := strings.Count(p.Lua, "\n"); n < strings.Count(doc, "\n") {
		t.Errorf("the Lua has %d lines, fewer than the document's %d: line numbers won't match", n, strings.Count(doc, "\n"))
	}
}

// TestCompileErrors checks what doesn't compile.
func TestCompileErrors(t *testing.T) {
	for _, c := range []struct{ name, doc, want string }{
		{"plain Markdown", "# Hello\n", "not a whymark document"},
		{"no whymark key", "---\ntitle: X\n---\n# Hello\n", "not a whymark document"},
		{"another version", "---\nwhymark:\n  version: 2\n---\n# Hello\n", "version 2"},
		{"bad Lua", "---\nwhymark:\n  version: 1\n---\n# Hello\n\n`if then`\n", "doc.md:7"},
		{"an unclosed lua block", "---\nwhymark:\n  version: 1\n---\n# Hello\n\n```lua\nx = 1\n", "closing fence"},
	} {
		_, err := Compile("doc.md", []byte(c.doc))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one mentioning %q", c.name, err, c.want)
		}
	}
}
