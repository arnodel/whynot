//go:build !js

package browser

import (
	"strings"
	"testing"
)

// TestInputFilter checks that input blocks are taken out of a page's
// Markdown, written in pieces, and the last one's field recorded, while
// other code blocks, even ones holding an input fence, pass through.
func TestInputFilter(t *testing.T) {
	page := "Hello.\n\n" +
		"```text\n```input not this one\n```\n\n" +
		"```input first\nIgnored\n```\n\n" +
		"~~~~input say #reply\nSay\nsomething…\n~~~~\n\n" +
		"Bye.\n"
	var out strings.Builder
	var input pageInput
	f := &inputFilter{w: &out, input: &input}
	for i := 0; i < len(page); i += 7 {
		f.Write([]byte(page[i:min(i+7, len(page))]))
	}
	f.Close()
	want := "Hello.\n\n```text\n```input not this one\n```\n\n\n\nBye.\n"
	if out.String() != want {
		t.Errorf("the page passed on is %q, want %q", out.String(), want)
	}
	if got := input.get(); got == nil || *got != (inputField{name: "say", target: "#reply", placeholder: "Say something…"}) {
		t.Errorf("the field is %+v, want say, to #reply, with its placeholder", got)
	}
}

// TestInputFilterStreams checks that an ordinary line is passed on as it
// arrives, before its end, while one that may be a fence or a controls
// heading waits for its end.
func TestInputFilterStreams(t *testing.T) {
	var out, source strings.Builder
	var input pageInput
	f := &inputFilter{w: &out, source: &source, input: &input}
	for _, c := range []struct{ write, want string }{
		{"Once upon", "Once upon"},
		{" a time.\n\n", "Once upon a time.\n\n"},
		{"   ", "Once upon a time.\n\n"},
		{"``", "Once upon a time.\n\n"},
		{"`input say\nSay…\n```\n", "Once upon a time.\n\n"},
		{"## (Contr", "Once upon a time.\n\n"},
		{"ols)\n", "Once upon a time.\n\n"},
		{"    indented", "Once upon a time.\n\n    indented"},
	} {
		f.Write([]byte(c.write))
		if out.String() != c.want {
			t.Errorf("after %q, passed on %q, want %q", c.write, out.String(), c.want)
		}
	}
	f.Close()
	if want := "Once upon a time.\n\n## (Controls)\n    indented"; source.String() != want {
		t.Errorf("the source is %q, want %q", source.String(), want)
	}
	if got := input.get(); got == nil || got.name != "say" {
		t.Errorf("the field is %+v, want say", got)
	}
}

// TestInputInBrowser checks a whymark page's input field: its
// placeholder, sending text as a turn, and drafts kept per page.
func TestInputInBrowser(t *testing.T) {
	dir := t.TempDir()
	start := writeTempMD(t, dir, "start.md", "# Start\n\n[Talk](talk.md)\n")
	writeTempMD(t, dir, "talk.md", "---\nwhymark:\n  version: 1\n---\n# Talk\n\n"+
		"`if turn.input.say then`\nYou said: `= turn.input.say`.\n`end`\n\n"+
		"```input say\nSay something…\n```\n")
	app := newTestApp(t, dir, start, "# Start\n\n[Talk](talk.md)\n")
	if _, ok := app.Input(); ok {
		t.Error("a plain page has an input field")
	}
	app.Follow("talk.md")
	placeholder, ok := app.Input()
	if !ok || placeholder != "Say something…" {
		t.Fatalf("Input() = %q, %v, want the block's placeholder", placeholder, ok)
	}
	if strings.Contains(pageText(app), "Say something") {
		t.Error("the input block shows on the page")
	}
	_, firstVersion := app.InputDraft()
	app.SetInputDraft("hel")

	app.SubmitInput("hello")
	if !strings.Contains(pageText(app), "You said: hello") {
		t.Errorf("after sending, the page shows %q", pageText(app))
	}
	if draft, version := app.InputDraft(); draft != "" || version == firstVersion {
		t.Errorf("the new page's draft is %q, version %d, want empty, and a new version", draft, version)
	}
	app.SetInputDraft("half-typed")
	app.Back()
	if draft, _ := app.InputDraft(); draft != "hello" {
		t.Errorf("back on the first page, the draft is %q, want what was sent from it", draft)
	}
	app.Forward()
	if draft, _ := app.InputDraft(); draft != "half-typed" {
		t.Errorf("forward again, the draft is %q, want what was typed", draft)
	}
}
