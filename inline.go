package whynot

import (
	"fmt"
	"image"
)

type Inline interface {
	Source
	// GetInlineLayout builds this Inline's layout, width the same
	// effectively-unbounded-or-real wrap width a Block's own
	// GetBlockLayout receives (see naturalWidthMeasure) - only
	// InlineImage uses it (see fitWidth), every other implementation
	// ignores it.
	GetInlineLayout(ctx RenderingContext, width int) InlineLayout
}

type InlineText struct {
	text string
	node *ASTNode

	// glued - see InlineLayout.Glued's doc comment. Set by the compiler
	// (MarkdownCompiler.appendString/pendingSpace) from whether the
	// source actually had whitespace immediately before this item;
	// false (the zero value) for every InlineText built directly rather
	// than through the compiler, matching the old always-space behavior.
	glued bool
}

var _ Inline = (*InlineText)(nil)

func (t *InlineText) Node() *ASTNode {
	return t.node
}

func (t *InlineText) GetInlineLayout(ctx RenderingContext, width int) InlineLayout {
	face, err := ctx.SelectFace(ctx.ResolvedTextStyle(t.node))
	if err != nil {
		panic(err)
	}
	return &TextBox{
		Text:            t.text,
		Face:            face,
		Color:           ctx.ResolvedColor(t.node),
		StrikeThickness: int(ctx.ScaledStrikeThickness(t.node)),
		LineHeight:      ctx.StyleSheet.LineHeight(t.node),
		glued:           t.glued,
		source:          t,
	}
}

// checkboxUnchecked/checkboxChecked are the semantically-correct ballot-box
// glyphs for a task list item's marker (☐/☑) - used when the active face
// actually has them (see TaskCheckbox.GetInlineLayout). Most fonts that
// have either have both, since they're adjacent in the same Unicode block.
const (
	checkboxUnchecked = '☐'
	checkboxChecked   = '☑'
)

// TaskCheckbox is a GFM task list item's checkbox marker (- [ ]/- [x]).
// Resolved at layout time, not compile time: whether to draw the real
// ballot-box glyph or a procedurally-drawn box (see CheckboxBox) depends
// on whether the active FaceSelector/StyleSheet's resolved font actually
// has that glyph, which can change across a re-layout (zoom, theme, a
// caller registering a different font) even though the semantic tree
// itself never does - most fonts, including the bundled Go fonts, don't
// have it.
type TaskCheckbox struct {
	checked bool
	node    *ASTNode
}

var _ Inline = (*TaskCheckbox)(nil)

func (c *TaskCheckbox) Node() *ASTNode {
	return c.node
}

func (c *TaskCheckbox) GetInlineLayout(ctx RenderingContext, width int) InlineLayout {
	face, err := ctx.SelectFace(ctx.ResolvedTextStyle(c.node))
	if err != nil {
		panic(err)
	}
	r := rune(checkboxUnchecked)
	if c.checked {
		r = checkboxChecked
	}
	if _, ok := face.GlyphAdvance(r); ok {
		return &TextBox{
			Text:       string(r),
			Face:       face,
			Color:      ctx.ResolvedColor(c.node),
			LineHeight: ctx.StyleSheet.LineHeight(c.node),
			source:     c,
		}
	}
	return newCheckboxBox(c.checked, face, ctx.ResolvedColor(c.node), c)
}

type InlineImage struct {
	src   string
	alt   string
	title string
	node  *ASTNode
	// fallbackNode is a TagUnsupported child of node - see the compile.go
	// KindImage case for why it's precomputed once, at parse time,
	// rather than created on demand here.
	fallbackNode *ASTNode

	// glued - see InlineLayout.Glued's doc comment and InlineText.glued.
	glued bool

	// ownCache is lazily created the first time GetInlineLayout runs
	// with no ImageCache configured (a bare RenderingContext{}, as most
	// of this package's own tests use) and reused on every later call -
	// loading is asynchronous, so a fresh cache created on every call
	// would restart the fetch from scratch each time and could never be
	// observed settling.
	ownCache *ImageCache
}

var _ Inline = (*InlineImage)(nil)

func (i *InlineImage) Node() *ASTNode {
	return i.node
}

// GetInlineLayout resolves, fetches, and decodes src via ctx.ImageCache
// (letting the embedder decide the resolution/fetch policy - relative
// to a document's location, over http(s), from an archive, whatever it
// needs; falls back to i.ownCache, lazily created from
// FileImageSource, if ctx was built as a bare RenderingContext{} with
// no ImageCache set, the same default NewView itself uses - so direct
// RenderingContext callers that never touch an image, like most of
// this package's own tests, don't need to know ImageCache exists) -
// never blocking, since ImageCache.Load never does. There are three
// outcomes:
//
//   - Ready: the decoded image, scaled by ctx.Scale like every other
//     sized quantity in the layout system (RenderingContext.ScaledMargins
//     and friends), so images grow and shrink along with zoom/DPI
//     instead of staying pixel-locked - then capped to width, preserving
//     aspect ratio, if that scaled size would still be wider (see
//     fitWidth) - the inline layer's equivalent of CSS's max-width: 100%,
//     so an image dropped into a document at its own native resolution
//     doesn't overflow the page.
//   - Pending with bounds already known (the header-peek in
//     ImageCache.fetchAndDecode beat the caller here): an ImageBox at
//     the correct, final (scaled and width-capped) size, but with no
//     image yet - DrawInline draws a placeholder instead, and nothing
//     needs to reflow once the real pixels arrive later.
//   - Pending with no bounds yet, or Failed (unreadable, undecodable, or
//     mid-retry-cooldown): falls back to text instead of a silent
//     zero-size gap - reusing InlineText's own GetInlineLayout, so it
//     renders exactly like any other construct whynot can't handle (see
//     appendUnsupportedInline). Either way the resulting TextBox is
//     stamped with which src it's standing in for, so
//     View.invalidateChangedImages knows to revisit it once that
//     changes.
func (i *InlineImage) GetInlineLayout(ctx RenderingContext, width int) InlineLayout {
	cache := ctx.ImageCache
	if cache == nil {
		if i.ownCache == nil {
			i.ownCache = NewImageCache(FileImageSource{})
		}
		cache = i.ownCache
	}
	resolved, result := cache.Load(i.src)
	switch result.Status {
	case ImageReady:
		return &ImageBox{
			img:    result.Image,
			anim:   result.Animation,
			bounds: fitWidth(scaleRect(result.Bounds, ctx.Scale), width),
			glued:  i.glued,
			source: i,
		}
	case ImagePending:
		if result.Bounds != (image.Rectangle{}) {
			return &ImageBox{
				bounds:           fitWidth(scaleRect(result.Bounds, ctx.Scale), width),
				placeholderColor: ctx.StyleSheet.BorderColor(i.node),
				pending:          []string{resolved},
				glued:            i.glued,
				source:           i,
			}
		}
		box := (&InlineText{text: "(loading image…)", node: i.node, glued: i.glued}).GetInlineLayout(ctx, width).(*TextBox)
		box.pending = []string{resolved}
		return box
	default: // ImageFailed
		fallback := i.fallback(resolved)
		fallback.glued = i.glued
		box := fallback.GetInlineLayout(ctx, width).(*TextBox)
		box.pending = []string{resolved}
		return box
	}
}

// fallback is what's shown in place of an image that's missing,
// unreadable, or undecodable - alt text if the Markdown gave any, else
// title, else a generic message naming resolved (the image's own
// destination if src couldn't even be resolved).
func (i *InlineImage) fallback(resolved string) *InlineText {
	text := i.alt
	if text == "" {
		text = i.title
	}
	if text == "" {
		text = fmt.Sprintf("(image not found: %s)", resolved)
	}
	return &InlineText{text: text, node: i.fallbackNode}
}

// scaleRect scales r (typically an image's native pixel bounds) by
// scale, the way every other sized quantity in the layout system is
// scaled (RenderingContext.ScaledMargins and friends).
func scaleRect(r image.Rectangle, scale float64) image.Rectangle {
	return image.Rectangle{Max: image.Pt(
		int(float64(r.Dx())*scale),
		int(float64(r.Dy())*scale),
	)}
}

// fitWidth scales r down, preserving aspect ratio, so its width never
// exceeds width - CSS's max-width: 100%, applied to an inline image.
// width <= 0 means unbounded (e.g. TableBlock's natural-width
// measurement pass, naturalWidthMeasure); r already fitting is the same
// case either way, returned unchanged. Never scales up - a small image
// stays its own size regardless of how much room is available.
func fitWidth(r image.Rectangle, width int) image.Rectangle {
	if width <= 0 || r.Dx() <= width {
		return r
	}
	return image.Rect(0, 0, width, r.Dy()*width/r.Dx())
}
