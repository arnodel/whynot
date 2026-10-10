package whynot

import (
	"image"
	"image/color"
	"math"
	"time"

	"github.com/arnodel/whynot/canvas"
	"github.com/arnodel/whynot/fetch"
	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/engine"
	"github.com/arnodel/whynot/internal/imagecache"
	"github.com/arnodel/whynot/internal/markdown"
	"github.com/arnodel/whynot/styles/simpletheme"
)

// View renders a Document into a rectangle of a Canvas, its Bounds,
// scrolled to a position. It lays out only what's needed to draw, lays
// out again only when the width, scale or StyleSheet change, and keeps
// the scroll position across those changes.
//
// A View reads no input itself: a Controller turns input into hovering,
// clicking and scrolling. Its own methods are for programmatic use:
// SetBounds, SetScale and SetZoom when the window, screen or zoom change, and positioning
// with ScrollBy, ScrollToRatio, ScrollToAnchor and RestoreScrollPosition.
//
// All its positions and distances are in canvas pixels (see Coordinates
// in the package documentation).
type View struct {
	doc *Document
	ctx engine.Context

	// reader is where the View is in doc, which may grow (see follow).
	// blocks are the top-level blocks it has taken from it, the last
	// tailBlocks of which are unfinished, and soleImages their sole images
	// (see prefetchImageSources).
	reader     reader
	blocks     []engine.Block
	tailBlocks int
	soleImages map[engine.Block]fetch.Source

	// hscroll is the sideways scrolling state of the View's code blocks
	// and tables, and draws their scrollbars: ctx's ScrollOffset and
	// Scrollbar hooks call it.
	hscroll *hscrollState

	// stack is the laid-out document and the scroll position within it.
	stack documentStack

	// bounds is where the View is drawn on the canvas. width and scale are
	// what stack was last laid out at.
	bounds image.Rectangle
	width  int
	scale  float64

	// displayScale and zoom make up the scale the View lays out at,
	// ctx.Scale: see SetScale and SetZoom.
	displayScale float64
	zoom         float64

	// vbar is the View's own vertical scrollbar, if enabled.
	vbar vscrollbar

	// imageCacheMark is the imagecache.Cache.ChangedSince mark from the last
	// time Draw checked for image state changes.
	imageCacheMark uint64

	// moves counts calls that set the scroll position (ScrollBy,
	// ScrollToRatio and so on), so a Controller can tell when something
	// else moved the View. Re-anchoring after a relayout doesn't count.
	moves uint64

	// highlightSlot is the top-level slot containing ctx.HighlightNode
	// (meaningless when HighlightNode is nil). Hover refreshes it on every
	// call, so it self-heals after an intervening rebuild.
	highlightSlot int
}

// ViewOption customizes a View at construction, via NewView's opts
// parameter.
type ViewOption func(*View)

// WithFaceSelector sets where the View's fonts come from, instead of the
// Go fonts (see [fonts.GoSelector]).
//
// Backends cache what they make from each font face, so give Views drawn
// by the same Renderer the same selector, rather than a new one each: then
// they share its faces, and the backend's caches. Views on different
// goroutines need different selectors, because font faces aren't safe
// for concurrent use.
func WithFaceSelector(s fonts.FaceSelector) ViewOption {
	return func(v *View) {
		v.ctx.FaceSelector = s
	}
}

// WithStyleSheet sets how the View looks, instead of the default,
// [simpletheme.DarkStyleSheet]. [View.SetStyleSheet] changes it later.
func WithStyleSheet(s StyleSheet) ViewOption {
	return func(v *View) {
		v.ctx.Styles = s.Styles()
	}
}

// defaultFaceSelector serves every View not given a selector of its own,
// so that they all share the same faces.
var defaultFaceSelector = fonts.NewGoSelector()

// NewView returns a View of doc (see [Parse]), at scale 1 and zoom 1. By
// default it uses the Go fonts and the dark theme of package simpletheme:
// opts can change that. It has empty bounds, so it shows nothing until
// [View.SetBounds] is called.
//
// Views with the default fonts share one selector, so use them all from
// one goroutine, as a user interface does, or give the others a selector
// of their own with [WithFaceSelector] (see Concurrency in the package
// documentation).
func NewView(doc *Document, opts ...ViewOption) *View {
	v := &View{
		doc: doc,
		ctx: engine.Context{
			Scale:        1,
			FaceSelector: defaultFaceSelector,
			Styles:       simpletheme.DarkStyleSheet.Styles(),
			ImageCache:   imagecache.NewCache(),
		},
		hscroll:      newHScrollState(),
		displayScale: 1,
		zoom:         1,
	}
	v.ctx.ScrollOffset = v.hscroll.scrollOffset
	v.ctx.Scrollbar = func(dst canvas.Canvas, r engine.ScrollRegion, now time.Duration) {
		v.hscroll.drawScrollbar(dst, r, now, v.ctx.Scale, v.ctx.Styles)
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// Document returns the Document this View renders.
func (v *View) Document() *Document {
	return v.doc
}

// CurrentHeadingID returns the id of the top-level heading at or just
// above the top of the viewport - the section currently on screen - or
// ok=false if there's none before the scroll position, or nothing has
// been laid out yet.
func (v *View) CurrentHeadingID() (id string, ok bool) {
	if !v.stack.laidOut() {
		return "", false
	}
	for i := min(v.stack.cursor.Index, v.stack.len()-1); i >= 0; i-- {
		if n := nodeOf(v.stack.blockAt(i)); n != nil && n.Tag >= ast.TagHeading1 && n.Tag <= ast.TagHeading6 {
			return n.ID, true
		}
	}
	return "", false
}

// nodeOf returns block's ast.Node, or nil for a nil block (a spacer slot).
func nodeOf(block engine.Block) *ast.Node {
	if block == nil {
		return nil
	}
	return block.Node()
}

// ScrollBy moves the scroll position dy pixels towards the end of the
// document (towards the start if dy is negative).
func (v *View) ScrollBy(dy float64) {
	if v.stack.laidOut() {
		v.stack.scroll(dy)
		v.moves++
		v.revealScrollbar()
	}
}

// ScrollPosition is an opaque snapshot of a View's scroll position,
// captured by ScrollPosition and restored by RestoreScrollPosition.
type ScrollPosition struct {
	cursor engine.StackCursor
}

// ScrollPosition captures the View's current scroll position.
func (v *View) ScrollPosition() ScrollPosition {
	return ScrollPosition{cursor: v.stack.cursor}
}

// RestoreScrollPosition restores a position captured earlier by
// ScrollPosition - only meaningful on the same View it was taken from.
func (v *View) RestoreScrollPosition(p ScrollPosition) {
	v.stack.cursor = p.cursor
	v.moves++
}

// ScrollToAnchor scrolls to put the heading with the given anchor id at
// the top of the viewport, e.g. after following a link with a URL
// fragment. ok is false, and the scroll position unchanged, if no heading
// has that id or nothing has been laid out yet. For a document still
// arriving, it looks in all that has arrived so far, including what hasn't
// been drawn yet.
//
// Only top-level headings are found: not one nested in a blockquote or
// list.
func (v *View) ScrollToAnchor(id string) bool {
	if !v.stack.laidOut() || id == "" {
		return false
	}
	v.follow()
	for i := range v.stack.len() {
		if n := nodeOf(v.stack.blockAt(i)); n != nil && n.ID == id {
			v.stack.scrollToSlot(i)
			v.moves++
			return true
		}
	}
	return false
}

// ScrollToRatio puts the top of the View at ratio (clamped to [0, 1]) of
// the document's estimated height, e.g. while dragging a scrollbar thumb
// (VisibleRange gives the ratio back).
func (v *View) ScrollToRatio(ratio float64) {
	if v.stack.laidOut() {
		v.stack.scrollToRatio(ratio)
		v.moves++
		v.revealScrollbar()
	}
}

// ScrollToEnd scrolls to show the end of the document: its bottom at the
// bottom of the View, or, for a document shorter than the View, its top at
// the top. Unlike ScrollToRatio(1), which puts the top of the View at the
// end, it's exact: it lays out the last block, and the blocks above it
// that fit in the View, but nothing else.
//
// For a document still arriving, it's the end of what has arrived so
// far, including what hasn't been drawn yet. To follow the end as more
// arrives, call it each frame until the reader scrolls (see [Scroll]).
func (v *View) ScrollToEnd() {
	if v.stack.laidOut() {
		v.follow()
		v.stack.scrollToEnd(v.bounds.Dy())
		v.moves++
		v.revealScrollbar()
	}
}

// HighlightColor is the color the View's StyleSheet shows a hovered link
// in - e.g. for an app echoing the link's destination elsewhere, in the
// same color.
func (v *View) HighlightColor() color.Color {
	return v.ctx.Styles.HighlightColor()
}

// ScrollbarColor is the color the View's StyleSheet gives a scrollbar
// thumb: hover whenever it's highlighted (including while dragged),
// pressed only while dragged - for a backend drawing the View's scrollbar.
func (v *View) ScrollbarColor(hover, pressed bool) color.Color {
	return v.ctx.Styles.ScrollbarColor(hover, pressed)
}

// VisibleRange returns the part of the document in view, as fractions of
// its height: from start to end, with 0 <= start <= end <= 1 - e.g. for
// drawing a scrollbar thumb. ScrollToRatio(start) is the way back. The
// height is an estimate until the whole document has been laid out, so
// the fractions can shift a little as more of it is seen. Before
// anything is laid out, it's (0, 1).
func (v *View) VisibleRange() (start, end float64) {
	if !v.stack.laidOut() || v.stack.cursor.Index >= v.stack.len() {
		return 0, 1
	}
	top, bottom, total := v.stack.visibleRange(v.bounds.Dy())
	if total <= 0 {
		return 0, 1
	}
	return top / total, bottom / total
}

// DocumentBounds returns where the View's document is on the canvas, as if
// all of it were drawn: starting above the View's top by what's scrolled
// past, and ending below its bottom by what's left, or above it, for a
// document that ends there. It's exact, to the pixel, up to limit beyond
// the View's top and bottom; beyond that, it only reaches past the limit.
// It lays out only what it measures, so a small limit keeps it cheap
// however long the document is. VisibleRange gives the same picture,
// estimated, as fractions of the whole.
func (v *View) DocumentBounds(limit int) image.Rectangle {
	if !v.stack.laidOut() {
		return v.bounds
	}
	height := v.bounds.Dy()
	above := v.stack.above(float64(limit))
	below := v.stack.below(float64(height)+float64(limit)) - float64(height)
	return image.Rect(v.bounds.Min.X, v.bounds.Min.Y-int(math.Round(above)),
		v.bounds.Max.X, v.bounds.Max.Y+int(math.Round(below)))
}

// ScrollWithin moves the scroll position dy pixels towards the end of the
// document (towards its start if dy is negative), as ScrollBy does, but
// no further than the document goes: up to its start, or down to its very
// end, at the top of the View. It returns rest, what was left of dy when
// it stopped: 0 if it moved all of it. A program scrolling several Views
// as one carries rest on to the next. It lays out only what it scrolls
// over.
func (v *View) ScrollWithin(dy float64) (rest float64) {
	if !v.stack.laidOut() || dy == 0 {
		return dy
	}
	if dy > 0 {
		moved := min(dy, v.stack.below(dy))
		if moved != 0 {
			v.ScrollBy(moved)
		}
		return dy - moved
	}
	moved := max(dy, -v.stack.above(-dy))
	if moved != 0 {
		v.ScrollBy(moved)
	}
	return dy - moved
}

// Bounds returns where the View is drawn on the canvas.
func (v *View) Bounds() image.Rectangle {
	return v.bounds
}

// SetBounds sets where the View is drawn on the canvas, which is also
// where its HitTest and LinkAt apply. A change of width lays the document
// out again, keeping the scroll position. A change of height alone is
// cheap, since nothing is laid out again: a program can give a View a
// different height each frame, to show part of it, say.
func (v *View) SetBounds(r image.Rectangle) {
	v.bounds = r
	v.relayout()
}

// Scale returns the display's scale (see SetScale).
func (v *View) Scale() float64 {
	return v.displayScale
}

// SetScale sets the display's scale: canvas pixels per logical pixel, so
// that text and margins are sized for the screen's density. The document
// is laid out at the scale times the zoom (see SetZoom): fonts at that
// times 72 DPI, and the StyleSheet's dimensions multiplied by it. A
// change lays the document out again, keeping the scroll position.
func (v *View) SetScale(scale float64) {
	v.displayScale = scale
	v.ctx.Scale = v.displayScale * v.zoom
	v.relayout()
}

// Zoom returns the document's zoom (see SetZoom).
func (v *View) Zoom() float64 {
	return v.zoom
}

// SetZoom magnifies the document by zoom (1 is 100%), on top of the
// display's scale (see SetScale). A change lays the document out again,
// keeping the scroll position.
func (v *View) SetZoom(zoom float64) {
	v.zoom = zoom
	v.ctx.Scale = v.displayScale * v.zoom
	v.relayout()
}

// relayout lays the document out again if its width or scale changed.
func (v *View) relayout() {
	if v.bounds.Dx() <= 0 || v.stack.laidOut() && v.bounds.Dx() == v.width && v.ctx.Scale == v.scale {
		return
	}
	v.width, v.scale = v.bounds.Dx(), v.ctx.Scale
	v.rebuild()
}

// Draw draws the View onto dst, within its Bounds, as it is at now:
// elapsed time on a clock of the caller's choosing (any origin, as long
// as it's the same on every call), which animated images and fading
// scrollbars follow. Call it every frame: it also picks up images that
// have finished loading.
//
// Draw fills its Bounds with the StyleSheet's background color first, so
// a caller doesn't need its own clear step. Its cost follows what's
// visible, not the document's size or how far into it the scroll
// position is.
func (v *View) Draw(dst canvas.Canvas, now time.Duration) {
	v.ctx.Time = now
	dst = dst.Clip(v.bounds)
	area := dst.Bounds()
	dst.DrawRect(area.Min.X, area.Min.Y, area.Dx(), area.Dy(), v.ctx.Styles.BackgroundColor())
	if !v.stack.laidOut() {
		return
	}
	v.update(now)
	if v.hscroll != nil {
		v.hscroll.beginFrame()
	}
	origin := v.contentOrigin()
	v.stack.box.DrawFrom(dst, v.stack.cursor, origin.X, origin.Y, now)
	v.drawScrollbar(dst)
}

// update does a frame's work before drawing at now: picking up what the
// Document gained and images that have loaded, and laying out and loading
// ahead of the viewport.
func (v *View) update(now time.Duration) {
	v.ctx.Time = now
	if !v.stack.laidOut() {
		return
	}
	v.follow()
	v.invalidateChangedImages()
	v.stack.preLayout(v.bounds.Dy())
	v.prefetchImageSources(v.bounds.Dy())
}

// contentOrigin is where the top-left corner of the content at the
// scroll position is drawn: the top-left corner of the bounds, shifted
// by the left ViewMargin. (The top and bottom ViewMargins are spacer
// slots in the stack: see rebuild.)
func (v *View) contentOrigin() image.Point {
	return v.bounds.Min.Add(image.Pt(int(v.ctx.ScaledViewMargins().Left), 0))
}

// HitTest returns the rectangle of the content at (x, y), e.g. for
// drawing an outline around it. ok is false if (x, y) is outside the
// Bounds, or doesn't land on any content: past the end of the document,
// or in a margin or gap.
func (v *View) HitTest(x, y int) (r image.Rectangle, ok bool) {
	hit, offset, _ := v.hitTest(x, y)
	if hit == nil {
		return image.Rectangle{}, false
	}
	return hit.Bounds().Add(offset), true
}

// hitTest is HitTest's real implementation, additionally reporting
// which top-level slot (x, y) resolved to (meaningful only when hit is
// non-nil).
func (v *View) hitTest(x, y int) (hit engine.Hit, offset image.Point, slot int) {
	if !v.stack.laidOut() || !image.Pt(x, y).In(v.bounds) {
		return nil, image.Point{}, 0
	}
	origin := v.contentOrigin()
	c := v.stack.at(y - origin.Y)
	if c.Index < 0 || c.Index >= v.stack.len() {
		return nil, image.Point{}, 0
	}
	box := v.stack.box.BoxAt(c.Index)
	local := image.Pt(x-origin.X, int(c.Offset))
	if !local.In(box.Bounds()) {
		return nil, image.Point{}, c.Index
	}
	hit, offset = box.HitTest(local)
	if hit == nil {
		return nil, image.Point{}, c.Index
	}
	// From box's own frame back to the canvas: box's top is c.Offset
	// above y.
	return hit, offset.Add(image.Pt(origin.X, y-int(c.Offset))), c.Index
}

// linkNodeAt returns the ast.Node of the link at (x, y), or nil if none,
// and the top-level slot it's in.
func (v *View) linkNodeAt(x, y int) (node *ast.Node, slot int) {
	hit, _, slot := v.hitTest(x, y)
	if hit == nil {
		return nil, 0
	}
	return hit.Source().Node().AncestorTag(ast.TagLink), slot
}

// hover updates which link is highlighted, and which sideways-scrolling
// block shows its scrollbar, given the pointer at (x, y). It also
// reports the link there, like LinkAt.
func (v *View) hover(x, y int) (destination string, ok bool) {
	if v.hscroll != nil {
		v.hscroll.hover(image.Pt(x, y), v.ctx.Time)
	}
	return v.hoverLink(x, y)
}

// unhover clears what hover set: the pointer is gone.
func (v *View) unhover() {
	if v.hscroll != nil {
		v.hscroll.unhover()
	}
	v.setHighlight(nil, 0)
}

// hoverLink is hover for links only.
func (v *View) hoverLink(x, y int) (destination string, ok bool) {
	node, slot := v.linkNodeAt(x, y)
	v.setHighlight(node, slot)
	if node == nil {
		return "", false
	}
	return node.Destination, true
}

// setHighlight highlights the link node, in top-level slot slot, or
// none if node is nil.
func (v *View) setHighlight(node *ast.Node, slot int) {
	if node != v.ctx.HighlightNode {
		if v.ctx.HighlightNode != nil && v.stack.laidOut() {
			v.stack.invalidate(v.highlightSlot)
		}
		v.ctx.HighlightNode = node
		if node != nil {
			v.stack.invalidate(slot)
		}
	}
	if node != nil {
		v.highlightSlot = slot
	}
}

// HoveredLink returns the destination of the link under the pointer, as
// written in the document, or ok=false if there's none. A Controller
// keeps it up to date; it's highlighted in the StyleSheet's
// HighlightColor.
func (v *View) HoveredLink() (destination string, ok bool) {
	if n := v.ctx.HighlightNode; n != nil {
		return n.Destination, true
	}
	return "", false
}

// LinkAt reports the destination URL of the link at (x, y). ok is false
// if (x, y) doesn't land on a link.
func (v *View) LinkAt(x, y int) (destination string, ok bool) {
	node, _ := v.linkNodeAt(x, y)
	if node == nil {
		return "", false
	}
	return node.Destination, true
}

// invalidateChangedImages picks up images that have settled since the
// last call (see imagecache.Cache). Only slots waiting on an image that
// changed are invalidated.
func (v *View) invalidateChangedImages() {
	if !v.stack.laidOut() || v.ctx.ImageCache == nil {
		return
	}
	changes, mark := v.ctx.ImageCache.ChangedSince(v.imageCacheMark)
	v.imageCacheMark = mark
	if len(changes) == 0 {
		return
	}
	changed := make(map[string]bool, len(changes))
	for _, c := range changes {
		changed[c.Key] = true
	}
	v.stack.invalidateWhere(func(box engine.BlockLayout) bool {
		for _, src := range box.PendingImages() {
			if changed[src] {
				return true
			}
		}
		return false
	})
}

// prefetchImageSourceHeightRadius is prefetchImageSources' reach - much
// larger than preLayoutHeightRadius, since it never lays anything out.
const prefetchImageSourceHeightRadius = 20000

// prefetchImageSources starts loading the image of every top-level
// image-only paragraph within prefetchImageSourceHeightRadius of the
// viewport - the common case of a large image (a screenshot, a diagram)
// whose real size would otherwise disrupt the height estimate once it
// scrolls into view. An image mixed into running text is loaded when its
// slot is laid out.
func (v *View) prefetchImageSources(viewportHeight int) {
	if !v.stack.laidOut() || v.ctx.ImageCache == nil {
		return
	}
	v.stack.forEachNearby(viewportHeight, prefetchImageSourceHeightRadius, func(block engine.Block) {
		if img, ok := v.soleImages[block]; ok {
			v.ctx.ImageCache.Load(img)
		}
	})
}

// SetStyleSheet swaps the View's StyleSheet and takes effect immediately:
// style values are resolved when the layout is built, so it's rebuilt
// here, re-anchoring the scroll position the same way a resize does. If
// nothing has been laid out yet, SetBounds lays it out with s.
func (v *View) SetStyleSheet(s StyleSheet) {
	v.ctx.Styles = s.Styles()
	if v.stack.laidOut() {
		v.rebuild()
	}
}

// rebuild lays out the document afresh at the current width.
//
// The StyleSheet's ViewMargins top/bottom become spacer slots, so they're
// real (if empty) space: scrolling clamps into the bottom margin like the
// true end of the document, and a click inside either margin misses.
// Left/Right instead narrow the laid-out width; Draw/HitTest shift by Left
// to compensate.
func (v *View) rebuild() {
	v.take()
	v.stack.setBox(v.layout())
}

// layout returns a new layout of the View's blocks, at the current width.
func (v *View) layout() *engine.StackBox {
	margin := v.ctx.ScaledViewMargins()
	contentWidth := max(0, v.width-int(margin.Left)-int(margin.Right))
	box := (&engine.StackBlock{Blocks: v.blocks}).StackLayout(&v.ctx, contentWidth)
	box.AddSpacers(int(margin.Top), int(margin.Bottom))
	return box
}

// follow takes what the View's Document has gained since it last looked,
// and lays it out, keeping the layout of what it had: blocks are only
// ever added at the end, or the unfinished ones there replaced.
func (v *View) follow() {
	if v.take() && v.stack.laidOut() {
		v.stack.extend(v.layout())
	}
}

// take takes what the View's Document has gained since it last looked
// into its blocks, reporting whether there was anything.
func (v *View) take() bool {
	if v.doc == nil {
		return false
	}
	if v.reader.doc == nil {
		v.reader.doc = v.doc
	}
	added, tail, ok := v.reader.next()
	if !ok {
		return false
	}
	v.blocks = v.blocks[:len(v.blocks)-v.tailBlocks]
	for _, pieces := range [][]markdown.Piece{added, tail} {
		for _, p := range pieces {
			v.blocks = append(v.blocks, p.Block)
			if p.SoleImage != nil {
				if v.soleImages == nil {
					v.soleImages = map[engine.Block]fetch.Source{}
				}
				v.soleImages[p.Block] = p.SoleImage
			}
		}
	}
	v.tailBlocks = len(tail)
	return true
}
