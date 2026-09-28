package whynot

import (
	"image"
	"time"
)

// View renders a Document onto a Canvas. It owns the layout cache (rebuilt
// only when width, scale or StyleSheet change, not every frame), viewport
// culling, and scroll-position anchoring across resizes - the parts of
// rendering a reflowing document that are easy to get wrong and not
// specific to any one game.
//
// A View doesn't read input itself: call Scroll with deltas from whatever
// input source is appropriate for the embedding game, and call Layout
// whenever the available width or the display scale changes (typically
// from the embedding game's own layout/resize callback).
type View struct {
	doc *Document
	ctx RenderingContext

	// stack is the laid-out document and the scroll position within it.
	stack documentStack

	// width and scale are what stack was last laid out at.
	width int
	scale float64

	// imageCacheMark is the ImageCache.ChangedSince mark from the last
	// time Layout checked for image state changes.
	imageCacheMark uint64

	// highlightSlot is the top-level slot containing ctx.HighlightNode
	// (meaningless when HighlightNode is nil). Hover refreshes it on every
	// call, so it self-heals after an intervening rebuild.
	highlightSlot int
}

// ViewOption customizes a View at construction, via NewView's opts
// parameter.
type ViewOption func(*View)

// WithStyleSheet overrides the StyleSheet NewView otherwise defaults to
// (NewDarkStyleSheet) - e.g. NewView(doc, faceSelector,
// WithStyleSheet(NewLightStyleSheet())).
func WithStyleSheet(s StyleSheet) ViewOption {
	return func(v *View) {
		v.ctx.StyleSheet = s
	}
}

// WithImageSource overrides the ImageSource NewView otherwise defaults
// to (FileImageSource) - e.g. for an embedder that wants images
// resolved relative to a document's own location, or fetched over
// http(s), the way cmd/whynot does. Wrapped in an ImageCache, so each
// image is resolved and fetched only once for the life of the View.
func WithImageSource(s ImageSource) ViewOption {
	return func(v *View) {
		v.ctx.ImageCache = NewImageCache(s)
	}
}

// NewView returns a View of doc (see Parse), ready to render it once Layout
// has been called at least once to establish a width.
func NewView(doc *Document, faceSelector FaceSelector, opts ...ViewOption) *View {
	v := &View{
		doc: doc,
		ctx: RenderingContext{
			FaceSelector: faceSelector,
			StyleSheet:   NewDarkStyleSheet(),
			ImageCache:   NewImageCache(FileImageSource{}),
		},
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
// ok=false if the document has no heading before the current scroll
// position, or nothing has been laid out yet. Only reads each slot's
// Block, never laying anything out.
func (v *View) CurrentHeadingID() (id string, ok bool) {
	if !v.stack.laidOut() {
		return "", false
	}
	for i := min(v.stack.cursor.index, v.stack.len()-1); i >= 0; i-- {
		if n := nodeOf(v.stack.blockAt(i)); n != nil && n.Tag >= TagHeading1 && n.Tag <= TagHeading6 {
			return n.ID, true
		}
	}
	return "", false
}

// nodeOf returns block's ASTNode, or nil for a nil block (a spacer slot).
func nodeOf(block Block) *ASTNode {
	if block == nil {
		return nil
	}
	return block.Node()
}

// Scroll adjusts the vertical scroll position by dy pixels: negative dy
// moves the cursor forward through the document, revealing later content
// ("scrolling down"); positive moves back toward the start. This matches
// ebiten.Wheel()'s dy passed straight through, so callers don't need to
// negate it.
func (v *View) Scroll(dy float64) {
	if v.stack.laidOut() {
		v.stack.scroll(-dy)
	}
}

// ScrollPosition is an opaque snapshot of a View's scroll position,
// captured by ScrollPosition and restored by RestoreScrollPosition.
type ScrollPosition struct {
	cursor stackCursor
}

// ScrollPosition captures the View's current scroll position.
func (v *View) ScrollPosition() ScrollPosition {
	return ScrollPosition{cursor: v.stack.cursor}
}

// RestoreScrollPosition restores a position captured earlier by
// ScrollPosition - only meaningful on the same View it was taken from.
func (v *View) RestoreScrollPosition(p ScrollPosition) {
	v.stack.cursor = p.cursor
}

// ScrollToAnchor scrolls to put the heading with the given anchor id
// (see ASTNode.ID) at the top of the viewport, e.g. after following a
// link with a URL fragment. ok is false, and the scroll position
// unchanged, if no heading has that id or nothing has been laid out
// yet (see Layout).
//
// Only top-level headings are found - a heading nested inside a
// blockquote or list won't be. Only reads each slot's Block, never laying
// anything out beyond the target.
func (v *View) ScrollToAnchor(id string) bool {
	if !v.stack.laidOut() {
		return false
	}
	for i := range v.stack.len() {
		if n := nodeOf(v.stack.blockAt(i)); n != nil && n.ID == id {
			v.stack.scrollToSlot(i)
			return true
		}
	}
	return false
}

// ScrollToRatio sets the scroll position to ratio (clamped to [0, 1])
// through the document's current best-estimated height - e.g. for a
// caller implementing scrollbar-thumb dragging, where DocumentBounds/
// VisibleViewBounds already give the inverse ratio back. Recomputes
// from the live estimate every call, so a jump into not-yet-laid-out
// territory corrects toward whatever ratio the caller asks for next.
func (v *View) ScrollToRatio(ratio float64) {
	if v.stack.laidOut() {
		v.stack.scrollToRatio(ratio)
	}
}

// ScaledViewMargins returns the View's current effective margin between
// its viewport edge and its content, in real screen pixels (already
// multiplied by the zoom/DPI scale most recently passed to Layout) -
// the same value Draw and HitTest use internally to place content,
// exposed so an embedder positioning something else relative to the
// View (e.g. a scrollbar drawn beside it) can stay in exact agreement
// with it.
func (v *View) ScaledViewMargins() Margins {
	return v.ctx.ScaledViewMargins()
}

// DocumentBounds returns the document's estimated extent, origin at
// (0, 0): width is the last Layout width; height is the current best
// estimate of the total, exact once every slot has been laid out. A
// caller scales the ratio between this and VisibleViewBounds to build its
// own scrollbar.
func (v *View) DocumentBounds() image.Rectangle {
	if !v.stack.laidOut() {
		return image.Rectangle{}
	}
	return image.Rect(0, 0, v.width, int(v.stack.totalHeight()))
}

// VisibleViewBounds returns the sub-rectangle of DocumentBounds
// currently visible for a viewport of viewportSize (the same size
// passed to Draw's dst).
func (v *View) VisibleViewBounds(viewportSize image.Point) image.Rectangle {
	if !v.stack.laidOut() || v.stack.cursor.index >= v.stack.len() {
		return image.Rectangle{}
	}
	top, bottom := v.stack.visibleRange(viewportSize.Y)
	return image.Rect(0, int(top), viewportSize.X, int(bottom))
}

// Draw renders the document onto dst with its top-left corner at (x, y),
// at the current scroll position. Content above the current cursor, and
// content outside dst's bounds, is never laid out or drawn - Draw's cost
// tracks what's visible, not the document's total size or how far into it
// the scroll position is.
//
// Draw first fills dst's whole bounds with the StyleSheet's background
// color, so a caller doesn't need its own clear step.
func (v *View) Draw(dst Canvas, x, y int) {
	bounds := dst.Bounds()
	dst.DrawRect(bounds.Min.X, bounds.Min.Y, bounds.Dx(), bounds.Dy(), v.ctx.StyleSheet.BackgroundColor())
	if !v.stack.laidOut() {
		return
	}
	// The top/bottom ViewMargins are spacer slots in the stack (see
	// rebuild); only Left needs applying here.
	v.stack.box.DrawFrom(dst, v.stack.cursor, x+int(v.ctx.ScaledViewMargins().Left), y, v.ctx.Time)
}

// HitTest identifies what's at document position (x, y) - the same
// coordinate space Draw's (x, y) places content's origin into. hit is
// nil if (x, y) doesn't land on any content - past the end of the
// document, or in a margin/gap. Otherwise offset is where hit.Bounds()
// should be placed to get its absolute position in that same space,
// e.g. for drawing an outline around it.
//
// Known imprecision: a position above the very start of the document
// resolves as if it landed on the first slot rather than missing. Callers
// only ever pass points within their own rendered viewport.
func (v *View) HitTest(x, y int) (hit Hit, offset image.Point) {
	hit, offset, _ = v.hitTest(x, y)
	return hit, offset
}

// hitTest is HitTest's real implementation, additionally reporting
// which top-level slot y resolved to (meaningful only when hit is
// non-nil).
func (v *View) hitTest(x, y int) (hit Hit, offset image.Point, slot int) {
	if !v.stack.laidOut() {
		return nil, image.Point{}, 0
	}
	left := int(v.ctx.ScaledViewMargins().Left)
	c := v.stack.at(y)
	if c.index < 0 || c.index >= v.stack.len() {
		return nil, image.Point{}, 0
	}
	box := v.stack.box.boxAt(c.index)
	local := image.Pt(x-left, int(c.offset))
	if !local.In(box.Bounds()) {
		return nil, image.Point{}, c.index
	}
	hit, offset = box.HitTest(local)
	if hit == nil {
		return nil, image.Point{}, c.index
	}
	// Shift back from box's own local frame into the frame (x, y) arrived
	// in, undoing both the left-margin shift and the cursor-relative y.
	return hit, offset.Add(image.Pt(left, y-int(c.offset))), c.index
}

// linkNodeAt returns the ASTNode of the link at document position
// (x, y), or nil if none - same coordinate space as HitTest - and the
// top-level slot it's in.
func (v *View) linkNodeAt(x, y int) (node *ASTNode, slot int) {
	hit, _, slot := v.hitTest(x, y)
	if hit == nil {
		return nil, 0
	}
	return hit.Source().Node().AncestorTag(TagLink), slot
}

// Hover updates the currently-highlighted link, given the mouse position
// in the same coordinate space HitTest/Draw use - call every frame from
// the embedding game's own input handling. Highlighting only changes
// color, never layout, so only the slot left and the slot entered are
// invalidated.
//
// Hover also reports the link under (x, y), same as LinkAt, so a caller
// handling a click at the same position doesn't need a second HitTest.
func (v *View) Hover(x, y int) (destination string, ok bool) {
	node, slot := v.linkNodeAt(x, y)
	if node != v.ctx.HighlightNode {
		if v.ctx.HighlightNode != nil && v.stack.laidOut() {
			v.stack.invalidate(v.highlightSlot)
		}
		v.ctx.HighlightNode = node
		if node != nil {
			v.stack.invalidate(slot)
		}
	}
	if node == nil {
		return "", false
	}
	v.highlightSlot = slot
	return node.Destination, true
}

// LinkAt reports the destination URL of the link at document position
// (x, y) - the same coordinate space HitTest/Hover use. ok is false if
// (x, y) doesn't land on a link. Unlike Hover, this never changes the
// highlight.
func (v *View) LinkAt(x, y int) (destination string, ok bool) {
	node, _ := v.linkNodeAt(x, y)
	if node == nil {
		return "", false
	}
	return node.Destination, true
}

// Layout sets the pixel width and display scale to render at (DPI = scale
// * 72), and now - elapsed time since rendering started, the embedder's
// own reference point (see RenderingContext.Time), stored on every call
// since animated images need fresh time to animate. Cheap to call every
// frame: the layout is only rebuilt when width or scale change.
//
// height is the viewport's height - not used for wrapping, only to know
// where the visible area ends when laying out and prefetching ahead.
func (v *View) Layout(width, height int, scale float64, now time.Duration) {
	v.ctx.SetDPI(scale * 72)
	v.ctx.Scale = scale
	v.ctx.Time = now

	if width == v.width && scale == v.scale {
		v.invalidateChangedImages()
	} else {
		v.width = width
		v.scale = scale
		v.rebuild()
	}
	v.stack.preLayout(height)
	v.prefetchImageSources(height)
}

// invalidateChangedImages is Layout's response to an unchanged
// width/scale: an image may have settled since the last call (see
// ImageCache) and need picking up. Only slots waiting on an image that
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
		changed[c.Src] = true
	}
	v.stack.invalidateWhere(func(box BlockLayout) bool {
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
	v.stack.forEachNearby(viewportHeight, prefetchImageSourceHeightRadius, func(block Block) {
		if src, ok := v.doc.soleImages[block]; ok {
			v.ctx.ImageCache.Load(src)
		}
	})
}

// SetStyleSheet swaps the View's StyleSheet and takes effect immediately:
// style values are resolved when the layout is built, so it's rebuilt
// here, re-anchoring the scroll position the same way a resize does. If
// nothing has been laid out yet, the first Layout call picks s up.
func (v *View) SetStyleSheet(s StyleSheet) {
	v.ctx.StyleSheet = s
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
	margin := v.ctx.ScaledViewMargins()
	contentWidth := max(0, v.width-int(margin.Left)-int(margin.Right))
	box := v.doc.root.stackLayout(&v.ctx, contentWidth)
	box.addSpacers(int(margin.Top), int(margin.Bottom))
	v.stack.setBox(box)
}
