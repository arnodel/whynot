package whynot

import (
	"image"
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
	// ctx.HighlightNode (meaningless when HighlightNode is nil) - kept
	// fresh by Hover on every call, not only on an actual highlight
	// change, specifically so it survives an intervening full rebuild
	// (Layout at a new width/scale, SetStyleSheet) that doesn't itself
	// change which node is highlighted: the very next Hover call
	// (cmd/whynot calls it every frame regardless) recomputes it from
	// the current (x, y) before it's ever needed to invalidate a slot.
	// One accepted, purely cosmetic edge case: if a full rebuild and a
	// jump straight from the highlighted link to a different one both
	// happen within the same tick - no intervening Hover call while the
	// mouse was simply sitting on the old link in between - this can be
	// stale at the exact moment it's used, leaving the old link
	// un-highlighted a frame late (or invalidating an unrelated slot,
	// wastefully but harmlessly). Self-corrects on the very next
	// transition either way; narrow enough not to be worth tracking
	// separately whether this is currently trustworthy.
	highlightSlot int

	// slotHeights holds the last-known real height of each of v.box's
	// top-level slots, in pixels - -1 for "never resolved yet".
	// Refreshed from whatever's currently resolved every time
	// heightEstimate reads it; left untouched (keeping its last real
	// value) for a slot that's been invalidated but not yet
	// re-resolved - Hover's surgical invalidation (see invalidateSlot)
	// never actually changes a slot's height, so the stale value
	// remains correct until it's naturally re-resolved. Reset only
	// where v.box itself is rebuilt from scratch (see rebuild) - a real
	// rebuild (unlike Hover) can change heights.
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

// heightEstimate returns the current best-known-or-estimated height of
// every top-level slot, in pixels - real where resolved (refreshing
// slotHeights as it goes), extrapolated from the average of whatever's
// known otherwise. len(result) == len(v.box.slots). DocumentBounds and
// VisibleViewBounds both derive from this one pass rather than each
// walking the slots separately.
func (v *View) heightEstimate() []float64 {
	if len(v.slotHeights) != len(v.box.slots) {
		v.slotHeights = make([]float64, len(v.box.slots))
		for i := range v.slotHeights {
			v.slotHeights[i] = -1
		}
	}
	var sum float64
	var knownCount int
	for i := range v.box.slots {
		if v.box.slots[i].box != nil {
			v.slotHeights[i] = float64(v.box.slots[i].box.Bounds().Dy())
		}
		if v.slotHeights[i] >= 0 {
			sum += v.slotHeights[i]
			knownCount++
		}
	}
	if knownCount == 0 || knownCount == len(v.slotHeights) {
		return v.slotHeights
	}
	avg := sum / float64(knownCount)
	out := make([]float64, len(v.slotHeights))
	for i, h := range v.slotHeights {
		if h < 0 {
			h = avg
		}
		out[i] = h
	}
	return out
}

// DocumentBounds returns the document's estimated extent, origin at
// (0, 0): width is what Layout was last called with; height is the
// current best estimate of the total document height (see
// heightEstimate) - exact once every slot has been resolved at least
// once, refined automatically before then as more of the document is
// visited. A caller building its own scrollbar (vertical, or - if it
// ever applies - horizontal) scales the ratio between this and
// VisibleViewBounds to whatever real pixel track it's drawing into.
func (v *View) DocumentBounds() image.Rectangle {
	if v.box == nil {
		return image.Rectangle{}
	}
	var total float64
	for _, h := range v.heightEstimate() {
		total += h
	}
	return image.Rect(0, 0, v.boxWidth, int(total))
}

// VisibleViewBounds returns the sub-rectangle of DocumentBounds
// currently visible for a viewport of viewportSize (the same size
// passed to Draw's dst). The top (everything above the cursor) uses
// heightEstimate's estimate, same as DocumentBounds - cheap even after
// a cursor jump (ScrollToAnchor) that skipped resolving everything in
// between. The bottom edge, deliberately, does not: it walks forward
// from the cursor resolving each slot for real (v.box.boxAt), exactly
// the same slots and the same break condition DrawFrom itself uses -
// since that range is what Draw is about to resolve anyway this frame,
// there's no laziness benefit to estimating it instead, and using the
// estimate here made the thumb's size visibly jump as slots crossed
// from "estimated" to "just resolved" with a different-than-average
// height while scrolling.
func (v *View) VisibleViewBounds(viewportSize image.Point) image.Rectangle {
	if v.box == nil || v.cursor.index >= len(v.box.slots) {
		return image.Rectangle{}
	}
	heights := v.heightEstimate()
	var before float64
	for i := 0; i < v.cursor.index; i++ {
		before += heights[i]
	}
	top := before + v.cursor.offset

	bottom := before // running position, starts at the top of cursor.index's own slot
	for i := v.cursor.index; i < len(v.box.slots); i++ {
		if bottom-top > float64(viewportSize.Y) {
			break
		}
		bottom += float64(v.box.boxAt(i).Bounds().Dy())
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
// which top-level slot p.Y resolved to - slot is meaningful only when
// hit is non-nil. HitTest itself discards it; linkNodeAt (and so
// Hover) reuses it instead of re-walking the cursor a second time for
// the same y.
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

// linkNodeAt returns the ASTNode of the link at document position (x, y)
// - the same coordinate space HitTest/Draw use - or nil if (x, y) doesn't
// land on a link. slot is the top-level slot index (x, y) resolved to,
// meaningful only when node is non-nil - Hover reuses it to know which
// slot to invalidate without a second cursor-relative walk; LinkAt just
// discards it. Shared by Hover and LinkAt so neither duplicates the
// hit-then-walk-to-the-enclosing-link logic.
func (v *View) linkNodeAt(x, y int) (node *ASTNode, slot int) {
	hit, _, slot := v.hitTest(x, y)
	if hit == nil {
		return nil, 0
	}
	return hit.Source().Node().AncestorTag(TagLink), slot
}

// Hover updates the currently-highlighted link, given the mouse position
// in the same coordinate space HitTest/Draw use - call every frame from
// the embedding game's own input handling. Finding the link under (x, y)
// is cheap (linkNodeAt), and so is applying a change: highlighting only
// ever changes color (RenderingContext.HighlightNode/ResolvedColor),
// never layout, so only the top-level slot being left and the one being
// entered - at most two - are discarded and lazily rebuilt (see
// invalidateSlot), the same surgical invalidation invalidateChangedImages
// already uses for a settled image. highlightSlot is kept fresh on every
// call, not only when the link actually changes, so it survives an
// unrelated full rebuild (a resize, a StyleSheet swap) that happens while
// something is already highlighted - see highlightSlot's own doc comment
// for the one narrow, purely-cosmetic edge case this doesn't cover.
//
// Hover also reports the link under (x, y), same as LinkAt, so a caller
// handling a click at the same position doesn't need a second HitTest -
// e.g. cmd/whynot calls Hover once per frame with the cursor position
// and can reuse its result if that frame also saw a click.
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
// it, with whatever's current in v.box.ctx, the next time something
// actually asks for it. The same per-slot invalidation
// invalidateChangedImages uses for a settled image.
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
func (v *View) Layout(width int, scale float64, now time.Duration) {
	v.ctx.SetDPI(scale * 72)
	v.ctx.Scale = scale
	v.ctx.Time = now

	if width == v.boxWidth && scale == v.boxScale {
		v.invalidateChangedImages()
		return
	}
	v.boxWidth = width
	v.boxScale = scale
	v.rebuild()
}

// invalidateChangedImages is Layout's response to an unchanged width/
// scale: images load asynchronously and in the background (see
// ImageCache), so even when nothing about the View's own configuration
// changed, an image it's showing a placeholder or fallback for may
// have settled since the last call and need picking up.
//
// A slot that was never resolved (most of a long document - see
// StackBox.boxAt) is left alone: nothing pending was ever reported for
// it, since nobody's asked for it yet. Of the slots that were
// resolved, one whose PendingImages() doesn't intersect what changed -
// fully settled, or waiting on a different, still-unrelated image - is
// also left exactly as it is. Only a slot actually waiting on
// something that changed gets its memoized box discarded, so the very
// next boxAt (imminent, since this is by definition already near the
// viewport) rebuilds just that one.
//
// The one exception is a change with BoundsRevealed: a slot showing
// "(loading image…)" text has no predictable size, so learning the
// image's real size for the first time can change that slot's height -
// unlike every other transition, which happens at an already-known,
// already-laid-out size. That can shift what's currently visible, so
// it goes through the full rebuild (with its existing ratio-based
// scroll re-anchoring) instead of the surgical path.
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
		if c.BoundsRevealed {
			v.rebuild()
			return
		}
	}

	for i := range v.box.slots {
		slot := v.box.slots[i]
		if slot.box == nil {
			continue
		}
		for _, src := range slot.box.PendingImages() {
			if changedSrcs[src] {
				v.invalidateSlot(i)
				break
			}
		}
	}
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

// rebuild re-lays-out the document at the current width, re-anchoring the
// scroll position by ratio through the current slot rather than by raw
// pixel offset, so a change to content heights (from a resize or a
// StyleSheet swap) doesn't change what's visible - and since the cursor
// is already (index, offset), this only needs one old and one new height,
// not a scan of the tree.
//
// The document is wrapped with a leading and trailing EmptyBox sized to
// the StyleSheet's ViewMargins, so the view's outer margin is real (if
// empty) space in the tree - the same way StackBlock.GetBlockLayout
// already represents inter-block gaps - rather than a separate draw-time
// overlay: scrolling clamps past the last block into the bottom margin
// exactly like clamping at the true end of the document, and a click
// inside either margin misses, since EmptyBox.HitTest always declines.
// Left/Right instead narrow the width passed to GetBlockLayout, since
// slots have no per-slot horizontal position the way they have a
// height - Draw/HitTest shift by Left to compensate.
func (v *View) rebuild() {
	v.slotHeights = nil // a real rebuild can change any slot's height

	ratio := 0.0
	if v.box != nil && v.cursor.index < len(v.box.slots) {
		if h := v.box.boxAt(v.cursor.index).Bounds().Dy(); h > 0 {
			ratio = v.cursor.offset / float64(h)
		}
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

	if v.cursor.index >= len(v.box.slots) {
		v.cursor.index = len(v.box.slots) - 1
	}
	newHeight := v.box.boxAt(v.cursor.index).Bounds().Dy()
	v.cursor = v.box.normalizeCursor(stackCursor{index: v.cursor.index, offset: ratio * float64(newHeight)})
}
