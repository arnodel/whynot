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
}

var _ Inline = (*InlineImage)(nil)

func (i *InlineImage) Node() *ASTNode {
	return i.node
}

// GetInlineLayout resolves, fetches, and decodes src via ctx.ImageCache
// (letting the embedder decide the resolution/fetch policy - relative
// to a document's location, over http(s), from an archive, whatever it
// needs; falls back to a fresh NewImageCache(FileImageSource{}) if ctx
// was built as a bare RenderingContext{} with no ImageCache set, the
// same default NewView itself uses - so direct RenderingContext
// callers that never touch an image, like most of this package's own
// tests, don't need to know ImageCache exists), then scales its
// decoded bounds by ctx.Scale like every other sized quantity in the
// layout system (RenderingContext.ScaledMargins and friends) so images
// grow and shrink along with zoom/DPI instead of staying pixel-locked.
//
// A missing, unreadable, or undecodable image falls back to fallback's
// text instead of a silent zero-size gap - reusing InlineText's own
// GetInlineLayout, so it renders exactly like any other construct
// whynot can't handle (see appendUnsupportedInline): flagged in
// StyleSheet.UnsupportedColor, not silently missing.
func (i *InlineImage) GetInlineLayout(ctx RenderingContext) InlineLayout {
	cache := ctx.ImageCache
	if cache == nil {
		cache = NewImageCache(FileImageSource{})
	}
	resolved, img, err := cache.Load(i.src)
	if err == nil {
		bounds := image.Rectangle{Max: image.Pt(
			int(float64(img.Bounds().Dx())*ctx.Scale),
			int(float64(img.Bounds().Dy())*ctx.Scale),
		)}
		return &ImageBox{img: img, bounds: bounds, source: i}
	}
	return i.fallback(resolved).GetInlineLayout(ctx)
}

// fallback is what's shown in place of an image GetInlineLayout
// couldn't open or decode - alt text if the Markdown gave any, else
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
