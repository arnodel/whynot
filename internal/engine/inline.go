package engine

import (
	"fmt"
	"image"

	"github.com/arnodel/whynot/fetch"
	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/imagecache"
)

type Inline interface {
	Source
	// GetInlineLayout builds this Inline's layout, width the same
	// effectively-unbounded-or-real wrap width a Block's own
	// GetBlockLayout receives (see naturalWidthMeasure) - only
	// InlineImage uses it (see fitWidth), every other implementation
	// ignores it.
	GetInlineLayout(ctx Context, width int) InlineLayout
}

type InlineText struct {
	Text    string
	ASTNode *ast.Node

	// Glued - see InlineLayout.Glued's doc comment. Set by the compiler
	// (compiler.appendString/pendingSpace) from whether the
	// source actually had whitespace immediately before this item;
	// false (the zero value) for every InlineText built directly rather
	// than through the compiler, matching the old always-space behavior.
	Glued bool
}

var _ Inline = (*InlineText)(nil)

func (t *InlineText) Node() *ast.Node {
	return t.ASTNode
}

func (t *InlineText) GetInlineLayout(ctx Context, width int) InlineLayout {
	face := ctx.selectFace(ctx.ResolvedTextStyle(t.ASTNode))
	return &TextBox{
		Text:            t.Text,
		Face:            face,
		Color:           ctx.ResolvedColor(t.ASTNode),
		StrikeThickness: int(ctx.scaledStrikeThickness(t.ASTNode)),
		LineHeight:      ctx.Styles.LineHeight(t.ASTNode),
		glued:           t.Glued,
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
// ballot-box glyph or a procedurally-drawn box (see checkboxBox) depends
// on whether the active FaceSelector/StyleSheet's resolved font actually
// has that glyph, which can change across a re-layout (zoom, theme, a
// caller registering a different font) even though the semantic tree
// itself never does - most fonts, including the bundled Go fonts, don't
// have it.
type TaskCheckbox struct {
	Checked bool
	ASTNode *ast.Node
}

var _ Inline = (*TaskCheckbox)(nil)

func (c *TaskCheckbox) Node() *ast.Node {
	return c.ASTNode
}

func (c *TaskCheckbox) GetInlineLayout(ctx Context, width int) InlineLayout {
	face := ctx.selectFace(ctx.ResolvedTextStyle(c.ASTNode))
	r := rune(checkboxUnchecked)
	if c.Checked {
		r = checkboxChecked
	}
	if _, ok := face.GlyphAdvance(r); ok {
		return &TextBox{
			Text:       string(r),
			Face:       face,
			Color:      ctx.ResolvedColor(c.ASTNode),
			LineHeight: ctx.Styles.LineHeight(c.ASTNode),
			source:     c,
		}
	}
	return newCheckboxBox(c.Checked, face, ctx.ResolvedColor(c.ASTNode), c)
}

type InlineImage struct {
	Src string // as written in the Markdown
	// Image is where the image comes from, resolved from Src at parse
	// time, or nil if that failed, with ImageErr saying why.
	Image    fetch.Source
	ImageErr error
	Alt      string
	Title    string
	ASTNode  *ast.Node
	// FallbackNode is a ast.TagUnsupported child of node - see the
	// compiler's KindImage case (internal/markdown/inlines.go) for why
	// it's precomputed once, at parse time, rather than created on demand
	// here.
	FallbackNode *ast.Node

	// Glued - see InlineLayout.Glued's doc comment and InlineText.Glued.
	Glued bool
}

var _ Inline = (*InlineImage)(nil)

func (i *InlineImage) Node() *ast.Node {
	return i.ASTNode
}

// GetInlineLayout fetches and decodes Image via ctx.ImageCache - never
// blocking, since imagecache.Cache.Load never does. If Src couldn't be
// resolved, or there's no image cache, the image isn't loaded and its
// fallback text is shown. Otherwise there are three outcomes:
//
//   - Ready: the decoded image, scaled by ctx.Scale like every other
//     sized quantity in the layout system (Context.ScaledMargins
//     and friends), so images grow and shrink along with zoom/DPI
//     instead of staying pixel-locked - then capped to width, preserving
//     aspect ratio, if that scaled size would still be wider (see
//     fitWidth) - the inline layer's equivalent of CSS's max-width: 100%,
//     so an image dropped into a document at its own native resolution
//     doesn't overflow the page.
//   - Pending with bounds already known (the header-peek in
//     imagecache.Cache.fetchAndDecode beat the caller here): an ImageBox at
//     the correct, final (scaled and width-capped) size, but with no
//     image yet - DrawInline draws a placeholder instead, and nothing
//     needs to reflow once the real pixels arrive later.
//   - Pending with no bounds yet, or Failed (unreadable, undecodable, or
//     mid-retry-cooldown): falls back to text instead of a silent
//     zero-size gap - reusing InlineText's own GetInlineLayout, so it
//     renders exactly like any other construct whynot can't handle (see
//     appendUnsupportedInline). Either way the resulting TextBox is
//     stamped with the key of the image it's standing in for, so
//     View.invalidateChangedImages knows to revisit it once that
//     changes.
func (i *InlineImage) GetInlineLayout(ctx Context, width int) InlineLayout {
	if i.Image == nil {
		return i.fallback(fmt.Sprintf("(image not loaded: %v)", i.ImageErr)).GetInlineLayout(ctx, width)
	}
	cache := ctx.ImageCache
	if cache == nil {
		return i.fallback(fmt.Sprintf("(image not loaded: %s)", i.Src)).GetInlineLayout(ctx, width)
	}
	key := i.Image.Key()
	result := cache.Load(i.Image)
	switch result.Status {
	case imagecache.Ready:
		return &ImageBox{
			Image:     result.Image,
			Animation: result.Animation,
			Rect:      fitWidth(scaleRect(result.Bounds, ctx.Scale), width),
			glued:     i.Glued,
			source:    i,
		}
	case imagecache.Pending:
		if result.Bounds != (image.Rectangle{}) {
			return &ImageBox{
				Rect:             fitWidth(scaleRect(result.Bounds, ctx.Scale), width),
				placeholderColor: ctx.Styles.BorderColor(i.ASTNode),
				Pending:          []string{key},
				glued:            i.Glued,
				source:           i,
			}
		}
		box := (&InlineText{Text: "(loading image…)", ASTNode: i.ASTNode, Glued: i.Glued}).GetInlineLayout(ctx, width).(*TextBox)
		box.pending = []string{key}
		return box
	default: // imagecache.Failed
		box := i.fallback(fmt.Sprintf("(image not found: %s)", i.Src)).GetInlineLayout(ctx, width).(*TextBox)
		box.pending = []string{key}
		return box
	}
}

// fallback is what's shown in place of an image that isn't displayed -
// alt text if the Markdown gave any, else title, else message.
func (i *InlineImage) fallback(message string) *InlineText {
	text := i.Alt
	if text == "" {
		text = i.Title
	}
	if text == "" {
		text = message
	}
	return &InlineText{Text: text, ASTNode: i.FallbackNode, Glued: i.Glued}
}

// scaleRect scales r (typically an image's native pixel bounds) by
// scale, the way every other sized quantity in the layout system is
// scaled (Context.ScaledMargins and friends).
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
