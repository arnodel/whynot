package whynot

import (
	"image"
	_ "image/jpeg" // registers the JPEG format with image.DecodeConfig
	"os"
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
	title string
	node  *ASTNode
}

var _ Inline = (*InlineImage)(nil)

func (i *InlineImage) Node() *ASTNode {
	return i.node
}

// GetInlineLayout probes src's dimensions via a cheap header-only read (no
// full decode, no rendering backend involved - reading an image's size
// isn't a backend-specific operation the way loading its pixels for
// drawing is). A missing or unreadable file yields a zero-size box rather
// than failing layout outright.
func (i *InlineImage) GetInlineLayout(ctx RenderingContext) InlineLayout {
	var bounds image.Rectangle
	if f, err := os.Open(i.src); err == nil {
		defer f.Close()
		if cfg, _, err := image.DecodeConfig(f); err == nil {
			bounds = image.Rectangle{Max: image.Pt(cfg.Width, cfg.Height)}
		}
	}
	return &ImageBox{
		src:    i.src,
		bounds: bounds,
		source: i,
	}
}
