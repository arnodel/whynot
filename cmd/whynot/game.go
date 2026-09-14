package main

import (
	"image"
	"net/url"
	"time"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/ebitenrenderer"
)

// document pairs a View with the location it was loaded from, so a
// relative link found inside it can be resolved to an absolute one
// before being followed.
type document struct {
	location *url.URL
	view     *whynot.View
}

// historyEntry is a place back can return to: a document plus the
// scroll position it was at when navigated away from. For a
// cross-document jump, its view is that document's own, kept around
// entirely so this scroll position stays valid; for an in-page anchor
// jump, its view is the same *whynot.View that's still current, and
// restoring scroll is all back needs to do.
type historyEntry struct {
	document
	scroll whynot.ScrollPosition
}

// game adapts a whynot.View to ebiten's Game interface: it owns window/input
// plumbing only, all rendering behavior lives in the library.
type game struct {
	current document
	// history is the stack of places navigated away from, most recent
	// last. A cross-document jump's entry keeps that document's own
	// *whynot.View, so going back restores its exact scroll position
	// for free, with no bookkeeping beyond not discarding the View. An
	// in-page anchor jump's entry instead reuses the current View - only
	// its pre-jump ScrollPosition needs remembering, so following many
	// anchors in the same document doesn't allocate a View each time.
	history []historyEntry
	// future is the same idea, the other direction: places back has
	// left that forward can return to. Any new navigation (follow, via
	// pushHistory) clears it, same as a browser discarding forward
	// history once you branch off somewhere new.
	future []historyEntry

	faceSelector whynot.FaceSelector
	// toolbarFaceSelector is independent of faceSelector, deliberately:
	// View.Layout unconditionally resets faceSelector's DPI every
	// frame (see relayout) to deviceScale*zoom, so the toolbar's own
	// text needs its own selector pinned to deviceScale alone - sharing
	// one would either zoom the toolbar's text against its fixed-size
	// buttons, or worse, thrash both selectors' font caches every frame
	// as Layout and drawToolbar fought over one shared DPI.
	toolbarFaceSelector whynot.FaceSelector
	styleSheet          whynot.StyleSheet
	// darkTheme tracks which of styleSheet's two possible values is
	// current - NewDarkStyleSheet/NewLightStyleSheet both return the
	// same concrete type, so there's no way to recover this from
	// styleSheet itself; it drives which of darkModeIcon/lightModeIcon
	// the theme button shows (the current theme's icon - clicking
	// switches to the other one).
	darkTheme bool
	renderer  *ebitenrenderer.Renderer
	debugHit  bool

	hoverX, hoverY int
	// hoverDest is the link under the cursor, if any - what the address
	// bar shows instead of the current location while hovering.
	hoverDest string

	// outsideWidth, outsideHeight are the logical (device-independent)
	// window dimensions ebiten's own Layout callback last reported -
	// relayout recomputes deviceScale/width/height/scale from these
	// whenever either changes, whether that's ebiten reporting a resize
	// or setZoom itself.
	outsideWidth, outsideHeight int

	// zoom is a user-controlled multiplier on top of the display's own
	// scale (1.0 = 100%, see setZoom) - +/- keys adjust it.
	zoom float64
	// zoomIndicatorUntil is when the "N%" popup (drawZoomIndicator)
	// should stop showing - reset to now plus a fixed duration on every
	// setZoom call, the zero Time otherwise (always in the past, so it
	// never shows before the first zoom change).
	zoomIndicatorUntil time.Time

	// deviceScale is the display's own scale, with no zoom applied -
	// what the toolbar's own size/font stays pinned to, and what
	// width/height (below) are computed from, so the buffer Layout
	// hands back to ebiten always matches the real window 1:1
	// regardless of zoom (ebiten stretches that buffer to fit the
	// actual window otherwise, which would silently cancel zoom back
	// out - a real, previously-shipped bug this comment is here to
	// stop from coming back).
	deviceScale float64

	// width, height are relayout's most recent physical dimensions,
	// computed from deviceScale alone (not zoom - see deviceScale).
	// scale is deviceScale*zoom, passed to View.Layout as the document's
	// own font/DPI scale - decoupled from width, which View.Layout
	// takes as a separate argument, so zoom changes glyph size without
	// changing the wrap width or the returned buffer size. follow and
	// back need width/scale too, to lay out a View that isn't the one
	// relayout just ran on; height (minus toolbarHeight) is the page
	// size Space/Shift+Space scroll by.
	width, height int
	scale         float64

	// toolbarHeight and the button rectangles are recomputed by
	// layoutToolbar whenever Layout runs - the document itself is drawn
	// below toolbarHeight, so this is also the y-offset HitTest/Hover
	// need subtracted from the raw cursor position. zoomIn/zoomOut sit
	// on the toolbar's right edge, apart from back/forward/reload on
	// the left - they're not navigation.
	toolbarHeight                           int
	backButton, forwardButton, reloadButton image.Rectangle
	backState, forwardState, reloadState    buttonState
	zoomInButton, zoomOutButton             image.Rectangle
	zoomInState, zoomOutState               buttonState
	themeButton                             image.Rectangle
	themeState                              buttonState

	// start is when the game began, for whynot.RenderingContext.Time
	// (elapsed time since rendering started - what an animated GIF's
	// current frame is picked from). Only ever compared to itself via
	// elapsed(), never to a wall-clock timestamp.
	start time.Time
}

// elapsed is how long the game has been running - see start.
func (g *game) elapsed() time.Duration {
	return time.Since(g.start)
}

// buttonState is a toolbar button's per-frame input state, driving its
// drawn appearance - the same "check each frame, draw differently"
// pattern View.Hover already uses for a hovered link.
type buttonState struct {
	hover, pressed bool
}
