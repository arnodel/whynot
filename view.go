package whynot

import (
	"image"
	"strings"
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
	v.box.DrawFrom(dst, v.cursor, x+int(v.ctx.ScaledViewMargins().Left), y)
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
	if v.box == nil {
		return nil, image.Point{}
	}
	left := int(v.ctx.ScaledViewMargins().Left)
	c := v.box.normalizeCursor(stackCursor{index: v.cursor.index, offset: v.cursor.offset + float64(y)})
	if c.index < 0 || c.index >= len(v.box.slots) {
		return nil, image.Point{}
	}
	box := v.box.boxAt(c.index)
	local := image.Pt(x-left, int(c.offset))
	if !local.In(box.Bounds()) {
		return nil, image.Point{}
	}
	hit, offset = box.HitTest(local)
	if hit == nil {
		return nil, image.Point{}
	}
	// Shift back from box's own local frame into the same frame (x, y)
	// arrived in - undoing both the left-margin shift and the
	// cursor-relative adjustment made to y above.
	return hit, offset.Add(image.Pt(left, y-int(c.offset)))
}

// linkNodeAt returns the ASTNode of the link at document position (x, y)
// - the same coordinate space HitTest/Draw use - or nil if (x, y) doesn't
// land on a link. Shared by Hover and LinkAt so neither duplicates the
// HitTest-then-walk-to-the-enclosing-link logic.
func (v *View) linkNodeAt(x, y int) *ASTNode {
	hit, _ := v.HitTest(x, y)
	if hit == nil {
		return nil
	}
	return hit.Source().Node().AncestorTag(TagLink)
}

// Hover updates the currently-highlighted link, given the mouse position
// in the same coordinate space HitTest/Draw use - call every frame from
// the embedding game's own input handling. Finding the link under (x, y)
// is cheap (HitTest), but applying a change isn't: rebuild re-lays-out
// the whole document, currently the simplest way to get the hovered
// link's Source restyled through the exact same StyleSheet-resolution
// path as everything else (see RenderingContext.HighlightNode/
// ResolvedColor) - so this only rebuilds when the link actually changes
// from the previous call.
//
// Hover also reports the link under (x, y), same as LinkAt, so a caller
// handling a click at the same position doesn't need a second HitTest -
// e.g. cmd/whynot calls Hover once per frame with the cursor position
// and can reuse its result if that frame also saw a click.
func (v *View) Hover(x, y int) (destination string, ok bool) {
	node := v.linkNodeAt(x, y)
	if node != v.ctx.HighlightNode {
		v.ctx.HighlightNode = node
		if v.box != nil {
			v.rebuild()
		}
	}
	if node == nil {
		return "", false
	}
	return node.Destination, true
}

// LinkAt reports the destination URL of the link at document position
// (x, y) - the same coordinate space HitTest/Hover use. ok is false if
// (x, y) doesn't land on a link. Unlike Hover, this never changes the
// highlight - use it to query a position other than the current hover
// (e.g. from a test, or a separate input source).
func (v *View) LinkAt(x, y int) (destination string, ok bool) {
	node := v.linkNodeAt(x, y)
	if node == nil {
		return "", false
	}
	return node.Destination, true
}

// Layout sets the pixel width and display scale to render at (DPI = scale
// * 72, matching main.go's convention). Cheap to call every frame: the
// layout tree only rebuilds when width or scale actually change.
func (v *View) Layout(width int, scale float64) {
	v.ctx.SetDPI(scale * 72)
	v.ctx.Scale = scale

	if width == v.boxWidth && scale == v.boxScale {
		return
	}
	v.boxWidth = width
	v.boxScale = scale
	v.rebuild()
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
