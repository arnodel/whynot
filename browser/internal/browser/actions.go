package browser

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/arnodel/whynot/input"
)

// mouseAfterTouch is how long after a touch mouse input doesn't count as
// the reader using a mouse: web browsers follow a tap with mouse events of
// their own.
const mouseAfterTouch = time.Second

// noteInput notes, from events at now, whether the reader is using touch
// or a mouse and keyboard, for the actions page to leave out keys for
// touch.
func (a *App) noteInput(events []input.Event, now time.Duration) {
	for _, e := range events {
		switch e.(type) {
		case input.TouchStart, input.TouchMove:
			a.touch, a.lastTouch = true, now
		case input.PointerMove, input.PointerButton, input.Wheel:
			if now-a.lastTouch > mouseAfterTouch {
				a.touch = false
			}
		}
	}
}

// ActionsShowing reports whether the actions page is showing (see
// ShowActions).
func (a *App) ActionsShowing() bool { return a.overlay == actionsOverlay }

// ShowActions shows the actions page over the current document: what the
// browser can do, as links that do it, with their keys. It's generated
// for the current state ("Turn journal mode on", or "off"), and generated
// again after an action that shows on the page itself, such as zooming,
// so that it can be repeated; an action that changes what's shown, such
// as journal mode, closes the page first. Only the actions page's own
// links run actions: they're "whynot:" links (see Follow).
func (a *App) ShowActions() {
	a.copied = false
	a.showOverlay(actionsOverlay, a.actionsSource())
}

// ToggleActions shows the actions page, or hides it if it's showing.
func (a *App) ToggleActions() {
	if a.overlay == actionsOverlay {
		a.hideOverlay()
		return
	}
	a.ShowActions()
}

// refreshActions generates the actions page again, for the state an
// action left, at the same place.
func (a *App) refreshActions() {
	start, _ := a.Panel.View().VisibleRange()
	view := a.overlayView(a.actionsSource())
	view.ScrollToRatio(start)
	a.Panel.SetView(view)
}

// copyAction is the actions page's copy link, page for a page, or journal
// in journal mode, followed by how the last copy went, if it ran since
// the page was shown.
func (a *App) copyAction(page, journal string) string {
	text := page
	if a.journal != nil {
		text = journal
	}
	switch {
	case !a.copied:
		return text
	case a.copyErr != nil:
		return text + " · *couldn't copy: " + escapeMarkdown(a.copyErr.Error()) + "*"
	}
	return text + " · *copied*"
}

// actionsSource generates the Markdown source of the actions page.
func (a *App) actionsSource() []byte {
	link := func(name, text string) string { return "[" + text + "](whynot:" + name + ")" }
	// unless is text, without its link if the action can't be done now.
	unless := func(cant bool, name, text string) string {
		if cant {
			return text
		}
		return link(name, text)
	}
	journal, journalAction := "off", link("journal", "turn on")
	if a.journal != nil {
		journal, journalAction = "on", link("journal", "turn off")
	}
	theme, themeAction := "dark", link("theme", "switch to light")
	if !a.darkTheme {
		theme, themeAction = "light", link("theme", "switch to dark")
	}

	// With touch, keys are left out.
	keys := !a.touch

	var b strings.Builder
	b.WriteString("# Actions\n\n")
	// Settings come first, zoom at the top: what zooming moves is what's
	// above it.
	if keys {
		b.WriteString("| | Now | | Keys |\n|---|---|---|---|\n")
	} else {
		b.WriteString("| | Now | |\n|---|---|---|\n")
	}
	for _, row := range [][4]string{
		{"Zoom", fmt.Sprintf("%d%%", int(math.Round(a.zoom*100))),
			link("zoom-in", "zoom in") + " · " + link("zoom-out", "zoom out") + " · " + link("zoom-reset", "reset"),
			keysOf("zoom-in", "zoom-out", "zoom-reset")},
		{"Theme", theme, themeAction, keysOf("theme")},
		{"Journal mode", journal, journalAction, keysOf("journal")},
	} {
		fmt.Fprintf(&b, "| %s | **%s** | %s |", row[0], row[1], row[2])
		if keys {
			fmt.Fprintf(&b, " %s |", row[3])
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	table := func(heading string, rows [][2]string) {
		fmt.Fprintf(&b, "## %s\n\n", heading)
		if !keys {
			for _, row := range rows {
				fmt.Fprintf(&b, "- %s\n", row[0])
			}
			b.WriteString("\n")
			return
		}
		b.WriteString("| | Keys |\n|---|---|\n")
		for _, row := range rows {
			fmt.Fprintf(&b, "| %s | %s |\n", row[0], row[1])
		}
		b.WriteString("\n")
	}
	table("Pages", [][2]string{
		{unless(len(a.history) == 0, "back", "Go back"), keysOf("back")},
		{unless(len(a.future) == 0, "forward", "Go forward"), keysOf("forward")},
		{link("reload", "Reload this page"), keysOf("reload")},
		{a.copyAction(link("copy", "Copy this page as Markdown"), link("copy", "Copy the journal as Markdown")), keysOf("copy")},
		{unless(!a.HasSelection(), "copy-selection", "Copy the selection as Markdown"), keysOf("copy-selection")},
		{link("select-all", "Select all of this page"), keysOf("select-all")},
		{link("paste", "Open the path or URL in the clipboard"), keysOf("paste")},
		{unless(!a.CanShowTOC(), "toc", "Show the table of contents"), keysOf("toc")},
	})
	if !keys {
		return []byte(b.String()) // reading is all keys
	}
	table("Reading", [][2]string{
		{"Scroll a step", keysOf("scroll-down", "scroll-up")},
		{"Scroll a page", keysOf("page-down", "page-up")},
		{"Scroll a wide code block or table sideways", keysOf("scroll-left", "scroll-right")},
		{"Close this page or the table of contents", keysOf("close")},
		{"Show or hide this page", keysOf("actions")},
	})
	return []byte(b.String())
}
