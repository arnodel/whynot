package browser

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/arnodel/whynot"
)

// overlay is what Panel shows in place of the current document, if
// anything: an aside that isn't in history, and that Back dismisses.
type overlay int

const (
	noOverlay overlay = iota
	tocOverlay
	sourceOverlay
)

// documentView returns the current document's View, even while an
// overlay is showing.
func (a *App) documentView() *whynot.View {
	if a.overlaid != nil {
		return a.overlaid
	}
	return a.Panel.View()
}

// showOverlay shows view, already placed, in place of the current
// document, replacing any overlay already showing.
func (a *App) showOverlay(kind overlay, view *whynot.View) {
	a.overlaid, a.overlay = a.documentView(), kind
	a.Panel.SetView(view)
	a.updateWindowTitle()
}

// HideOverlay restores the document an overlay replaced, exactly where
// it was left - a no-op if none is showing.
func (a *App) HideOverlay() {
	if a.overlaid == nil {
		return
	}
	a.Panel.SetView(a.overlaid) // re-applies a.styleSheet, then relayouts
	a.overlaid, a.overlay = nil, noOverlay
	a.updateWindowTitle()
}

// SourceShowing reports whether ShowSource is in effect.
func (a *App) SourceShowing() bool { return a.overlay == sourceOverlay }

// ToggleSource shows the source, or hides it if it's showing.
func (a *App) ToggleSource() {
	if a.SourceShowing() {
		a.HideOverlay()
	} else {
		a.ShowSource()
	}
}

// ShowSource shows the current document's source, as an overlay like
// the TOC: its Markdown as it was shown, and for a lua: page, its
// script too. It replaces the TOC, if that's showing.
func (a *App) ShowSource() {
	if a.overlay == sourceOverlay {
		return
	}
	view := a.NewView([]byte(a.sourceMarkdown()), a.location)
	a.place(view)
	a.showOverlay(sourceOverlay, view)
}

// sourceMarkdown returns the Markdown of ShowSource's overlay.
func (a *App) sourceMarkdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Source\n\n%s\n\n", codeFence("text", a.location.String()))
	if a.location.Scheme == "lua" {
		script, err := os.ReadFile(luaScriptPath(a.location))
		if err != nil {
			log.Printf("reading %s's script: %v", a.location, err)
		} else {
			fmt.Fprintf(&b, "## Script\n\n%s\n\n## Page\n\n", codeFence("lua", string(script)))
		}
	}
	b.WriteString(codeFence("markdown", string(a.source)))
	return b.String()
}

// codeFence returns text as a fenced code block in the given language,
// with a fence longer than any run of backquotes in text.
func codeFence(language, text string) string {
	longest, run := 0, 0
	for _, r := range text {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", max(3, longest+1))
	return fence + language + "\n" + strings.TrimSuffix(text, "\n") + "\n" + fence
}
