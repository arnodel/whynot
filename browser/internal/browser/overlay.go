package browser

import "github.com/arnodel/whynot"

// overlayKind is what's shown over the current document instead of it,
// if anything: a page the browser generates, such as the table of
// contents.
type overlayKind int

const (
	noOverlay      overlayKind = iota
	tocOverlay                 // the table of contents (see ShowTOC)
	actionsOverlay             // the actions page (see ShowActions)
)

// showOverlay shows source, a page the browser generated, over the
// current document, as kind. The document's View is kept aside, where
// it was, until hideOverlay; one overlay replaces another.
func (a *App) showOverlay(kind overlayKind, source []byte) {
	docView := a.Panel.View()
	if a.overlaid != nil {
		docView = a.overlaid
	}
	if a.journal != nil && a.overlaid == nil {
		// The Panel shows the overlay, as it would without the journal.
		a.Panel.SetBounds(a.bounds)
		a.Panel.SetScale(a.deviceScale)
		a.Panel.SetZoom(a.zoom)
	}
	view := a.overlayView(source)
	a.overlaid, a.overlay = docView, kind
	a.arrival = arrival{}
	a.Panel.SetView(view)
	a.updateWindowTitle()
}

// overlayView returns a View of source, a page the browser generated.
func (a *App) overlayView(source []byte) *whynot.View {
	view := whynot.NewView(
		whynot.Parse(source),
		whynot.WithFaceSelector(a.faceSelector),
		whynot.WithStyleSheet(a.styleSheet),
	)
	a.place(view)
	return view
}

// hideOverlay shows the document an overlay was over again, exactly
// where it was left. A no-op without an overlay.
func (a *App) hideOverlay() {
	docView := a.overlaid
	if docView == nil {
		return
	}
	a.Panel.SetView(docView) // re-applies a.styleSheet, then relayouts
	a.overlaid, a.overlay = nil, noOverlay
	if a.journal != nil {
		a.prepare(docView) // a page of the stack again
	}
	a.updateWindowTitle()
}

// HideOverlay hides the table of contents or the actions page, whichever
// is showing.
func (a *App) HideOverlay() { a.hideOverlay() }

// overlaySuffix is what the window title gets while an overlay shows,
// after the document's own title.
func (a *App) overlaySuffix() string {
	switch a.overlay {
	case tocOverlay:
		return " - TOC"
	case actionsOverlay:
		return " - Actions"
	}
	return ""
}
