package whynot

import (
	"fmt"
	"image"
)

type Inline interface {
	Source
	GetInlineLayout(RenderingContext) InlineLayout
}

type InlineText struct {
	text string
	node *ASTNode
}

var _ Inline = (*InlineText)(nil)

func (t *InlineText) Node() *ASTNode {
	return t.node
}

func (t *InlineText) GetInlineLayout(ctx RenderingContext) InlineLayout {
	face, err := ctx.SelectFace(ctx.ResolvedTextStyle(t.node))
	if err != nil {
		panic(err)
	}
	return &TextBox{
		Text:            t.text,
		Face:            face,
		Color:           ctx.ResolvedColor(t.node),
		StrikeThickness: int(ctx.ScaledStrikeThickness(t.node)),
		source:          t,
	}
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
//     instead of staying pixel-locked.
//   - Pending with bounds already known (the header-peek in
//     ImageCache.fetchAndDecode beat the caller here): an ImageBox at
//     the correct, final (scaled) size, but with no image yet -
//     DrawInline draws a placeholder instead, and nothing needs to
//     reflow once the real pixels arrive later.
//   - Pending with no bounds yet, or Failed (unreadable, undecodable, or
//     mid-retry-cooldown): falls back to text instead of a silent
//     zero-size gap - reusing InlineText's own GetInlineLayout, so it
//     renders exactly like any other construct whynot can't handle (see
//     appendUnsupportedInline). Either way the resulting TextBox is
//     stamped with which src it's standing in for, so
//     View.invalidateChangedImages knows to revisit it once that
//     changes.
func (i *InlineImage) GetInlineLayout(ctx RenderingContext) InlineLayout {
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
			bounds: scaleRect(result.Bounds, ctx.Scale),
			source: i,
		}
	case ImagePending:
		if result.Bounds != (image.Rectangle{}) {
			return &ImageBox{
				bounds:           scaleRect(result.Bounds, ctx.Scale),
				placeholderColor: ctx.StyleSheet.BorderColor(i.node),
				pending:          []string{resolved},
				source:           i,
			}
		}
		box := (&InlineText{text: "(loading image…)", node: i.node}).GetInlineLayout(ctx).(*TextBox)
		box.pending = []string{resolved}
		return box
	default: // ImageFailed
		box := i.fallback(resolved).GetInlineLayout(ctx).(*TextBox)
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
