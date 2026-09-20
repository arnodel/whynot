package whynot

import (
	"image"
	"math"
	"strings"
	"time"
)

// View renders a parsed Markdown document onto a Canvas. It owns the
// layout cache (rebuilt only when width or scale change, not every frame),
// viewport culling, and scroll-position anchoring across resizes - the
// parts of rendering a reflowing document that are easy to get wrong and
// not specific to any one game, so a caller doesn't have to reimplement
// them.
//
// A View doesn't read input itself: call Scroll with deltas from whatever
// input source is appropriate for the embedding game, and call Layout
// whenever the available width or the display scale changes (typically
// from the embedding game's own layout/resize callback).
type View struct {
	block Block
	ctx   RenderingContext

	// cursor is the scroll position: which slot is at the top of the
	// viewport, and how far into it. This is what stays meaningful across
	// a rebuild, unlike a raw pixel offset - reflow can change a slot's
	// height, but not which slot content belongs to.
	cursor stackCursor

	box      *StackBox
	boxWidth int
	boxScale float64

	// imageCacheMark is the ImageCache.ChangedSince mark from the last
	// time Layout checked for image state changes - see
	// invalidateChangedImages.
	imageCacheMark uint64

	// highlightSlot is the top-level slot index containing
	// ctx.HighlightNode (meaningless when HighlightNode is nil). Hover
	// refreshes it on every call, not just on a change, so it self-heals
	// after an intervening rebuild without needing a separate "is this
	// still trustworthy" flag.
	highlightSlot int

	// slotHeights holds the last-known-or-estimated height of each
	// v.box top-level slot, in pixels - -1 for never known. Refreshed
	// lazily by refreshSlotHeights; otherwise left alone across
	// invalidation or rebuild, since a stale per-slot value is a better
	// estimate than falling back to the document-wide average.
	slotHeights []float64
}

// ViewOption customizes a View at construction, via NewView's opts
// parameter.
type ViewOption func(*View)

// WithStyleSheet overrides the StyleSheet NewView otherwise defaults to
// (NewDarkStyleSheet) - e.g. NewView(source, faceSelector,
// WithStyleSheet(NewLightStyleSheet())).
func WithStyleSheet(s StyleSheet) ViewOption {
	return func(v *View) {
		v.ctx.StyleSheet = s
	}
}

// WithImageSource overrides the ImageSource NewView otherwise defaults
// to (FileImageSource) - e.g. for an embedder that wants images
// resolved relative to a document's own location, or fetched over
// http(s), the way cmd/whynot does. Wrapped in a cache automatically
// (see ImageCache), so resolving/fetching a given image only happens
// once for the life of the resulting View, however many times it's
// asked for (every layout rebuild - resize, zoom, theme change, even
// hovering a different link - asks again).
func WithImageSource(s ImageSource) ViewOption {
	return func(v *View) {
		v.ctx.ImageCache = NewImageCache(s)
	}
}

// NewView parses source and returns a View ready to render it once Layout
// has been called at least once to establish a width.
func NewView(source []byte, faceSelector FaceSelector, opts ...ViewOption) *View {
	v := &View{
		block: Parse(source),
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

// Title returns the document's own title - the text of its first
// heading, at any level - or ok=false if it has none. A caller (e.g.
// for a window/tab title) decides its own fallback; a document with no
// heading at all is a normal, unremarkable case, not an error.
//
// Only a top-level heading is found, the same limitation
// ScrollToAnchor has and for the same reason: one nested inside a
// blockquote or list isn't reachable this way.
func (v *View) Title() (string, bool) {
	stack, ok := v.block.(*StackBlock)
	if !ok {
		return "", false
	}
	for _, block := range stack.blocks {
		node := block.Node()
		if node == nil || node.Tag < TagHeading1 || node.Tag > TagHeading6 {
			continue
		}
		mb, ok := block.(*MarginBlock)
		if !ok {
			continue
		}
		tb, ok := mb.Block.(*TextBlock)
		if !ok {
			continue
		}
		return plainTextOf(tb), true
	}
	return "", false
}

// plainTextOf reconstructs a TextBlock's plain text - its parts are
// mostly *InlineText, one per word (see appendString), rejoined with
// single spaces; anything else (e.g. an *InlineImage, if a heading
// contained one) contributes nothing rather than failing the whole
// result.
func plainTextOf(tb *TextBlock) string {
	var words []string
	for _, part := range tb.parts {
		if it, ok := part.(*InlineText); ok {
			words = append(words, it.text)
		}
	}
	return strings.Join(words, " ")
}

// Scroll adjusts the vertical scroll position by dy pixels: negative dy
// moves the cursor forward through the document, revealing later content
// ("scrolling down"); positive moves back toward the start. This matches
// ebiten.Wheel()'s dy passed straight through, so callers don't need to
// negate it.
func (v *View) Scroll(dy float64) {
	if v.box == nil {
		return
	}
	v.cursor = v.box.moveCursor(v.cursor, -dy)
}

// ScrollPosition is an opaque snapshot of a View's scroll position,
// captured by ScrollPosition and restored by RestoreScrollPosition.
type ScrollPosition struct {
	cursor stackCursor
}

// ScrollPosition captures the View's current scroll position.
func (v *View) ScrollPosition() ScrollPosition {
	return ScrollPosition{cursor: v.cursor}
}

// RestoreScrollPosition restores a position captured earlier by
// ScrollPosition - only meaningful on the same View it was taken from.
func (v *View) RestoreScrollPosition(p ScrollPosition) {
	v.cursor = p.cursor
}

// ScrollToAnchor scrolls to put the heading with the given anchor id
// (see ASTNode.ID) at the top of the viewport, e.g. after following a
// link with a URL fragment. ok is false, and the scroll position
// unchanged, if no heading has that id or nothing has been laid out
// yet (see Layout).
//
// Only top-level headings are found - a heading nested inside a
// blockquote or list won't be. Cheap regardless of document size: a
// slot's own Node() is known without resolving its content into a box
// (see StackBox.boxAt), so this never lays out anything beyond what's
// already built.
func (v *View) ScrollToAnchor(id string) bool {
	if v.box == nil {
		return false
	}
	for i, slot := range v.box.slots {
		if slot.block == nil {
			continue
		}
		if n := slot.block.Node(); n != nil && n.ID == id {
			v.cursor = v.box.normalizeCursor(stackCursor{index: i})
			return true
		}
	}
	return false
}

// ScrollToRatio sets the scroll position to ratio (clamped to [0, 1])
// through the document's current best-estimated height - e.g. for a
// caller implementing scrollbar-thumb dragging, where DocumentBounds/
// VisibleViewBounds already give the inverse ratio back. Recomputes
// from the live estimate every call rather than a target captured
// once, so a jump into not-yet-resolved territory only ever corrects
// toward whatever ratio the caller asks for next, never drifts from
// it.
func (v *View) ScrollToRatio(ratio float64) {
	if v.box == nil {
		return
	}
	ratio = math.Max(0, math.Min(1, ratio))
	avg := v.refreshSlotHeights()
	var total float64
	for i := range v.slotHeights {
		total += v.estimatedHeight(i, avg)
	}
	v.cursor = v.box.normalizeCursor(v.cursorAtOffset(ratio*total, avg))
}

// cursorAtOffset returns the stackCursor for document offset y, using
// avg (see refreshSlotHeights) for any not-yet-resolved slot - the
// caller normalizes it afterward, which resolves the landing slot for
// real and corrects offset if the estimate was off enough to spill
// into a neighbor.
func (v *View) cursorAtOffset(y, avg float64) stackCursor {
	for i := range v.box.slots {
		h := v.estimatedHeight(i, avg)
		if y < h || i == len(v.box.slots)-1 {
			return stackCursor{index: i, offset: y}
		}
		y -= h
	}
	return stackCursor{}
}

// refreshSlotHeights updates v.slotHeights in place from whatever's
// currently resolved (see StackBox.boxAt) and returns avg - the value
// estimatedHeight substitutes for any slot still at -1 (never resolved
// or seeded). Doesn't allocate beyond the one-time resize when the
// slot count itself changes.
//
// Re-reads Bounds() on every already-resolved slot on every call,
// rather than tracking which slots are already known-fresh and
// skipping them - deliberately: every BlockLayout.Bounds() this
// project has (StackBox, TextBox, LineBox, ...) already memoizes
// itself internally (its own "computed" flag, checked once and
// cached), so a slot whose box hasn't changed costs a cheap flag check
// here regardless, not real work. Tracking box identity here too to
// skip even that would need an extra parallel slice kept in sync at
// every write site (see VisibleViewBounds's forcing walk) for a
// measured difference of ~0 (benchmarked on testdata/test.md) - worth
// revisiting only if some future BlockLayout's Bounds() stops being
// cheap to call repeatedly.
func (v *View) refreshSlotHeights() (avg float64) {
	if len(v.slotHeights) != len(v.box.slots) {
		v.slotHeights = make([]float64, len(v.box.slots))
		for i := range v.slotHeights {
			v.slotHeights[i] = -1
		}
	}
	var sum float64
	var knownCount int
	for i := range v.box.slots {
		if box := v.box.slots[i].box; box != nil {
			v.slotHeights[i] = float64(box.Bounds().Dy())
		}
		if v.slotHeights[i] >= 0 {
			sum += v.slotHeights[i]
			knownCount++
		}
	}
	if knownCount == 0 {
		return 0
	}
	return sum / float64(knownCount)
}

// estimatedHeight returns slot i's real height if known, else avg
// (see refreshSlotHeights).
func (v *View) estimatedHeight(i int, avg float64) float64 {
	if h := v.slotHeights[i]; h >= 0 {
		return h
	}
	return avg
}

// ScaledViewMargins returns the View's current effective margin between
// its viewport edge and its content, in real screen pixels (already
// multiplied by the zoom/DPI scale most recently passed to Layout) -
// the same value Draw and HitTest use internally to place content,
// exposed so an embedder positioning something else relative to the
// View (e.g. cmd/whynot's own scrollbar, drawn beside it rather than
// by it) can stay in exact agreement with it.
func (v *View) ScaledViewMargins() Margins {
	return v.ctx.ScaledViewMargins()
}

// DocumentBounds returns the document's estimated extent, origin at
// (0, 0): width is the last Layout width; height is the current best
// estimate of the total (see refreshSlotHeights), exact once every
// slot has been resolved. A caller scales the ratio between this and
// VisibleViewBounds to build its own scrollbar.
func (v *View) DocumentBounds() image.Rectangle {
	if v.box == nil {
		return image.Rectangle{}
	}
	avg := v.refreshSlotHeights()
	var total float64
	for i := range v.slotHeights {
		total += v.estimatedHeight(i, avg)
	}
	return image.Rect(0, 0, v.boxWidth, int(total))
}

// VisibleViewBounds returns the sub-rectangle of DocumentBounds
// currently visible for a viewport of viewportSize (the same size
// passed to Draw's dst). The bottom edge is exactly top+viewportSize.Y,
// clamped to the document's total - deliberately not "wherever the
// last slot DrawFrom would draw happens to end": a slot's own bottom
// can overshoot the viewport by a lot (a large image or table), and
// which slot ends up last shifts discretely while scrolling, so
// pinning the result to it made the thumb's size visibly jump - a
// quantization artifact independent of estimation accuracy. The
// forward walk below still force-resolves the same slots DrawFrom
// would draw this frame, as a side effect: no laziness benefit in
// skipping it, since Draw resolves them anyway, and doing so keeps the
// document total as accurate as possible near the viewport.
func (v *View) VisibleViewBounds(viewportSize image.Point) image.Rectangle {
	if v.box == nil || v.cursor.index >= len(v.box.slots) {
		return image.Rectangle{}
	}
	avg := v.refreshSlotHeights()
	var before, total float64
	for i := range v.slotHeights {
		h := v.estimatedHeight(i, avg)
		total += h
		if i < v.cursor.index {
			before += h
		}
	}
	top := before + v.cursor.offset

	// The walk below resolves each slot it touches for real, which can
	// replace that slot's avg-based contribution to total with a very
	// different real one (see TestViewVisibleViewBoundsResolvesRealHeights) -
	// correct total in place rather than re-deriving it with a second
	// full pass over every slot afterward.
	pos := before // running position, starts at the top of cursor.index's own slot
	for i := v.cursor.index; i < len(v.box.slots); i++ {
		if pos-top > float64(viewportSize.Y) {
			break
		}
		estimated := v.estimatedHeight(i, avg)
		real := float64(v.box.boxAt(i).Bounds().Dy())
		v.slotHeights[i] = real
		total += real - estimated
		pos += real
	}

	bottom := top + float64(viewportSize.Y)
	if bottom > total {
		bottom = total
	}
	return image.Rect(0, int(top), viewportSize.X, int(bottom))
}

// Draw renders the document onto dst with its top-left corner at (x, y),
// at the current scroll position. Content above the current cursor, and
// content outside dst's bounds, is never resolved or drawn - Draw's cost
// tracks what's visible, not the document's total size or how far into it
// the scroll position is.
//
// Draw fills dst's whole bounds with the StyleSheet's background color
// first - a caller doesn't need its own clear/fill step (or to know the
// StyleSheet's background color itself) before calling Draw. The
// ViewMargins' top/bottom is already baked into the scroll position (see
// rebuild); only Left needs applying here.
func (v *View) Draw(dst Canvas, x, y int) {
	bounds := dst.Bounds()
	dst.DrawRect(bounds.Min.X, bounds.Min.Y, bounds.Dx(), bounds.Dy(), v.ctx.StyleSheet.BackgroundColor())
	if v.box == nil {
		return
	}
	v.box.DrawFrom(dst, v.cursor, x+int(v.ctx.ScaledViewMargins().Left), y, v.ctx.Time)
}

// HitTest identifies what's at document position (x, y) - the same
// coordinate space Draw's (x, y) places content's origin into. hit is
// nil if (x, y) doesn't land on any content - past the end of the
// document, or in a margin/gap. Otherwise offset is where hit.Bounds()
// should be placed to get its absolute position in that same space,
// e.g. for drawing an outline around it.
//
// y is resolved via normalizeCursor, the same cursor-relative walk
// Scroll/Layout use, so a click far from the current scroll position
// doesn't force-build every slot in between.
//
// Known imprecision: normalizeCursor clamps a position above the very
// start of the document to the same value as the start itself, so
// clicking well above the rendered content resolves as if it landed on
// the first slot rather than missing. Not worth fixing since callers
// only ever pass points within their own rendered viewport.
func (v *View) HitTest(x, y int) (hit Hit, offset image.Point) {
	hit, offset, _ = v.hitTest(x, y)
	return hit, offset
}

// hitTest is HitTest's real implementation, additionally reporting
// which top-level slot p.Y resolved to (meaningful only when hit is
// non-nil) - reused by linkNodeAt/Hover to avoid a second cursor walk.
func (v *View) hitTest(x, y int) (hit Hit, offset image.Point, slot int) {
	if v.box == nil {
		return nil, image.Point{}, 0
	}
	left := int(v.ctx.ScaledViewMargins().Left)
	c := v.box.normalizeCursor(stackCursor{index: v.cursor.index, offset: v.cursor.offset + float64(y)})
	if c.index < 0 || c.index >= len(v.box.slots) {
		return nil, image.Point{}, 0
	}
	box := v.box.boxAt(c.index)
	local := image.Pt(x-left, int(c.offset))
	if !local.In(box.Bounds()) {
		return nil, image.Point{}, c.index
	}
	hit, offset = box.HitTest(local)
	if hit == nil {
		return nil, image.Point{}, c.index
	}
	// Shift back from box's own local frame into the same frame (x, y)
	// arrived in - undoing both the left-margin shift and the
	// cursor-relative adjustment made to y above.
	return hit, offset.Add(image.Pt(left, y-int(c.offset))), c.index
}

// linkNodeAt returns the ASTNode of the link at document position
// (x, y), or nil if none - same coordinate space as HitTest. slot is
// the resolved top-level slot (meaningful only when node is non-nil),
// reused by Hover to invalidate without a second cursor walk.
func (v *View) linkNodeAt(x, y int) (node *ASTNode, slot int) {
	hit, _, slot := v.hitTest(x, y)
	if hit == nil {
		return nil, 0
	}
	return hit.Source().Node().AncestorTag(TagLink), slot
}

// Hover updates the currently-highlighted link, given the mouse position
// in the same coordinate space HitTest/Draw use - call every frame from
// the embedding game's own input handling. Highlighting only ever
// changes color, never layout, so at most two slots (the one left, the
// one entered) are surgically invalidated rather than triggering a
// full rebuild.
//
// Hover also reports the link under (x, y), same as LinkAt, so a caller
// handling a click at the same position doesn't need a second HitTest.
func (v *View) Hover(x, y int) (destination string, ok bool) {
	node, slot := v.linkNodeAt(x, y)
	changed := node != v.ctx.HighlightNode
	if changed {
		oldNode := v.ctx.HighlightNode
		v.ctx.HighlightNode = node
		if v.box != nil {
			v.box.ctx.HighlightNode = node
			if oldNode != nil {
				v.invalidateSlot(v.highlightSlot)
			}
		}
	}
	if node != nil && v.box != nil {
		if changed {
			v.invalidateSlot(slot)
		}
		v.highlightSlot = slot // refreshed every call, not just on change
	}
	if node == nil {
		return "", false
	}
	return node.Destination, true
}

// invalidateSlot discards slot i's memoized box - boxAt lazily rebuilds
// it the next time something actually asks for it.
func (v *View) invalidateSlot(i int) {
	if i < 0 || i >= len(v.box.slots) {
		return
	}
	v.box.slots[i].box = nil
	v.box.boundsComputed = false
}

// LinkAt reports the destination URL of the link at document position
// (x, y) - the same coordinate space HitTest/Hover use. ok is false if
// (x, y) doesn't land on a link. Unlike Hover, this never changes the
// highlight - use it to query a position other than the current hover
// (e.g. from a test, or a separate input source).
func (v *View) LinkAt(x, y int) (destination string, ok bool) {
	node, _ := v.linkNodeAt(x, y)
	if node == nil {
		return "", false
	}
	return node.Destination, true
}

// Layout sets the pixel width and display scale to render at (DPI = scale
// * 72, matching main.go's convention), and now - elapsed time since
// rendering started, the embedder's own reference point (see
// RenderingContext.Time) - which it stores unconditionally, even when
// nothing else changed, since an animated image needs fresh time every
// call to actually animate. Cheap to call every frame otherwise: the
// layout tree only rebuilds when width or scale actually change.
//
// height is the viewport's own height - not used for wrapping (only
// width affects that), only so pre-layout/prefetch below know where
// the visible area ends. Not stored - used transiently within this call.
func (v *View) Layout(width, height int, scale float64, now time.Duration) {
	v.ctx.SetDPI(scale * 72)
	v.ctx.Scale = scale
	v.ctx.Time = now

	if width == v.boxWidth && scale == v.boxScale {
		v.invalidateChangedImages()
	} else {
		v.boxWidth = width
		v.boxScale = scale
		v.rebuild()
	}
	v.preLayoutNearby(height)
	v.prefetchImageSources(height)
}

// invalidateChangedImages is Layout's response to an unchanged
// width/scale: an image may have settled since the last call (see
// ImageCache) and need picking up. Only a slot actually waiting on a
// changed src is invalidated; everything else is left alone.
//
// A change with BoundsRevealed is special: the first time an image's
// real size becomes known can change its slot's height (every other
// transition happens at an already-known, already-laid-out size - see
// ImageChange.BoundsRevealed). That only matters if the cursor is
// anchored in that slot, in which case its offset is re-anchored by
// ratio (reanchorCursor) - scoped to that one slot rather than a full
// rebuild, which would otherwise discard every other slot's height
// estimate too.
//
// A changed slot before the cursor - already scrolled past - is
// re-resolved immediately instead of left lazily invalidated: nothing
// else ever revisits an earlier slot, so DocumentBounds' estimate would
// otherwise freeze at a stale value forever.
func (v *View) invalidateChangedImages() {
	if v.box == nil || v.ctx.ImageCache == nil {
		return
	}
	changes, newMark := v.ctx.ImageCache.ChangedSince(v.imageCacheMark)
	v.imageCacheMark = newMark
	if len(changes) == 0 {
		return
	}

	changedSrcs := make(map[string]bool, len(changes))
	for _, c := range changes {
		changedSrcs[c.Src] = true
	}

	var reanchor bool
	var oldHeight int
	for i := range v.box.slots {
		slot := v.box.slots[i]
		if slot.box == nil {
			continue
		}
		changed := false
		for _, src := range slot.box.PendingImages() {
			if changedSrcs[src] {
				changed = true
				break
			}
		}
		if !changed {
			continue
		}
		if i == v.cursor.index {
			reanchor, oldHeight = true, slot.box.Bounds().Dy()
		}
		v.invalidateSlot(i)
		if i < v.cursor.index {
			v.box.boxAt(i)
		}
	}
	if reanchor {
		v.reanchorCursor(oldHeight)
	}
}

// preLayoutHeightRadius is how far beyond the visible viewport
// preLayoutNearby fully lays out content ahead of time, in physical
// pixels - a few screens' worth for ordinary content.
const preLayoutHeightRadius = 3000

// preLayoutTimeBudget bounds how long preLayoutNearby spends per Layout
// call, so a big jump (ScrollToRatio, a resize, ...) spreads its
// catch-up over however many frames it takes rather than stalling one.
const preLayoutTimeBudget = 2 * time.Millisecond

// preLayoutNearby resolves slots within preLayoutHeightRadius of the
// visible viewport, in both directions, before Draw/VisibleViewBounds
// would otherwise force them for real - so a slot's real height is
// usually already known by the time the user scrolls there, instead of
// being discovered as a surprise right under the cursor (the
// scrollbar-jump bug this exists to fix). viewportHeight offsets only
// the forward walk, so it starts beyond the visible area rather than
// re-covering it; the backward walk needs no such adjustment.
func (v *View) preLayoutNearby(viewportHeight int) {
	if v.box == nil {
		return
	}
	deadline := time.Now().Add(preLayoutTimeBudget)
	v.preLayoutDirection(v.cursor.index, 1, float64(viewportHeight), deadline)
	v.preLayoutDirection(v.cursor.index-1, -1, 0, deadline)
}

// preLayoutDirection resolves slots starting at i, stepping by dir (+1
// forward, -1 backward), until accumulated height reaches
// preLayoutHeightRadius or deadline passes.
func (v *View) preLayoutDirection(i, dir int, skipHeight float64, deadline time.Time) {
	height := -skipHeight
	for step := 0; i >= 0 && i < len(v.box.slots) && height < preLayoutHeightRadius; step++ {
		if step&7 == 7 && time.Now().After(deadline) {
			return
		}
		height += float64(v.box.boxAt(i).Bounds().Dy())
		i += dir
	}
}

// prefetchImageSourceHeightRadius is prefetchImageSources' reach - much
// larger than preLayoutHeightRadius, since it never lays anything out,
// only checks each slot's already-parsed Block.
const prefetchImageSourceHeightRadius = 20000

// prefetchImageSources starts loading (ImageCache.Load) any standalone
// image paragraph within prefetchImageSourceHeightRadius of the visible
// viewport, in both directions - the dominant real case for a large,
// estimate-disrupting image (a screenshot, a diagram). An image mixed
// into running text is left to load when its slot is actually resolved,
// as before.
func (v *View) prefetchImageSources(viewportHeight int) {
	if v.box == nil || v.ctx.ImageCache == nil {
		return
	}
	avg := v.refreshSlotHeights()
	v.prefetchImageSourcesDirection(v.cursor.index, 1, float64(viewportHeight), avg)
	v.prefetchImageSourcesDirection(v.cursor.index-1, -1, 0, avg)
}

func (v *View) prefetchImageSourcesDirection(i, dir int, skipHeight, avg float64) {
	height := -skipHeight
	for i >= 0 && i < len(v.box.slots) && height < prefetchImageSourceHeightRadius {
		if src, ok := soleImageSrc(v.box.slots[i].block); ok {
			v.ctx.ImageCache.Load(src)
		}
		height += v.estimatedHeight(i, avg)
		i += dir
	}
}

// soleImageSrc reports the src of block's one InlineImage, if block is
// a TextBlock or ListItemHeadBlock whose only content is a single
// image - unwrapping MarginBlock first, since every real top-level
// paragraph is wrapped in one (see compile.go).
func soleImageSrc(block Block) (string, bool) {
	for {
		mb, ok := block.(*MarginBlock)
		if !ok {
			break
		}
		block = mb.Block
	}
	var parts []Inline
	switch b := block.(type) {
	case *TextBlock:
		parts = b.parts
	case *ListItemHeadBlock:
		parts = b.parts
	default:
		return "", false
	}
	if len(parts) != 1 {
		return "", false
	}
	img, ok := parts[0].(*InlineImage)
	if !ok {
		return "", false
	}
	return img.src, true
}

// SetStyleSheet swaps the View's StyleSheet and takes effect immediately -
// GetBlockLayout/GetInlineLayout resolve and bake in concrete style values (colors,
// margins collapsed to gaps, ...) when the layout tree is built, so
// without an explicit rebuild here the change wouldn't be visible until
// whatever next happened to resize the view. Rebuilding also re-anchors
// the scroll position by ratio through the current slot, the same way a
// resize does - a new StyleSheet can change content heights (different
// margins, font sizes, ...) just as reflowing at a new width can.
func (v *View) SetStyleSheet(s StyleSheet) {
	v.ctx.StyleSheet = s
	if v.box == nil {
		// Nothing laid out yet - the eventual first Layout call will pick
		// this StyleSheet up on its own; rebuilding now would only waste
		// a pass at a meaningless zero width.
		return
	}
	v.rebuild()
}

// rebuild re-lays-out the document at the current width. slotHeights
// is deliberately left alone (not reset) - the old per-slot heights
// remain useful seed estimates even though width/scale/StyleSheet can
// change any of them; heightEstimate reinitializes from scratch only
// if the slot count itself changed.
//
// The document is wrapped with a leading and trailing EmptyBox sized to
// the StyleSheet's ViewMargins, so the view's outer margin is real (if
// empty) space in the tree rather than a separate draw-time overlay:
// scrolling clamps into the bottom margin like the true end of the
// document, and a click inside either margin misses (EmptyBox.HitTest
// always declines). Left/Right instead narrow the width passed to
// GetBlockLayout; Draw/HitTest shift by Left to compensate.
func (v *View) rebuild() {
	oldHeight := 0
	if v.box != nil && v.cursor.index < len(v.box.slots) {
		oldHeight = v.box.boxAt(v.cursor.index).Bounds().Dy()
	}

	margin := v.ctx.ScaledViewMargins()
	contentWidth := v.boxWidth - int(margin.Left) - int(margin.Right)
	if contentWidth < 0 {
		contentWidth = 0
	}

	v.box = asStackBox(v.block.GetBlockLayout(v.ctx, contentWidth))
	if top := int(margin.Top); top > 0 {
		v.box.slots = append([]stackSlot{{box: NewEmptyBox(contentWidth, top)}}, v.box.slots...)
	}
	if bottom := int(margin.Bottom); bottom > 0 {
		v.box.slots = append(v.box.slots, stackSlot{box: NewEmptyBox(contentWidth, bottom)})
	}

	v.reanchorCursor(oldHeight)
}

// reanchorCursor rescales v.cursor's offset to the same proportion
// through its own slot's new height as oldHeight represented, so
// scrolling stays at the same logical position across a change that
// may have resized that slot - rather than leaving offset pointing at
// a leftover pixel that might now mean something else, or overflow the
// slot entirely. oldHeight <= 0 means no ratio is known, landing at
// the slot's own top instead.
func (v *View) reanchorCursor(oldHeight int) {
	ratio := 0.0
	if oldHeight > 0 {
		ratio = v.cursor.offset / float64(oldHeight)
	}
	if v.cursor.index >= len(v.box.slots) {
		v.cursor.index = len(v.box.slots) - 1
	}
	newHeight := v.box.boxAt(v.cursor.index).Bounds().Dy()
	v.cursor = v.box.normalizeCursor(stackCursor{index: v.cursor.index, offset: ratio * float64(newHeight)})
}
