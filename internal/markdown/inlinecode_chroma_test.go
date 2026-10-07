package markdown

import (
	"fmt"
	"strings"
	"testing"

	"github.com/arnodel/whynot/codeblocks"
	"github.com/arnodel/whynot/codeblocks/chromahighlight"
	"github.com/arnodel/whynot/internal/engine"
)

// TestInlineCodeWithChroma checks inline Lua through the real chroma
// plugin.
func TestInlineCodeWithChroma(t *testing.T) {
	parts := inlineCodeParts(t, "Run `if has(\"key\") then` now.", "lua", chromahighlight.Plugin{})
	classes := map[string]string{}
	for _, part := range parts {
		text := part.(*engine.InlineText)
		classes[text.Text] = text.ASTNode.Class
	}
	if got := textOf(t, parts); !stringsEqual(got, []string{"Run", "if", "has", "(", `"key"`, ")", "then", "now", "."}) {
		t.Fatalf("words = %v", got)
	}
	for text, want := range map[string]string{"if": codeblocks.ClassKeyword, "then": codeblocks.ClassKeyword, `"key"`: codeblocks.ClassString, "now": ""} {
		if classes[text] != want {
			t.Errorf("class of %q = %q, want %q", text, classes[text], want)
		}
	}
}

// BenchmarkInlineCode compiles a long document full of code spans, with
// and without coloring them.
func BenchmarkInlineCode(b *testing.B) {
	var doc strings.Builder
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&doc, "Paragraph %d: `if x then`, `= state.n`, `show(\"a\")` and `end`.\n\n", i)
	}
	source := []byte(doc.String())
	for _, language := range []string{"", "lua"} {
		b.Run(fmt.Sprintf("language=%q", language), func(b *testing.B) {
			for b.Loop() {
				Compile(source, Options{Plugins: []codeblocks.Plugin{chromahighlight.Plugin{}}, InlineCodeLanguage: language})
			}
		})
	}
}
