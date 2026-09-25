// Package browser is the backend-agnostic half of a whynot document
// browser: navigation history, the current theme/zoom, document/image
// loading, the welcome page, and toolbar icons - everything cmd/whynot
// needs that isn't specific to ebiten, shaped so a Gio-based app can use
// it too. It's a separate package from the root whynot module (rather
// than living there) because it pulls in net/http, os/exec, and embed,
// none of which a caller of the core rendering library should have to
// import transitively.
//
// What's deliberately not here: toolbar layout, drawing, and input -
// cmd/whynot's hand-rolled rect-hit-testing and a future Gio app's
// native widget.Clickable-based toolbar are different enough, and
// simple enough, that sharing the input layer isn't worth it (see
// whynot.Interaction's doc comment for the same reasoning applied to
// the document panel's scrollbar, which is the same kind of exception).
package browser

import (
	"errors"
	"image"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/chromahighlight"
)

// document pairs a View with the location it was loaded from, so a
// relative link found inside it can be resolved to an absolute one
// before being followed.
type document struct {
	location *url.URL
	view     *whynot.View
}

// historyEntry is a place Back can return to: a document plus the
// scroll position it was at when navigated away from. For a
// cross-document jump, its view is that document's own, kept around
// entirely so this scroll position stays valid; for an in-page anchor
// jump, its view is the same *whynot.View that's still current, and
// restoring scroll is all Back needs to do.
type historyEntry struct {
	document
	scroll whynot.ScrollPosition
}

// App is the navigation/history/theme/zoom state machine behind a
// whynot document browser - everything cmd/whynot's own game struct
// used to own beyond ebiten's own event loop and toolbar. Panel must be
// assigned before calling any other method; NewView does not depend on
// it, specifically so the very first View can be built (to construct
// the real Panel from) before Panel itself is set - see Open.
type App struct {
	// Panel owns the current View, its layout, and all document-area
	// input handling - everything except the toolbar and navigation,
	// which stay this type's or the caller's own concern.
	Panel whynot.Panel

	// OnTitleChange is called with the current document's own title
	// (View.Title(), or a generic fallback) whenever it changes -
	// wired to whichever backend-specific window-title API is
	// available (e.g. ebiten.SetWindowTitle).
	OnTitleChange func(title string)

	// location is where the current document (Panel.View()) was loaded
	// from - kept separately from the View itself, which Panel alone
	// owns; document still pairs the two together for historyEntry,
	// where each entry owns its own *whynot.View.
	location *url.URL
	// history is the stack of places navigated away from, most recent
	// last. future is the same idea, the other direction: places Back
	// has left that Forward can return to. Any new navigation (see
	// pushHistory) clears future, same as a browser discarding forward
	// history once you branch off somewhere new.
	history, future []historyEntry

	faceSelector whynot.FaceSelector
	styleSheet   whynot.StyleSheet
	// darkTheme tracks which of styleSheet's two possible values is
	// current - NewDarkStyleSheet/NewLightStyleSheet both return the
	// same concrete type, so there's no way to recover this from
	// styleSheet itself.
	darkTheme bool

	// zoom is a user-controlled multiplier on top of the display's own
	// scale (1.0 = 100%, see SetZoom).
	zoom float64
	// zoomIndicatorUntil is when DrawZoomIndicator should stop showing
	// - reset to now plus a fixed duration on every SetZoom call, the
	// zero Time otherwise (always in the past, so it never shows before
	// the first zoom change).
	zoomIndicatorUntil time.Time

	// deviceScale, width, height, toolbarHeight are Relayout's most
	// recent inputs/outputs - see its own doc comment. scale is
	// deviceScale*zoom, passed to View.Layout as the document's own
	// font/DPI scale.
	deviceScale                  float64
	width, height, toolbarHeight int
	scale                        float64

	// hoverDest is the link under the cursor, if any - what the
	// address bar should show instead of Location() while hovering.
	// Set via OnLinkHover, wired as Panel's own OnLinkHover field.
	hoverDest string

	// start is when the app began, for whynot.RenderingContext.Time
	// (elapsed time since rendering started - what an animated GIF's
	// current frame is picked from). Only ever compared to itself via
	// elapsed(), never to a wall-clock timestamp.
	start time.Time
}

// NewApp constructs an App with no Panel yet and no current location -
// call NewView to build the first document's View, construct the real
// Panel from it, assign it to Panel, then call Open to make it current.
func NewApp(faceSelector whynot.FaceSelector, styleSheet whynot.StyleSheet, dark bool) *App {
	return &App{
		faceSelector: faceSelector,
		styleSheet:   styleSheet,
		darkTheme:    dark,
		zoom:         1,
		start:        time.Now(),
	}
}

// Open makes location the current document - for the very first
// document only, after Panel has been assigned (it reads
// Panel.View().Title()); every later navigation goes through Follow/
// Reload/Paste/Back/Forward instead, which also push history.
func (a *App) Open(location *url.URL) {
	a.location = location
	a.updateWindowTitle()
}

// Location returns where the current document was loaded from.
func (a *App) Location() *url.URL { return a.location }

// HoverDest returns the destination of the link currently under the
// cursor, or "" if none - see OnLinkHover.
func (a *App) HoverDest() string { return a.hoverDest }

// CanGoBack/CanGoForward report whether Back/Forward would do anything.
func (a *App) CanGoBack() bool    { return len(a.history) > 0 }
func (a *App) CanGoForward() bool { return len(a.future) > 0 }

// Zoom returns the current zoom multiplier (1.0 = 100%).
func (a *App) Zoom() float64 { return a.zoom }

// DarkTheme reports whether the current StyleSheet is the dark one.
func (a *App) DarkTheme() bool { return a.darkTheme }

// StyleSheet returns the current StyleSheet.
func (a *App) StyleSheet() whynot.StyleSheet { return a.styleSheet }

// elapsed is how long the app has been running - see start.
func (a *App) elapsed() time.Duration {
	return time.Since(a.start)
}

// SetTheme switches between whynot's built-in dark and light
// StyleSheets - keeps styleSheet and darkTheme in sync so darkTheme
// never drifts out of what a caller's theme-toggle UI (e.g. which icon
// to show) reads.
func (a *App) SetTheme(dark bool) {
	if dark {
		a.styleSheet = whynot.NewDarkStyleSheet()
	} else {
		a.styleSheet = whynot.NewLightStyleSheet()
	}
	a.darkTheme = dark
	a.Panel.SetStyleSheet(a.styleSheet)
}

// updateWindowTitle fires OnTitleChange with the current document's own
// title (View.Title(): its first heading, any level), or a generic
// fallback if it has none - call whenever Panel's View is replaced with
// a different document's.
func (a *App) updateWindowTitle() {
	if a.OnTitleChange == nil {
		return
	}
	title, ok := a.Panel.View().Title()
	if !ok {
		title = "Untitled document"
	}
	a.OnTitleChange(title)
}

// ResolveLink parses dest and resolves it against the current
// document's own location - what a relative or fragment-only link is
// relative to. Used both to show where a hovered link actually points
// (e.g. in an address bar, since dest alone is just the literal
// Markdown destination text) and to navigate there on click.
func (a *App) ResolveLink(dest string) (*url.URL, error) {
	return resolveAgainst(a.location, dest)
}

// OnLinkHover is Panel.OnLinkHover: dest is the raw destination text of
// whatever's under the cursor, or "" when nothing is - resolved (same
// as ResolveLink does for a click) so HoverDest reflects where a
// relative or fragment-only link actually points, not just its literal
// Markdown text.
func (a *App) OnLinkHover(dest string) {
	a.hoverDest = ""
	if dest == "" {
		return
	}
	if resolved, err := a.ResolveLink(dest); err == nil {
		a.hoverDest = resolved.String()
	}
}

// NewView builds a View for source, loaded from location - bundling
// the options every call site needs together: the current StyleSheet,
// an ImageSource that resolves an image's src against location the
// same way ResolveLink resolves a link's href (so a relative or
// http(s) image works regardless of where its document came from), and
// a chromahighlight.Highlighter for syntax-colored code blocks. The
// welcome page is the one exception for images: its own (if any) are
// bundled alongside it, not fetched, so it gets welcomeImageSource
// instead of the general fetch-based one.
func (a *App) NewView(source []byte, location *url.URL) *whynot.View {
	imageSource := whynot.ImageSource(docImageSource{base: location})
	if location.Scheme == "whynot" {
		imageSource = welcomeImageSource{}
	}
	return whynot.NewView(source, a.faceSelector,
		whynot.WithStyleSheet(a.styleSheet),
		whynot.WithImageSource(imageSource),
		whynot.WithHighlighter(chromahighlight.Highlighter{}),
	)
}

// Follow resolves dest against the current document's own location -
// so a relative link works whether the current document came from
// disk or from an http(s) fetch. A fragment-only link to the current
// document (e.g. a table-of-contents entry) just scrolls in place,
// reusing the current View; otherwise the new document is loaded, laid
// out immediately (so a fragment can be resolved into it right away),
// and replaces it. Either way, wherever the jump started from is
// pushed onto history first, so Back can return to it.
func (a *App) Follow(dest string) {
	resolved, err := a.ResolveLink(dest)
	if err != nil {
		log.Printf("link destination %q: %v", dest, err)
		return
	}

	if samePage(a.location, resolved) {
		if resolved.Fragment == "" {
			return
		}
		a.pushHistory()
		a.Panel.View().ScrollToAnchor(resolved.Fragment)
		// resolved (unlike a.location) carries the fragment, so a
		// caller's address bar reflects the jump even though the
		// document itself didn't change.
		a.location = resolved
		return
	}

	source, err := LoadDocument(resolved)
	if err != nil {
		var htmlErr *htmlContentError
		if errors.As(err, &htmlErr) {
			openInBrowser(resolved.String())
			return
		}
		log.Printf("loading %s: %v", resolved, err)
		return
	}
	view := a.NewView(source, resolved)
	view.Layout(a.width, a.height-a.toolbarHeight, a.scale, a.elapsed())
	if resolved.Fragment != "" {
		view.ScrollToAnchor(resolved.Fragment)
	}
	a.pushHistory() // must run before SetView - it reads the page being left
	a.Panel.SetView(view)
	a.location = resolved
	a.updateWindowTitle()
}

// pushHistory saves the current document and scroll position onto
// history, so Back can return to it - and clears future, the same way
// a browser discards forward history once you navigate anywhere new
// rather than pressing its forward button.
func (a *App) pushHistory() {
	view := a.Panel.View()
	a.history = append(a.history, historyEntry{
		document: document{location: a.location, view: view},
		scroll:   view.ScrollPosition(),
	})
	a.future = nil
}

// samePage reports whether x and y name the same document, ignoring
// any URL fragment - so a link differing only by '#anchor' scrolls
// instead of reloading.
func samePage(x, y *url.URL) bool {
	a, b := *x, *y
	a.Fragment, a.RawFragment = "", ""
	b.Fragment, b.RawFragment = "", ""
	return a.String() == b.String()
}

// Back pops the most recently visited place, if any - a no-op at the
// start of history.
func (a *App) Back() {
	if len(a.history) == 0 {
		return
	}
	entry := a.history[len(a.history)-1]
	a.history = a.history[:len(a.history)-1]
	a.travelTo(entry, &a.future)
}

// Forward undoes the last Back, if any - a no-op with nothing to redo,
// and cleared by any new navigation (see pushHistory), same as a
// browser's own forward button.
func (a *App) Forward() {
	if len(a.future) == 0 {
		return
	}
	entry := a.future[len(a.future)-1]
	a.future = a.future[:len(a.future)-1]
	a.travelTo(entry, &a.history)
}

// travelTo makes entry the current document, first pushing the one
// being left behind onto undoStack (future when going back, history
// when going forward) so the trip itself can be undone. entry's View
// may be stale if the window was resized or the theme toggled while it
// wasn't current, so its scroll position is restored first, then
// Panel.SetView refreshes its StyleSheet and layout - in that order,
// so each rebuild's ratio-preserving cursor logic works from the right
// position.
func (a *App) travelTo(entry historyEntry, undoStack *[]historyEntry) {
	current := a.Panel.View()
	*undoStack = append(*undoStack, historyEntry{
		document: document{location: a.location, view: current},
		scroll:   current.ScrollPosition(),
	})
	entry.view.RestoreScrollPosition(entry.scroll)
	a.Panel.SetView(entry.view) // re-applies a.styleSheet, then relayouts
	a.location = entry.location
	a.updateWindowTitle()
}

// Reload re-fetches the current document's own location and replaces
// its View in place - not pushed onto history, since it's still the
// same place, just re-read. Scroll position is carried over to the new
// View the same way it already is across a resize or theme change.
func (a *App) Reload() {
	source, err := LoadDocument(a.location)
	if err != nil {
		var htmlErr *htmlContentError
		if errors.As(err, &htmlErr) {
			openInBrowser(a.location.String())
			return
		}
		log.Printf("reloading %s: %v", a.location, err)
		return
	}
	scroll := a.Panel.View().ScrollPosition()
	view := a.NewView(source, a.location)
	view.Layout(a.width, a.height-a.toolbarHeight, a.scale, a.elapsed())
	view.RestoreScrollPosition(scroll)
	a.Panel.SetView(view)
	a.updateWindowTitle()
}

// Paste navigates to the clipboard's current text content, if it's the
// word "welcome" (see WelcomeURL), an http(s) URL, or an existing local
// file path - unlike Follow, this is always treated as an absolute
// destination, never resolved relative to the current document, since
// pasting is a user-initiated "go here," not a link inside whatever's
// currently on screen.
func (a *App) Paste() {
	text, err := readClipboard()
	if err != nil {
		log.Printf("reading clipboard: %v", err)
		return
	}
	text = strings.TrimSpace(text)

	resolved, err := ResolveLocationArg(text)
	if err != nil {
		log.Printf("clipboard content: %v", err)
		return
	}

	source, err := LoadDocument(resolved)
	if err != nil {
		var htmlErr *htmlContentError
		if errors.As(err, &htmlErr) {
			openInBrowser(resolved.String())
			return
		}
		log.Printf("loading %s: %v", resolved, err)
		return
	}
	view := a.NewView(source, resolved)
	view.Layout(a.width, a.height-a.toolbarHeight, a.scale, a.elapsed())
	a.pushHistory() // must run before SetView - it reads the page being left
	a.Panel.SetView(view)
	a.location = resolved
	a.updateWindowTitle()
}

// zoomStep is a fixed step of the original (100%) size, not of the
// current zoom - so repeated zoom-in/out goes 100%, 110%, 120%, ...
// rather than steps shrinking as you zoom out or growing as you zoom
// in.
const zoomStep = 0.1

// minZoom, maxZoom clamp SetZoom to a sane range.
const minZoom, maxZoom = 0.5, 3.0

// ZoomIn/ZoomOut step the zoom level by zoomStep, clamped.
func (a *App) ZoomIn()  { a.SetZoom(a.zoom + zoomStep) }
func (a *App) ZoomOut() { a.SetZoom(a.zoom - zoomStep) }

// SetZoom changes the zoom level (1.0 = 100%), clamped to
// [minZoom, maxZoom], and re-lays-out immediately at the new scale -
// the same idea as a window resize, just user-triggered instead.
func (a *App) SetZoom(zoom float64) {
	a.zoom = max(minZoom, min(maxZoom, zoom))
	a.Relayout(int(float64(a.width)/a.deviceScale), int(float64(a.height)/a.deviceScale), a.deviceScale, a.toolbarHeight)
	a.zoomIndicatorUntil = time.Now().Add(zoomIndicatorDuration)
}

// Relayout recomputes width/height/scale from outsideWidth/
// outsideHeight (the window's logical/device-independent size),
// deviceScale (the display's own scale, with no zoom applied), and
// toolbarHeight (physical pixels already reserved above the document,
// laid out by the caller - toolbar layout stays backend-specific, see
// this package's own doc comment), then applies them to Panel. Cheap
// when nothing changed; call once a frame, the same "called
// unconditionally" pattern Panel.Draw itself already uses (so e.g. an
// animated GIF gets fresh timing every rendered frame).
func (a *App) Relayout(outsideWidth, outsideHeight int, deviceScale float64, toolbarHeight int) {
	a.deviceScale = deviceScale
	a.scale = deviceScale * a.zoom
	a.width = int(float64(outsideWidth) * deviceScale)
	a.height = int(float64(outsideHeight) * deviceScale)
	a.toolbarHeight = toolbarHeight
	a.Panel.SetBounds(image.Rect(0, toolbarHeight, a.width, a.height))
	a.Panel.SetScale(a.scale)
}
