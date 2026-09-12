package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/ebitenrenderer"
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
	ebiten.SetWindowTitle("Why Not?")
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
		faceSelector: faceSelector,
		styleSheet:   styleSheet,
		renderer:     ebitenrenderer.New(),
		debugHit:     *debugHit,
	}
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

	faceSelector whynot.FaceSelector
	styleSheet   whynot.StyleSheet
	renderer     *ebitenrenderer.Renderer
	debugHit     bool

	hoverX, hoverY int

	// width, scale are Layout's most recent physical width and display
	// scale - follow and back both need them to lay out a View that
	// isn't the one ebiten just called Layout on.
	width int
	scale float64
}

func (g *game) Update() error {
	_, dy := ebiten.Wheel()
	g.current.view.Scroll(dy * ebiten.Monitor().DeviceScaleFactor() * 2)

	g.hoverX, g.hoverY = ebiten.CursorPosition()
	dest, hasLink := g.current.view.Hover(g.hoverX, g.hoverY)

	if hasLink && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		g.follow(dest)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		g.back()
	}

	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyL):
		g.setStyleSheet(whynot.NewLightStyleSheet())
	case inpututil.IsKeyJustPressed(ebiten.KeyD):
		g.setStyleSheet(whynot.NewDarkStyleSheet())
	}
	return nil
}

func (g *game) setStyleSheet(s whynot.StyleSheet) {
	g.styleSheet = s
	g.current.view.SetStyleSheet(s)
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
	target, err := url.Parse(dest)
	if err != nil {
		log.Printf("link destination %q: %v", dest, err)
		return
	}
	resolved := g.current.location.ResolveReference(target)

	if samePage(g.current.location, resolved) {
		if resolved.Fragment == "" {
			return
		}
		g.pushHistory()
		g.current.view.ScrollToAnchor(resolved.Fragment)
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
}

// pushHistory saves the current document and scroll position onto
// history, so back can return to it.
func (g *game) pushHistory() {
	g.history = append(g.history, historyEntry{
		document: g.current,
		scroll:   g.current.view.ScrollPosition(),
	})
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
// start of history. A cross-document entry's View may be stale if the
// window was resized or the theme toggled while it wasn't current, so
// its scroll position is restored first, then its StyleSheet and
// layout are refreshed - in that order, so each rebuild's
// ratio-preserving cursor logic works from the right position.
func (g *game) back() {
	if len(g.history) == 0 {
		return
	}
	entry := g.history[len(g.history)-1]
	g.history = g.history[:len(g.history)-1]
	entry.view.RestoreScrollPosition(entry.scroll)
	entry.view.SetStyleSheet(g.styleSheet)
	entry.view.Layout(g.width, g.scale)
	g.current = entry.document
}

func (g *game) Draw(screen *ebiten.Image) {
	canvas := g.renderer.NewCanvas(screen)

	// View.Draw fills the background itself, from the View's StyleSheet -
	// no separate clear step needed here.
	g.current.view.Draw(canvas, 0, 0)

	if g.debugHit {
		if hit, offset := g.current.view.HitTest(g.hoverX, g.hoverY); hit != nil {
			drawOutline(canvas, hit.Bounds().Add(offset), color.RGBA{255, 0, 0, 255})
		}
	}
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

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	s := ebiten.Monitor().DeviceScaleFactor()
	width := int(float64(outsideWidth) * s)
	height := int(float64(outsideHeight) * s)
	g.width, g.scale = width, s
	g.current.view.Layout(width, s)
	return width, height
}
