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
// simple enough, that sharing the input layer isn't worth it.
package browser

import (
	"errors"
	"image"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/codeblocks/chromahighlight"
	"github.com/arnodel/whynot/codeblocks/kroki"
	"github.com/arnodel/whynot/fetch"
	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/styles/simpletheme"
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
	Panel *whynot.Panel

	// OnTitleChange is called with the current document's own title
	// (see documentTitle, or a generic fallback) whenever it changes -
	// wired to whichever backend-specific window-title API is
	// available (e.g. ebiten.SetWindowTitle).
	OnTitleChange func(title string)

	// OnDocumentChange, if set before documents are loaded, is called
	// whenever more of a document still arriving has come in, on a
	// goroutine of its own: for a program that draws only when asked, as
	// Gio does, to ask for a frame.
	OnDocumentChange func()
	// title is the title last given to OnTitleChange.
	title string
	// frontMatter is the raw front matter frontMatterTitle last read, and
	// frontMatterTitle its title.
	frontMatter, frontMatterTitle string

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

	faceSelector fonts.FaceSelector
	styleSheet   whynot.StyleSheet
	// registry is what documents, and their images, can be fetched from.
	registry *fetch.Registry
	// parser parses every document, with the code-block plugins.
	parser *whynot.Parser
	// arrival is what's left to do as the current document arrives.
	arrival arrival
	// journal is journal mode's state, nil outside it (see ToggleJournal).
	journal *journal
	// scrollbar is whether the documents shown draw their own scrollbar
	// (see SetScrollbar).
	scrollbar bool
	// darkTheme tracks which of the two built-in stylesheets is current,
	// for a theme-toggle UI - a StyleSheet is opaque, so it can't be
	// recovered from styleSheet itself.
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
	// recent inputs/outputs - see its own doc comment. bounds is where
	// documents are shown: Panel's bounds, except in journal mode, where
	// Panel isn't shown and its bounds aren't kept up to date.
	deviceScale                  float64
	width, height, toolbarHeight int
	bounds                       image.Rectangle

	// tocDocView is the real document's View, saved here while a
	// synthetic table-of-contents View is current in Panel instead - nil
	// whenever the TOC isn't showing (see ShowTOC/HideTOC in toc.go).
	// Every method below that reads or replaces a.Panel.View() checks
	// this first, since that's the TOC, not the document, while it's set.
	tocDocView *whynot.View
}

// NewApp constructs an App with no Panel yet and no current location -
// call NewView to build the first document's View, construct the real
// Panel from it, assign it to Panel, then call Open to make it current.
// Documents and their images are fetched through registry (see
// NewRegistry).
func NewApp(faceSelector fonts.FaceSelector, styleSheet whynot.StyleSheet, dark bool, registry *fetch.Registry) *App {
	return &App{
		faceSelector: faceSelector,
		styleSheet:   styleSheet,
		registry:     registry,
		// kroki first, so a mermaid fence is a diagram, then chroma for
		// syntax-colored code (and a diagram's source while it loads).
		parser: whynot.NewParser(
			whynot.WithImageRegistry(registry),
			whynot.WithCodeBlockPlugin(kroki.Plugin{}),
			whynot.WithCodeBlockPlugin(chromahighlight.Plugin{}),
		),
		darkTheme: dark,
		zoom:      1,
	}
}

// arrival is what's left to do as a document arrives, until the reader
// scrolls: jump to the heading its URL's fragment names, once it has
// arrived, or for "#end", keep its end in view. In journal mode, a page
// without a fragment is revealed below the trail as it arrives (see
// ViewStack.Reveal).
type arrival struct {
	fragment string
	end      bool
	reveal   bool
}

// Open makes location the current document - for the very first
// document only, after Panel has been assigned (it reads
// Panel.View().Document().Title()); every later navigation goes through Follow/
// Reload/Paste/Back/Forward instead, which also push history.
func (a *App) Open(location *url.URL) {
	a.location = location
	a.arrive(location.Fragment)
	a.updateWindowTitle()
}

// Update carries on with what showing the current document needs as it
// arrives: the jump to its URL's fragment once that heading has arrived,
// keeping its end in view for "#end", and its title. Call it once a
// frame, before drawing.
func (a *App) Update() {
	if a.tocDocView == nil {
		a.continueArrival()
	}
	a.updateWindowTitle()
}

// arrive starts the current document's arrival at fragment (see arrival).
func (a *App) arrive(fragment string) {
	switch {
	case strings.EqualFold(fragment, "end"):
		a.arrival = arrival{end: true}
	case fragment == "" && a.journal != nil:
		a.arrival = arrival{reveal: true}
	default:
		a.arrival = arrival{fragment: fragment}
	}
	a.continueArrival()
}

// continueArrival does what it can of the current document's arrival.
// Whether the document is complete is read first: once it is, the jump
// either happens or never will.
func (a *App) continueArrival() {
	view := a.Panel.View()
	complete := view.Document().Complete()
	if j := a.journal; j != nil {
		last := len(j.stack.views) - 1
		switch {
		case a.arrival.end:
			j.stack.ScrollToEnd()
			a.arrival.end = !complete
		case a.arrival.fragment != "":
			if j.stack.scrollToFragment(last, a.arrival.fragment) || complete {
				a.arrival.fragment = ""
			}
		case a.arrival.reveal:
			j.stack.Reveal(last)
			a.arrival.reveal = !complete
		}
		return
	}
	switch {
	case a.arrival.end:
		view.ScrollToEnd()
		a.arrival.end = !complete
	case a.arrival.fragment != "":
		if scrollToFragment(view, a.arrival.fragment) || complete {
			a.arrival.fragment = ""
		}
	}
}

// Bounds returns where documents are shown, as Relayout last set it.
func (a *App) Bounds() image.Rectangle { return a.bounds }

// Location returns where the current document was loaded from.
func (a *App) Location() *url.URL { return a.location }

// HoverDest returns where the link under the cursor points, resolved
// against the current document's location, or "" if there's none: what
// an address bar shows instead of Location while hovering.
func (a *App) HoverDest() string {
	if a.Panel == nil {
		return ""
	}
	view, location := a.Panel.View(), a.location
	if j := a.journal; j != nil {
		view = nil
		for i, v := range j.stack.views {
			if _, ok := v.HoveredLink(); ok {
				view, location = v, j.pages[i].location
			}
		}
		if view == nil {
			return ""
		}
	}
	dest, ok := view.HoveredLink()
	if !ok {
		return ""
	}
	resolved, err := resolveAgainst(location, dest)
	if err != nil {
		return ""
	}
	return resolved.String()
}

// HandleEvents follows the links clicked in Panel, as reported by
// Panel.Frame, and stops what's left of the current document's arrival
// (see Update) when the reader scrolls. Panel's own anchor scrolling must
// be off (see Panel.SetAnchorScrolling), since FollowAnchor scrolls,
// after recording history.
func (a *App) HandleEvents(events []whynot.Event) {
	for _, e := range events {
		switch e := e.(type) {
		case whynot.LinkClick:
			a.Follow(e.Destination)
		case whynot.AnchorClick:
			a.FollowAnchor(e.ID)
		case whynot.Scroll:
			a.arrival = arrival{}
		}
	}
}

// CanGoBack/CanGoForward report whether Back/Forward would do
// anything. CanGoBack is also true while the TOC is showing, since Back
// dismisses it (see tocDocView); CanGoForward is false there instead.
func (a *App) CanGoBack() bool    { return a.tocDocView != nil || len(a.history) > 0 }
func (a *App) CanGoForward() bool { return a.tocDocView == nil && len(a.future) > 0 }

// CanReload reports whether Reload would do anything.
func (a *App) CanReload() bool { return a.tocDocView == nil }

// Zoom returns the current zoom multiplier (1.0 = 100%).
func (a *App) Zoom() float64 { return a.zoom }

// DarkTheme reports whether the current StyleSheet is the dark one.
func (a *App) DarkTheme() bool { return a.darkTheme }

// StyleSheet returns the current StyleSheet.
func (a *App) StyleSheet() whynot.StyleSheet { return a.styleSheet }

// SetTheme switches between whynot's built-in dark and light
// StyleSheets - keeps styleSheet and darkTheme in sync so darkTheme
// never drifts out of what a caller's theme-toggle UI (e.g. which icon
// to show) reads.
func (a *App) SetTheme(dark bool) {
	if dark {
		a.styleSheet = simpletheme.DarkStyleSheet
	} else {
		a.styleSheet = simpletheme.LightStyleSheet
	}
	a.darkTheme = dark
	a.Panel.SetStyleSheet(a.styleSheet)
	if j := a.journal; j != nil {
		for _, v := range j.stack.views {
			v.SetStyleSheet(a.styleSheet)
		}
		j.stack.SetColors(a.stackColors())
		j.stack.SetScrollbar(a.stackScrollbar())
	}
}

// updateWindowTitle fires OnTitleChange with the current document's own
// title (see documentTitle), or a generic fallback if it has none, if
// it's changed since it last fired - call whenever Panel's View is
// replaced with a different document's. While the TOC is showing, its
// own "Table of contents" heading is skipped in favor of the real
// document's title (a.tocDocView) plus a " - TOC" suffix.
func (a *App) updateWindowTitle() {
	if a.OnTitleChange == nil {
		return
	}
	view := a.Panel.View()
	if a.tocDocView != nil {
		view = a.tocDocView
	}
	title, ok := a.documentTitle(view.Document())
	if !ok {
		title = "Untitled document"
	}
	if a.tocDocView != nil {
		title += " - TOC"
	}
	if title != a.title {
		a.title = title
		a.OnTitleChange(title)
	}
}

// documentTitle returns doc's title: the title its front matter gives,
// else its first heading's text.
func (a *App) documentTitle(doc *whynot.Document) (string, bool) {
	if raw := doc.RawFrontMatter(); raw != "" {
		if raw != a.frontMatter {
			a.frontMatter, a.frontMatterTitle = raw, frontMatterTitle(raw)
		}
		if a.frontMatterTitle != "" {
			return a.frontMatterTitle, true
		}
	}
	return doc.Title()
}

// ResolveLink parses dest and resolves it against the current
// document's own location - what a relative or fragment-only link is
// relative to. Used both to show where a hovered link actually points
// (e.g. in an address bar, since dest alone is just the literal
// Markdown destination text) and to navigate there on click.
func (a *App) ResolveLink(dest string) (*url.URL, error) {
	return resolveAgainst(a.location, dest)
}

// Load fetches the document at location (see NewRegistry), returning it
// as it arrives: complete if it arrives quickly, otherwise growing as the
// rest comes in, in the background. Content that isn't Markdown is an
// error, and a web page is a *webPageError, which App opens in a web
// browser instead.
func (a *App) Load(location *url.URL) (*whynot.Document, error) {
	doc, err := loadDocument(a.parser, a.registry, location)
	if err != nil {
		return nil, loadError(err, location)
	}
	if f := a.OnDocumentChange; f != nil {
		go func() {
			for range doc.Updates() {
				f()
			}
		}()
	}
	return doc, nil
}

// NewView returns a View of doc with the current fonts and StyleSheet.
func (a *App) NewView(doc *whynot.Document) *whynot.View {
	return whynot.NewView(
		doc,
		whynot.WithFaceSelector(a.faceSelector),
		whynot.WithStyleSheet(a.styleSheet),
	)
}

// Follow resolves dest against the current document's own location -
// so a relative link works whether the current document came from
// disk or from an http(s) fetch. A fragment-only link to the current
// document just scrolls in place, reusing the current View; otherwise
// the new document is loaded, laid out immediately (so a fragment can
// be resolved into it right away), and replaces it. Either way,
// wherever the jump started from is pushed onto history first, so Back
// can return to it.
//
// While the TOC is showing, dest is always one of its own "#id" links:
// resolved against a.location as usual, but the jump lands on
// a.tocDocView, which becomes current again, closing the TOC.
func (a *App) Follow(dest string) {
	resolved, err := a.ResolveLink(dest)
	if err != nil {
		log.Printf("link destination %q: %v", dest, err)
		return
	}
	a.follow(resolved)
}

// FollowAnchor is Follow for a link within the current document to the
// heading with the given id, already percent-decoded, as in a
// whynot.AnchorClick.
func (a *App) FollowAnchor(id string) {
	resolved := *a.location
	resolved.Fragment, resolved.RawFragment = id, ""
	a.follow(&resolved)
}

// follow is Follow once dest is resolved.
func (a *App) follow(resolved *url.URL) {
	if a.tocDocView != nil {
		docView := a.tocDocView
		a.pushHistoryFor(a.location, docView)
		a.Panel.SetView(docView)
		a.tocDocView = nil
		if resolved.Fragment != "" {
			a.arrive(resolved.Fragment)
		}
		a.location = resolved
		a.updateWindowTitle()
		return
	}

	if samePage(a.location, resolved) {
		if resolved.Fragment == "" {
			return
		}
		a.pushHistory()
		a.arrive(resolved.Fragment)
		// resolved (unlike a.location) carries the fragment, so a
		// caller's address bar reflects the jump even though the
		// document itself didn't change.
		a.location = resolved
		return
	}

	doc, err := a.Load(resolved)
	if err != nil {
		if !openIfWebPage(err, resolved) {
			log.Printf("loading %s: %v", resolved, err)
		}
		return
	}
	a.show(doc, resolved)
}

// show makes doc, loaded from location, the current document, pushing
// the one being left onto history, and scrolls to location's fragment.
func (a *App) show(doc *whynot.Document, location *url.URL) {
	view := a.NewView(doc)
	a.place(view)
	a.pushHistory() // must run before SetView - it reads the page being left
	a.Panel.SetView(view)
	a.location = location
	a.restack()
	a.arrive(location.Fragment)
	a.updateWindowTitle()
}

// openIfWebPage opens location in a web browser if err says it's a web
// page rather than Markdown (see webPageError), reporting whether it did.
func openIfWebPage(err error, location *url.URL) bool {
	var pageErr *webPageError
	if !errors.As(err, &pageErr) {
		return false
	}
	openInBrowser(location.String())
	return true
}

// pushHistory saves the current document and scroll position onto
// history, so Back can return to it - and clears future, the same way
// a browser discards forward history once you navigate anywhere new
// rather than pressing its forward button.
func (a *App) pushHistory() {
	a.pushHistoryFor(a.location, a.Panel.View())
}

// pushHistoryFor is pushHistory's real implementation, taking the
// location/view to save explicitly - needed by Follow's TOC branch,
// which must push a.tocDocView rather than a.Panel.View().
func (a *App) pushHistoryFor(loc *url.URL, view *whynot.View) {
	a.history = append(a.history, historyEntry{
		document: document{location: loc, view: view},
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
// start of history. While the TOC is showing, Back dismisses it instead
// - the most recently visited "place" from the user's perspective -
// without consuming a history entry.
func (a *App) Back() {
	if a.tocDocView != nil {
		a.HideTOC()
		return
	}
	if len(a.history) == 0 {
		return
	}
	entry := a.history[len(a.history)-1]
	a.history = a.history[:len(a.history)-1]
	a.travelTo(entry, &a.future)
}

// Forward undoes the last Back, if any - a no-op with nothing to redo,
// and cleared by any new navigation (see pushHistory), same as a
// browser's own forward button. Also a no-op while the TOC is showing,
// unlike Back - disabled rather than dismissing it too.
func (a *App) Forward() {
	if a.tocDocView != nil || len(a.future) == 0 {
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
	a.arrival = arrival{}
	a.restack()
	a.updateWindowTitle()
}

// Reload re-fetches the current document's own location and replaces
// its View in place - not pushed onto history, since it's still the
// same place, just re-read. Scroll position is carried over to the new
// View the same way it already is across a resize or theme change. A
// no-op while the TOC is showing (see CanReload).
func (a *App) Reload() {
	if a.tocDocView != nil {
		return
	}
	doc, err := a.Load(a.location)
	if err != nil {
		if !openIfWebPage(err, a.location) {
			log.Printf("reloading %s: %v", a.location, err)
		}
		return
	}
	scroll := a.Panel.View().ScrollPosition()
	view := a.NewView(doc)
	a.place(view)
	view.RestoreScrollPosition(scroll)
	a.Panel.SetView(view)
	a.arrival = arrival{}
	a.restack()
	a.updateWindowTitle()
}

// Paste navigates to the clipboard's current text content - see
// Navigate for what "navigates to" means and why this is unlike Follow.
func (a *App) Paste() {
	text, err := readClipboard()
	if err != nil {
		log.Printf("reading clipboard: %v", err)
		return
	}
	a.Navigate(strings.TrimSpace(text))
}

// Navigate resolves text - the word "welcome" (see WelcomeURL), an
// http(s) URL, or an existing local file path, the same as
// ResolveLocationArg - loads it, and replaces the current document,
// pushing history first same as Follow. Unlike Follow, text is always
// treated as an absolute destination, never resolved relative to the
// current document, since this is a user-initiated "go here" (typed
// into an address bar, or pasted - see Paste), not a link inside
// whatever's currently on screen.
//
// Returns the resolve/load error, if any, so a caller with somewhere to
// show it (e.g. an editable address bar) can - an HTML response is
// still handled the same as Follow (opened in a web browser) and
// reported as no error, since that's not a mistake for the caller to
// show. A no-op (nil error) while the TOC is showing, same as Reload.
func (a *App) Navigate(text string) error {
	if a.tocDocView != nil {
		return nil
	}
	resolved, err := ResolveLocationArg(text)
	if err != nil {
		log.Printf("navigating to %q: %v", text, err)
		return err
	}

	doc, err := a.Load(resolved)
	if err != nil {
		if openIfWebPage(err, resolved) {
			return nil
		}
		log.Printf("loading %s: %v", resolved, err)
		return err
	}
	a.show(doc, resolved)
	return nil
}

// zoomStep is a fixed step of the original (100%) size, not of the
// current zoom - so repeated zoom-in/out goes 100%, 110%, 120%, ...
// rather than steps shrinking as you zoom out or growing as you zoom
// in.
const zoomStep = 0.1

// place lays view out where Panel shows its View, so it can be scrolled
// (e.g. to an anchor) before it's shown.
func (a *App) place(view *whynot.View) {
	view.SetScale(a.deviceScale)
	view.SetZoom(a.zoom)
	view.SetBounds(a.bounds)
}

// scrollToFragment scrolls v to the anchor a URL fragment names, the way
// browsers do: the heading with that id, else the top of the document
// for an empty fragment or "top". It reports whether it scrolled.
func scrollToFragment(v *whynot.View, id string) bool {
	if v.ScrollToAnchor(id) {
		return true
	}
	if id == "" || strings.EqualFold(id, "top") {
		v.ScrollToRatio(0)
		return true
	}
	return false
}

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

// Relayout recomputes width/height from outsideWidth/
// outsideHeight (the window's logical/device-independent size),
// deviceScale (the display's own scale, with no zoom applied), and
// toolbarHeight (physical pixels already reserved above the document,
// laid out by the caller - toolbar layout stays backend-specific, see
// this package's own doc comment), then applies them to Panel, or in
// journal mode to the stack of pages (see ToggleJournal). Cheap
// when nothing changed; call once a frame, the same "called
// unconditionally" pattern Panel.Draw itself already uses (so e.g. an
// animated GIF gets fresh timing every rendered frame).
func (a *App) Relayout(outsideWidth, outsideHeight int, deviceScale float64, toolbarHeight int) {
	a.deviceScale = deviceScale
	a.width = int(float64(outsideWidth) * deviceScale)
	a.height = int(float64(outsideHeight) * deviceScale)
	a.toolbarHeight = toolbarHeight
	a.bounds = image.Rect(0, toolbarHeight, a.width, a.height)
	j := a.journal
	if j == nil {
		a.Panel.SetBounds(a.bounds)
		a.Panel.SetScale(deviceScale)
		a.Panel.SetZoom(a.zoom)
		return
	}
	// The current page is in the stack, which gives it its bounds: Panel
	// would give it its own.
	changed := j.stack.bounds != a.bounds
	for _, v := range j.stack.views {
		if v.Scale() != deviceScale || v.Zoom() != a.zoom {
			v.SetScale(deviceScale)
			v.SetZoom(a.zoom)
			changed = true
		}
	}
	if changed {
		j.stack.SetBounds(a.bounds)
	}
}
