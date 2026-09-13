package main

import (
	"bytes"
	"embed"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"math"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"golang.org/x/image/font"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/ebitenrenderer"
)

//go:embed arrow_back.png arrow_forward.png refresh.png
var iconFS embed.FS

// icon loads a PNG embedded via iconFS into an *ebiten.Image - decoded
// once at startup (see backIcon/forwardIcon/reloadIcon below), not per
// frame.
func icon(name string) *ebiten.Image {
	data, err := iconFS.ReadFile(name)
	if err != nil {
		panic(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	return ebiten.NewImageFromImage(img)
}

// backIcon/forwardIcon/reloadIcon are the toolbar buttons' icons -
// light, mostly-white silhouettes on a transparent background, so
// drawIcon can tint them to match a button's current state the same
// way drawButton already tints its border/fill.
var (
	backIcon    = icon("arrow_back.png")
	forwardIcon = icon("arrow_forward.png")
	reloadIcon  = icon("refresh.png")
)

func main() {
	light := flag.Bool("light", false, "use whynot's light theme instead of the default dark one")
	debugHit := flag.Bool("debug-hit", false, "outline the box under the mouse, via View.HitTest")
	flag.Parse()
	f := "test.md"
	if flag.NArg() != 0 {
		f = flag.Arg(0)
	}

	location, err := absFileURL(f)
	if err != nil {
		panic(err)
	}
	source, err := loadDocument(location)
	if err != nil {
		panic(err)
	}

	ebiten.SetWindowSize(1024, 768)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	styleSheet := whynot.StyleSheet(whynot.NewDarkStyleSheet())
	if *light {
		styleSheet = whynot.NewLightStyleSheet()
	}

	scale := ebiten.Monitor().DeviceScaleFactor()
	faceSelector := whynot.NewGoFontFaceSelector(72 * scale)
	game := &game{
		current: document{
			location: location,
			view:     whynot.NewView(source, faceSelector, whynot.WithStyleSheet(styleSheet)),
		},
		faceSelector:        faceSelector,
		toolbarFaceSelector: whynot.NewGoFontFaceSelector(72 * scale),
		styleSheet:          styleSheet,
		renderer:            ebitenrenderer.New(),
		debugHit:            *debugHit,
		zoom:                1,
	}
	game.updateWindowTitle()
	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

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

// absFileURL turns a command-line path into a file: URL with an
// absolute path, so it can be used as the base for resolving a
// relative link the same way an http(s) URL would be.
func absFileURL(path string) (*url.URL, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return &url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}, nil
}

// loadDocument fetches the bytes at location - a local read for a
// file: URL, an HTTP GET for http(s). Any other scheme (e.g. a
// mailto: autolink) is rejected rather than misread as a file path -
// and so is an http(s) response whose Content-Type is clearly not
// Markdown/plain text (e.g. a real webpage, not a .md file): whynot has
// no way to tell HTML apart from Markdown itself, so without this
// check it would just get fed straight into the Markdown parser.
func loadDocument(location *url.URL) ([]byte, error) {
	switch location.Scheme {
	case "http", "https":
		client := http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(location.String())
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: %s", location, resp.Status)
		}
		// A missing or unparseable Content-Type is let through - a
		// heuristic, not a guarantee, since some servers omit or
		// misreport it for a perfectly good Markdown file.
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			if mediaType, _, err := mime.ParseMediaType(ct); err == nil &&
				mediaType != "text/plain" && mediaType != "text/markdown" {
				return nil, fmt.Errorf("%s: not Markdown (Content-Type: %s)", location, mediaType)
			}
		}
		return io.ReadAll(resp.Body)
	case "file", "":
		return os.ReadFile(location.Path)
	default:
		return nil, fmt.Errorf("unsupported link scheme %q", location.Scheme)
	}
}

// readClipboard returns the clipboard's current text content, via each
// OS's own command-line paste utility rather than a third-party
// clipboard package - ebiten has no clipboard API of its own, and this
// is a small enough need (cmd/whynot's own convenience, not the
// library's) that it isn't worth a module dependency for.
func readClipboard() (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbpaste")
	case "windows":
		cmd = exec.Command("powershell", "-NoProfile", "-Command", "Get-Clipboard")
	default:
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--output")
		} else {
			return "", fmt.Errorf("no clipboard utility found (tried xclip, xsel)")
		}
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
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
	renderer            *ebitenrenderer.Renderer
	debugHit            bool

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
	// need subtracted from the raw cursor position.
	toolbarHeight                           int
	backButton, forwardButton, reloadButton image.Rectangle
	backState, forwardState, reloadState    buttonState
}

// buttonState is a toolbar button's per-frame input state, driving its
// drawn appearance - the same "check each frame, draw differently"
// pattern View.Hover already uses for a hovered link.
type buttonState struct {
	hover, pressed bool
}

func (g *game) Update() error {
	_, dy := ebiten.Wheel()
	g.current.view.Scroll(dy * ebiten.Monitor().DeviceScaleFactor() * 2)

	g.hoverX, g.hoverY = ebiten.CursorPosition()
	docY := g.hoverY - g.toolbarHeight

	var dest string
	var hasLink bool
	if docY >= 0 {
		dest, hasLink = g.current.view.Hover(g.hoverX, docY)
	} else {
		// Over the toolbar, not the document - (-1, -1) can't land on
		// anything, so this only ever clears a highlight left over from
		// just having moved off a link.
		g.current.view.Hover(-1, -1)
	}
	g.hoverDest = ""
	if hasLink {
		if resolved, err := g.resolveLink(dest); err == nil {
			g.hoverDest = resolved.String()
		}
	}

	cursor := image.Pt(g.hoverX, g.hoverY)
	mouseDown := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	g.backState = buttonState{hover: cursor.In(g.backButton), pressed: mouseDown && cursor.In(g.backButton)}
	g.forwardState = buttonState{hover: cursor.In(g.forwardButton), pressed: mouseDown && cursor.In(g.forwardButton)}
	g.reloadState = buttonState{hover: cursor.In(g.reloadButton), pressed: mouseDown && cursor.In(g.reloadButton)}

	clicked := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	switch {
	case hasLink && clicked:
		g.follow(dest)
	case clicked && g.backState.hover:
		g.back()
	case clicked && g.forwardState.hover:
		g.forward()
	case clicked && g.reloadState.hover:
		g.reload()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			g.forward()
		} else {
			g.back()
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		// A little overlap with the previous page - not the full
		// viewport height - so the last line you were reading is still
		// there as a landmark, same idea as a pager's own page-down.
		const pageOverlapFrac = 0.1
		page := float64(g.height-g.toolbarHeight) * (1 - pageOverlapFrac)
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			g.current.view.Scroll(page)
		} else {
			g.current.view.Scroll(-page)
		}
	}

	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyL):
		g.setStyleSheet(whynot.NewLightStyleSheet())
	case inpututil.IsKeyJustPressed(ebiten.KeyD):
		g.setStyleSheet(whynot.NewDarkStyleSheet())
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyV) && (ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)) {
		g.paste()
	}
	// A fixed step of the original (100%) size, not of the current
	// zoom - so it's 100%, 110%, 120%, ... rather than steps shrinking
	// as you zoom out or growing as you zoom in.
	const zoomStep = 0.1
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyEqual):
		g.setZoom(g.zoom + zoomStep)
	case inpututil.IsKeyJustPressed(ebiten.KeyMinus):
		g.setZoom(g.zoom - zoomStep)
	}
	return nil
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
	target, err := url.Parse(dest)
	if err != nil {
		return nil, err
	}
	return g.current.location.ResolveReference(target), nil
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
		log.Printf("loading %s: %v", resolved, err)
		return
	}
	view := whynot.NewView(source, g.faceSelector, whynot.WithStyleSheet(g.styleSheet))
	view.Layout(g.width, g.scale)
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
	entry.view.Layout(g.width, g.scale)
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
		log.Printf("reloading %s: %v", g.current.location, err)
		return
	}
	scroll := g.current.view.ScrollPosition()
	view := whynot.NewView(source, g.faceSelector, whynot.WithStyleSheet(g.styleSheet))
	view.Layout(g.width, g.scale)
	view.RestoreScrollPosition(scroll)
	g.current.view = view
	g.updateWindowTitle()
}

// paste navigates to the clipboard's current text content, if it's an
// http(s) URL or an existing local file path - unlike follow, this is
// always treated as an absolute destination, never resolved relative
// to the current document, since pasting is a user-initiated "go
// here," not a link inside whatever's currently on screen.
func (g *game) paste() {
	text, err := readClipboard()
	if err != nil {
		log.Printf("reading clipboard: %v", err)
		return
	}
	text = strings.TrimSpace(text)

	var resolved *url.URL
	if u, err := url.Parse(text); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		resolved = u
	} else if _, err := os.Stat(text); err == nil {
		if abs, err := absFileURL(text); err == nil {
			resolved = abs
		}
	}
	if resolved == nil {
		log.Printf("clipboard content %q isn't a URL or an existing file path", text)
		return
	}

	source, err := loadDocument(resolved)
	if err != nil {
		log.Printf("loading %s: %v", resolved, err)
		return
	}
	view := whynot.NewView(source, g.faceSelector, whynot.WithStyleSheet(g.styleSheet))
	view.Layout(g.width, g.scale)
	g.pushHistory()
	g.current = document{location: resolved, view: view}
	g.updateWindowTitle()
}

func (g *game) Draw(screen *ebiten.Image) {
	canvas := g.renderer.NewCanvas(screen)

	// View.Draw fills the background itself, from the View's StyleSheet -
	// no separate clear step needed here. It's drawn below the toolbar,
	// which is painted over it afterward.
	g.current.view.Draw(canvas, 0, g.toolbarHeight)
	g.drawToolbar(screen, canvas)
	g.drawZoomIndicator(canvas)

	if g.debugHit {
		docY := g.hoverY - g.toolbarHeight
		if hit, offset := g.current.view.HitTest(g.hoverX, docY); hit != nil {
			drawOutline(canvas, hit.Bounds().Add(offset).Add(image.Pt(0, g.toolbarHeight)), color.RGBA{255, 0, 0, 255})
		}
	}
}

// drawToolbar paints the address bar (the current document's location,
// or - while hovering a link - that link's destination instead, in
// StyleSheet.HighlightColor to match the hovered link's own color in
// the document) and the back/forward/reload buttons. Icon drawing
// needs dst directly - whynot.Canvas has no primitive for a scaled,
// tinted image - so this is the one part of cmd/whynot's own UI that
// goes around the library's rendering abstraction rather than through
// it.
func (g *game) drawToolbar(dst *ebiten.Image, canvas whynot.Canvas) {
	canvas.DrawRect(0, 0, g.width, g.toolbarHeight, color.RGBA{0x20, 0x20, 0x20, 0xFF})

	drawButton(dst, canvas, backIcon, g.backButton, len(g.history) > 0, g.backState)
	drawButton(dst, canvas, forwardIcon, g.forwardButton, len(g.future) > 0, g.forwardState)
	drawButton(dst, canvas, reloadIcon, g.reloadButton, true, g.reloadState)

	face, err := g.toolbarFaceSelector.SelectFace(whynot.TextStyle{Size: 14})
	if err != nil {
		return
	}
	text, textColor := g.current.location.String(), color.Color(color.RGBA{0xCC, 0xCC, 0xCC, 0xFF})
	if g.hoverDest != "" {
		text, textColor = g.hoverDest, g.styleSheet.HighlightColor()
	}
	x := g.reloadButton.Max.X + int(16*g.deviceScale)
	canvas.DrawText(text, face, x, baselineIn(face, image.Rect(x, 0, g.width, g.toolbarHeight)), textColor)
}

// zoomIndicatorDuration is how long the "N%" popup stays up after a
// zoom change - long enough to read, short enough to get out of the
// way on its own.
const zoomIndicatorDuration = 1500 * time.Millisecond

// drawZoomIndicator shows the current zoom level for a little while
// after it changes (see setZoom), in the content area's top-right
// corner - not the toolbar's, which will get its own zoom control
// later and is a fixed size regardless of zoom (see deviceScale);
// this is explicitly "on top of the document" instead, since zoom is a
// document-only setting. Uses toolbarFaceSelector, for the same reason
// drawToolbar does - a size fixed at the display's own scale, not
// zoomed.
func (g *game) drawZoomIndicator(canvas whynot.Canvas) {
	if !time.Now().Before(g.zoomIndicatorUntil) {
		return
	}
	face, err := g.toolbarFaceSelector.SelectFace(whynot.TextStyle{Size: 14})
	if err != nil {
		return
	}
	label := fmt.Sprintf("%.0f%%", g.zoom*100)
	padX, padY := int(10*g.deviceScale), int(6*g.deviceScale)
	margin := int(12 * g.deviceScale)
	textW := font.MeasureString(face, label).Ceil()
	textH := (face.Metrics().Ascent + face.Metrics().Descent).Ceil()
	w, h := textW+2*padX, textH+2*padY

	x := g.width - margin - w
	y := g.toolbarHeight + margin
	r := image.Rect(x, y, x+w, y+h)
	canvas.DrawRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), color.RGBA{0x20, 0x20, 0x20, 0xFF})
	canvas.DrawText(label, face, r.Min.X+padX, baselineIn(face, r), color.RGBA{0xE0, 0xE0, 0xE0, 0xFF})
}

// drawButton draws an icon button, filling the whole of r - no border,
// no margin around the icon, just a background fill (for hover/pressed
// feedback) showing through wherever the icon's own transparency lets
// it. enabled only affects appearance; back is still harmless to click
// with no history, so nothing needs disabling functionally (see
// buttonColors).
func drawButton(dst *ebiten.Image, canvas whynot.Canvas, iconImg *ebiten.Image, r image.Rectangle, enabled bool, st buttonState) {
	fillColor, tint := buttonColors(enabled, st)
	if fillColor != nil {
		canvas.DrawRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), fillColor)
	}
	drawIcon(dst, iconImg, r, tint)
}

// drawIcon draws icon scaled to fill r exactly (preserving aspect
// ratio, icons are square anyway), tinted to clr - icon is expected to
// be a light, mostly-white silhouette on transparency, the same
// assumption ebiten's multiplicative ColorScale tinting relies on
// elsewhere (e.g compositing a hovered link's glyphs).
func drawIcon(dst, iconImg *ebiten.Image, r image.Rectangle, clr color.Color) {
	b := iconImg.Bounds()
	scale := float64(r.Dy()) / float64(b.Dy())
	if s := float64(r.Dx()) / float64(b.Dx()); s < scale {
		scale = s
	}
	w, h := float64(b.Dx())*scale, float64(b.Dy())*scale

	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Scale(scale, scale)
	opts.GeoM.Translate(float64(r.Min.X)+(float64(r.Dx())-w)/2, float64(r.Min.Y)+(float64(r.Dy())-h)/2)
	opts.ColorScale.ScaleWithColor(clr)
	dst.DrawImage(iconImg, opts)
}

// buttonColors picks a button's fill/icon-tint colors for its current
// state - a disabled button ignores hover/pressed entirely, reading as
// inert regardless of where the cursor is. fillColor is nil for "no
// fill", i.e. the toolbar's own background shows through.
func buttonColors(enabled bool, st buttonState) (fillColor, iconTint color.Color) {
	switch {
	case !enabled:
		return nil, color.RGBA{0x60, 0x60, 0x60, 0xFF}
	case st.pressed:
		return color.RGBA{0x50, 0x50, 0x50, 0xFF}, color.White
	case st.hover:
		return color.RGBA{0x30, 0x30, 0x30, 0xFF}, color.RGBA{0xF0, 0xF0, 0xF0, 0xFF}
	default:
		return nil, color.RGBA{0xE0, 0xE0, 0xE0, 0xFF}
	}
}

// baselineIn returns the y coordinate DrawText needs to vertically
// center one line of face-set text within r.
func baselineIn(face font.Face, r image.Rectangle) int {
	m := face.Metrics()
	return r.Min.Y + (r.Dy()+m.Ascent.Ceil()-m.Descent.Ceil())/2
}

// drawOutline draws a thin border around r - Canvas has no dedicated
// outline primitive, so this is 4 filled rects along the edges.
func drawOutline(dst whynot.Canvas, r image.Rectangle, clr color.Color) {
	const thickness = 2
	dst.DrawRect(r.Min.X, r.Min.Y, r.Dx(), thickness, clr)
	dst.DrawRect(r.Min.X, r.Max.Y-thickness, r.Dx(), thickness, clr)
	dst.DrawRect(r.Min.X, r.Min.Y, thickness, r.Dy(), clr)
	dst.DrawRect(r.Max.X-thickness, r.Min.Y, thickness, r.Dy(), clr)
}

// toolbarLogicalHeight is the address bar / button row's height, in the
// same DPI-relative logical units as StyleSheet text sizes (i.e. not
// pre-multiplied by scale here - layoutToolbar does that).
const toolbarLogicalHeight = 36

// layoutToolbar recomputes the toolbar's height and button positions
// for the current device scale - deliberately deviceScale, not scale,
// so the toolbar's own size stays fixed regardless of zoom (see
// deviceScale's own doc comment on the game struct).
func (g *game) layoutToolbar() {
	s := g.deviceScale
	g.toolbarHeight = int(toolbarLogicalHeight * s)
	pad := int(8 * s)
	btn := g.toolbarHeight - 2*pad // square icon buttons
	next := pad
	nextButton := func() image.Rectangle {
		r := image.Rect(next, pad, next+btn, pad+btn)
		next = r.Max.X + pad
		return r
	}
	g.backButton = nextButton()
	g.forwardButton = nextButton()
	g.reloadButton = nextButton()
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	g.outsideWidth, g.outsideHeight = outsideWidth, outsideHeight
	g.relayout()
	return g.width, g.height
}

// relayout recomputes physical width/height (from deviceScale alone)
// and scale (deviceScale*zoom) from the last known logical window size
// (outsideWidth/outsideHeight), and applies them to the toolbar and
// the current View - shared by the ebiten-driven Layout callback and
// setZoom, which needs the same recomputation to happen immediately
// rather than waiting for ebiten's next own Layout call (same reason
// follow/back/reload/paste each lay out their View immediately instead
// of leaving it for next frame).
func (g *game) relayout() {
	g.deviceScale = ebiten.Monitor().DeviceScaleFactor()
	g.scale = g.deviceScale * g.zoom
	g.width = int(float64(g.outsideWidth) * g.deviceScale)
	g.height = int(float64(g.outsideHeight) * g.deviceScale)
	g.toolbarFaceSelector.SetDPI(g.deviceScale * 72)
	g.layoutToolbar()
	g.current.view.Layout(g.width, g.scale)
}

// setZoom changes the zoom level (1.0 = 100%), clamped to a sane
// range, and re-lays-out immediately at the new scale - the same idea
// as a window resize, just user-triggered instead.
func (g *game) setZoom(zoom float64) {
	const minZoom, maxZoom = 0.5, 3.0
	g.zoom = math.Max(minZoom, math.Min(maxZoom, zoom))
	g.relayout()
	g.zoomIndicatorUntil = time.Now().Add(zoomIndicatorDuration)
}
