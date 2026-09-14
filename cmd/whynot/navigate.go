package main

import (
	"errors"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
)

// setTheme switches between whynot's built-in dark and light
// StyleSheets - the L/D keyboard shortcuts and the toolbar's theme
// button both go through this, so darkTheme (what the theme button's
// icon is chosen from) never drifts out of sync with styleSheet.
func (g *game) setTheme(dark bool) {
	if dark {
		g.setStyleSheet(whynot.NewDarkStyleSheet())
	} else {
		g.setStyleSheet(whynot.NewLightStyleSheet())
	}
	g.darkTheme = dark
}

func (g *game) setStyleSheet(s whynot.StyleSheet) {
	g.styleSheet = s
	g.current.view.SetStyleSheet(s)
}

// updateWindowTitle sets the OS window title to the current document's
// own title (View.Title: its first heading, any level), or a generic
// fallback if it has none - call whenever g.current.view is replaced
// with a different document's.
func (g *game) updateWindowTitle() {
	title, ok := g.current.view.Title()
	if !ok {
		title = "Untitled document"
	}
	ebiten.SetWindowTitle(title)
}

// resolveLink parses dest and resolves it against the current
// document's own location - what a relative or fragment-only link is
// relative to. Used both to show where a hovered link actually points
// (in the address bar, since dest alone is just the literal Markdown
// destination text) and to navigate there on click.
func (g *game) resolveLink(dest string) (*url.URL, error) {
	return resolveAgainst(g.current.location, dest)
}

// newView builds a View for source, loaded from location - bundling
// the options every call site needs together: the current StyleSheet,
// and an ImageSource that resolves an image's src against location the
// same way resolveLink resolves a link's href, so a relative or
// http(s) image works regardless of where its document came from. The
// welcome page is the one exception: its own images (if any) are
// bundled alongside it in assetsFS, not fetched, so it gets
// welcomeImageSource instead of the general fetch-based one.
func (g *game) newView(source []byte, location *url.URL) *whynot.View {
	imageSource := whynot.ImageSource(docImageSource{base: location})
	if location.Scheme == "whynot" {
		imageSource = welcomeImageSource{}
	}
	return whynot.NewView(source, g.faceSelector,
		whynot.WithStyleSheet(g.styleSheet),
		whynot.WithImageSource(imageSource),
	)
}

// follow resolves dest against the current document's own location -
// so a relative link works whether the current document came from
// disk or from an http(s) fetch. A fragment-only link to the current
// document (e.g. a table-of-contents entry) just scrolls in place,
// reusing the current View; otherwise the new document is loaded, laid
// out immediately (so a fragment can be resolved into it right away),
// and replaces it. Either way, wherever the jump started from is
// pushed onto history first, so back can return to it.
func (g *game) follow(dest string) {
	resolved, err := g.resolveLink(dest)
	if err != nil {
		log.Printf("link destination %q: %v", dest, err)
		return
	}

	if samePage(g.current.location, resolved) {
		if resolved.Fragment == "" {
			return
		}
		g.pushHistory()
		g.current.view.ScrollToAnchor(resolved.Fragment)
		// resolved (unlike g.current.location) carries the fragment, so
		// the address bar reflects the jump even though the document
		// itself didn't change.
		g.current.location = resolved
		return
	}

	source, err := loadDocument(resolved)
	if err != nil {
		var htmlErr *htmlContentError
		if errors.As(err, &htmlErr) {
			openInBrowser(resolved.String())
			return
		}
		log.Printf("loading %s: %v", resolved, err)
		return
	}
	view := g.newView(source, resolved)
	view.Layout(g.width, g.scale, g.elapsed())
	if resolved.Fragment != "" {
		view.ScrollToAnchor(resolved.Fragment)
	}
	g.pushHistory()
	g.current = document{location: resolved, view: view}
	g.updateWindowTitle()
}

// pushHistory saves the current document and scroll position onto
// history, so back can return to it - and clears future, the same way
// a browser discards forward history once you navigate anywhere new
// rather than pressing its forward button.
func (g *game) pushHistory() {
	g.history = append(g.history, historyEntry{
		document: g.current,
		scroll:   g.current.view.ScrollPosition(),
	})
	g.future = nil
}

// samePage reports whether a and b name the same document, ignoring
// any URL fragment - so a link differing only by '#anchor' scrolls
// instead of reloading.
func samePage(a, b *url.URL) bool {
	x, y := *a, *b
	x.Fragment, x.RawFragment = "", ""
	y.Fragment, y.RawFragment = "", ""
	return x.String() == y.String()
}

// back pops the most recently visited place, if any - a no-op at the
// start of history.
func (g *game) back() {
	if len(g.history) == 0 {
		return
	}
	entry := g.history[len(g.history)-1]
	g.history = g.history[:len(g.history)-1]
	g.travelTo(entry, &g.future)
}

// forward undoes the last back, if any - a no-op with nothing to redo,
// and cleared by any new navigation (see pushHistory), same as a
// browser's own forward button.
func (g *game) forward() {
	if len(g.future) == 0 {
		return
	}
	entry := g.future[len(g.future)-1]
	g.future = g.future[:len(g.future)-1]
	g.travelTo(entry, &g.history)
}

// travelTo makes entry the current document, first pushing the one
// being left behind onto undoStack (g.future when going back, g.history
// when going forward) so the trip itself can be undone. entry's View
// may be stale if the window was resized or the theme toggled while it
// wasn't current, so its scroll position is restored first, then its
// StyleSheet and layout are refreshed - in that order, so each
// rebuild's ratio-preserving cursor logic works from the right
// position.
func (g *game) travelTo(entry historyEntry, undoStack *[]historyEntry) {
	*undoStack = append(*undoStack, historyEntry{
		document: g.current,
		scroll:   g.current.view.ScrollPosition(),
	})
	entry.view.RestoreScrollPosition(entry.scroll)
	entry.view.SetStyleSheet(g.styleSheet)
	entry.view.Layout(g.width, g.scale, g.elapsed())
	g.current = entry.document
	g.updateWindowTitle()
}

// reload re-fetches the current document's own location and replaces
// its View in place - not pushed onto history, since it's still the
// same place, just re-read. Scroll position is carried over to the new
// View the same way it already is across a resize or theme change.
func (g *game) reload() {
	source, err := loadDocument(g.current.location)
	if err != nil {
		var htmlErr *htmlContentError
		if errors.As(err, &htmlErr) {
			openInBrowser(g.current.location.String())
			return
		}
		log.Printf("reloading %s: %v", g.current.location, err)
		return
	}
	scroll := g.current.view.ScrollPosition()
	view := g.newView(source, g.current.location)
	view.Layout(g.width, g.scale, g.elapsed())
	view.RestoreScrollPosition(scroll)
	g.current.view = view
	g.updateWindowTitle()
}

// paste navigates to the clipboard's current text content, if it's the
// word "welcome" (see welcomeURL), an http(s) URL, or an existing local
// file path - unlike follow, this is always treated as an absolute
// destination, never resolved relative to the current document, since
// pasting is a user-initiated "go here," not a link inside whatever's
// currently on screen.
func (g *game) paste() {
	text, err := readClipboard()
	if err != nil {
		log.Printf("reading clipboard: %v", err)
		return
	}
	text = strings.TrimSpace(text)

	var resolved *url.URL
	if strings.EqualFold(text, "welcome") {
		resolved = welcomeURL
	} else if u, err := url.Parse(text); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		resolved = u
	} else if _, err := os.Stat(text); err == nil {
		if abs, err := absFileURL(text); err == nil {
			resolved = abs
		}
	}
	if resolved == nil {
		log.Printf("clipboard content %q isn't \"welcome\", a URL, or an existing file path", text)
		return
	}

	source, err := loadDocument(resolved)
	if err != nil {
		var htmlErr *htmlContentError
		if errors.As(err, &htmlErr) {
			openInBrowser(resolved.String())
			return
		}
		log.Printf("loading %s: %v", resolved, err)
		return
	}
	view := g.newView(source, resolved)
	view.Layout(g.width, g.scale, g.elapsed())
	g.pushHistory()
	g.current = document{location: resolved, view: view}
	g.updateWindowTitle()
}
