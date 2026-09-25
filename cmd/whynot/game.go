package main

import (
	"image"
	"time"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/browser"
	"github.com/arnodel/whynot/ebitenrenderer"
)

// game adapts a browser.App (navigation/history/theme/zoom) and an
// ebitenrenderer.Panel (the document itself) to ebiten's Game
// interface: it owns window/input plumbing and its own toolbar, all
// other behavior lives in the library or in browser.
type game struct {
	app   *browser.App
	panel *ebitenrenderer.Panel

	// toolbarFaceSelector is independent of app's own document
	// faceSelector, deliberately: View.Layout unconditionally resets
	// the document faceSelector's DPI every frame (see relayout) to
	// deviceScale*zoom, so the toolbar's own text needs its own
	// selector pinned to deviceScale alone - sharing one would either
	// zoom the toolbar's text against its fixed-size buttons, or
	// worse, thrash both selectors' font caches every frame as Layout
	// and drawToolbar fought over one shared DPI.
	toolbarFaceSelector whynot.FaceSelector
	renderer            *ebitenrenderer.Renderer
	debugHit            bool

	// debugStats shows FPS/TPS and per-frame timing (see
	// drawDebugStats) - set from -debug-stats at startup, togglable at
	// runtime with F (see Update). updateDuration/drawDuration are the
	// most recent Update/Draw call's wall-clock time, timed
	// unconditionally (cheap - two time.Now() calls) so toggling
	// debugStats on doesn't need to wait a frame for the first reading.
	debugStats                   bool
	updateDuration, drawDuration time.Duration

	hoverX, hoverY int

	// outsideWidth, outsideHeight are the logical (device-independent)
	// window dimensions ebiten's own Layout callback last reported -
	// relayout recomputes app's own width/height/scale from these
	// whenever either changes, whether that's ebiten reporting a resize
	// or setZoom itself.
	outsideWidth, outsideHeight int

	// deviceScale is the display's own scale, with no zoom applied -
	// what the toolbar's own size/font stays pinned to (see
	// toolbarFaceSelector).
	deviceScale float64

	// width is relayout's most recent physical width, computed from
	// deviceScale alone (not zoom).
	width int

	// toolbarHeight and the button rectangles are recomputed by
	// layoutToolbar whenever Layout runs - the document itself is drawn
	// below toolbarHeight (relayout passes it to app.Relayout, which
	// sets panel's own Bounds to start there, so panel's own input
	// handling and debug-hit's HitTest both offset by it via
	// panel.Bounds() rather than this field directly). zoomIn/zoomOut
	// sit on the toolbar's right edge, apart from back/forward/reload
	// on the left - they're not navigation.
	toolbarHeight                           int
	backButton, forwardButton, reloadButton image.Rectangle
	backState, forwardState, reloadState    buttonState
	zoomInButton, zoomOutButton             image.Rectangle
	zoomInState, zoomOutState               buttonState
	themeButton                             image.Rectangle
	themeState                              buttonState
}

// buttonState is a toolbar button's per-frame input state, driving its
// drawn appearance - the same "check each frame, draw differently"
// pattern View.Hover already uses for a hovered link.
type buttonState struct {
	hover, pressed bool
}
