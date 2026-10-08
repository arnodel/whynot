package browser

import (
	"strings"
	"testing"

	"github.com/arnodel/whynot/codeblocks"
	"github.com/arnodel/whynot/codeblocks/chromahighlight"
)

// TestAIBlockHighlighting checks that an ai block's prose is a
// comment and its code spans are Lua, and that its spans still add up to
// the block's source.
func TestAIBlockHighlighting(t *testing.T) {
	code := "Describe the shop.\n`if has(\"oil\") then`\nThey hold ``oil``.\n`end`\n"
	content, ok := chromahighlight.Plugin{}.Parse("ai", code).(codeblocks.Tokens)
	if !ok {
		t.Fatal("no tokens for an ai block")
	}
	classes := map[string]string{}
	var all strings.Builder
	for _, span := range content.Spans {
		all.WriteString(span.Text)
		classes[strings.TrimSpace(span.Text)] = span.Class
	}
	if all.String() != code {
		t.Errorf("spans = %q, want them to add up to %q", all.String(), code)
	}
	for text, want := range map[string]string{
		"Describe the shop.": codeblocks.ClassComment,
		"if":                 codeblocks.ClassKeyword,
		`"oil"`:              codeblocks.ClassString,
	} {
		if classes[text] != want {
			t.Errorf("class of %q = %q, want %q", text, classes[text], want)
		}
	}
	if !(chromahighlight.Plugin{}).Handles("noai") {
		t.Error("no lexer for noai blocks")
	}
}
