package whynot

import (
	"image"
	"image/color"
	"time"
)

// NewDiagramBlock returns a Block that renders img's image once it
// resolves, and fallback's own layout - typically the same CodeBlock
// that would've rendered without a plugin - while it's still pending or
// failed, so a diagram's raw source stays visible rather than a bare
// placeholder message. See CodeBlockPlugin.
func NewDiagramBlock(node *ASTNode, img AsyncImage, fallback Block) Block {
	return &diagramBlock{node: node, imageNode: node.AddChild(TagImage), img: img, fallback: fallback}
}

type diagramBlock struct {
	WithoutMargins
	node *ASTNode
	// imageNode is a TagImage child of node, precomputed once here
	// rather than on demand inside GetBlockLayout (which can run many
	// times - a resize, a theme change - and ASTNode.AddChild isn't
	// idempotent) - mirrors InlineImage's identical fallbackNode field.
	// Its only job is giving the ready image's frame a StyleSheet.
	// BorderColor to resolve (TagCodeBlock, node's own tag, has none).
	imageNode *ASTNode
	img       AsyncImage
	fallback  Block
}

var _ Block = (*diagramBlock)(nil)

func (b *diagramBlock) Node() *ASTNode {
	return b.node
}

func (b *diagramBlock) GetBlockLayout(ctx RenderingContext, width int) BlockLayout {
	if ctx.ImageCache == nil {
		return b.fallback.GetBlockLayout(ctx, width)
	}
	result := ctx.ImageCache.LoadImage(b.img)
	if result.Status != ImageReady {
		// Pending or Failed: show the fallback (raw/highlighted code)
		// instead - but still report the diagram's own key as pending,
		// so View.invalidateChangedImages revisits this slot once the
		// fetch resolves (Pending -> Ready) or retries (Failed -> a
		// later Pending/Ready, per ImageCache's own retry timer), even
		// though the fallback layout itself knows nothing about it.
		return &diagramBox{inner: b.fallback.GetBlockLayout(ctx, width), pendingKey: b.img.Key}
	}

	frameThickness := int(ctx.ScaledThematicBreakThickness(b.imageNode))
	inset := frameThickness + int(diagramPadding(ctx.StyleSheet, b.imageNode)*ctx.Scale)
	availableWidth := width - 2*inset
	if availableWidth < 0 {
		availableWidth = 0
	}
	imgBounds := fitWidth(scaleRect(result.Bounds, ctx.Scale), availableWidth)
	return &diagramBox{inner: &imageLayout{
		img:            result.Image,
		anim:           result.Animation,
		bounds:         image.Rect(0, 0, imgBounds.Dx()+2*inset, imgBounds.Dy()+2*inset),
		inset:          inset,
		frameThickness: frameThickness,
		frameColor:     ctx.StyleSheet.BorderColor(b.imageNode),
		source:         b,
	}}
}

// DiagramStyleSheet is an optional StyleSheet capability for
// customizing a rendered diagram's own "matting" (see NewDiagramBlock) -
// a StyleSheet that doesn't implement it gets fallbackDiagramPadding
// instead (DefaultStyleSheet does implement it, so both of whynot's own
// built-in themes are already customizable this way, including via the
// usual embed-and-override-one-method pattern documented on
// DefaultStyleSheet's own dimensional-constant fields).
//
// This - a small capability interface specific to one Block/BlockLayout
// pair, checked with a type assertion, rather than a method on the core
// StyleSheet interface itself - is the general pattern for any future
// CodeBlockPlugin (or other extension) that needs its own styling knob:
// StyleSheet stays fixed, so every existing implementation keeps
// compiling unmodified, and only a StyleSheet that actually wants to
// customize that one extension's own concern needs to implement its
// (equally small) interface.
type DiagramStyleSheet interface {
	// DiagramPadding is the "matting" between a diagram's frame and its
	// own content, in logical (unscaled) pixels - without it, the frame
	// hugs the diagram's own edges too tightly, which often come right
	// up to its bounding box.
	DiagramPadding(node *ASTNode) float64
}

// fallbackDiagramPadding is diagramPadding's answer for a StyleSheet
// that doesn't implement DiagramStyleSheet at all (some pre-existing
// custom implementation, most likely).
const fallbackDiagramPadding = 12

// diagramPadding resolves DiagramStyleSheet if styleSheet implements it,
// falling back to fallbackDiagramPadding otherwise.
func diagramPadding(styleSheet StyleSheet, node *ASTNode) float64 {
	if ds, ok := styleSheet.(DiagramStyleSheet); ok {
		return ds.DiagramPadding(node)
	}
	return fallbackDiagramPadding
}

// diagramBox is a thin delegating BlockLayout: Bounds/Source/
// drawContents/HitTest all forward to inner - either the fallback's own
// layout (pending/failed) or a fresh imageLayout (ready). PendingImages
// is the one place behavior differs from a plain pass-through, appending
// pendingKey when set - what keeps the async redraw-when-ready wiring
// working even though inner itself may know nothing about it.
type diagramBox struct {
	inner      BlockLayout
	pendingKey string
}

var _ BlockLayout = (*diagramBox)(nil)

func (b *diagramBox) Bounds() image.Rectangle {
	return b.inner.Bounds()
}

func (b *diagramBox) Source() Source {
	return b.inner.Source()
}

func (b *diagramBox) drawContents(dst Canvas, x, y int, now time.Duration) {
	b.inner.drawContents(dst, x, y, now)
}

func (b *diagramBox) HitTest(p image.Point) (Hit, image.Point) {
	return b.inner.HitTest(p)
}

func (b *diagramBox) PendingImages() []string {
	if b.pendingKey == "" {
		return b.inner.PendingImages()
	}
	return append(b.inner.PendingImages(), b.pendingKey)
}

// imageLayout is the block-level equivalent of inline.go's ImageBox,
// for a diagramBox's ready state - a resolved diagram just draws like
// any other image once decoded, except for one deliberate difference:
// it always draws on an opaque white backdrop, framed the same way
// TableBox's own border is (frameThickness/frameColor, both from
// ctx.StyleSheet). A rendered diagram's own colors (arrow strokes, thin
// lines, text) are chosen by whatever tool produced it assuming a light
// backdrop, and Kroki's own output is transparent where nothing is
// drawn - composited directly over the page's own background, a dark
// theme can make thin/dark strokes disappear entirely. Forcing white
// sidesteps that regardless of the diagram's own palette (which whynot
// has no way to know or safely re-theme, e.g. by inverting colors -
// that would just as easily corrupt a diagram that legitimately uses
// non-grayscale colors); the frame keeps a white rectangle from looking
// like a stray hole in the page on a dark background. diagramPadding
// insets the image from the frame itself, so content that runs right up
// to the diagram's own edges (common - most diagrams don't leave their
// own margin) doesn't look cramped against it.
//
// HitTest always declines, like RuleBox/EmptyBox: a diagram isn't
// interactive.
type imageLayout struct {
	img    image.Image
	anim   *AnimatedImage
	bounds image.Rectangle // outer bounds - the frame and diagramPadding "matting" both live inside this, not added on top of it
	// inset is frameThickness plus the scaled diagramPadding - the
	// distance from bounds' own edge in to where the image itself
	// starts.
	inset          int
	frameThickness int
	frameColor     color.Color
	source         Source
}

var _ BlockLayout = (*imageLayout)(nil)

func (b *imageLayout) Bounds() image.Rectangle {
	return b.bounds
}

func (b *imageLayout) Source() Source {
	return b.source
}

func (b *imageLayout) drawContents(dst Canvas, x, y int, now time.Duration) {
	w, h := b.bounds.Dx(), b.bounds.Dy()
	dst.DrawRect(x, y, w, h, color.White)
	imgX, imgY := x+b.inset, y+b.inset
	imgW, imgH := w-2*b.inset, h-2*b.inset
	if b.anim != nil {
		dst.DrawImage(b.anim.CurrentFrame(now), imgX, imgY, imgW, imgH)
	} else {
		dst.DrawImage(b.img, imgX, imgY, imgW, imgH)
	}
	t := b.frameThickness
	dst.DrawRect(x, y, w, t, b.frameColor)     // top
	dst.DrawRect(x, y+h-t, w, t, b.frameColor) // bottom
	dst.DrawRect(x, y, t, h, b.frameColor)     // left
	dst.DrawRect(x+w-t, y, t, h, b.frameColor) // right
}

func (b *imageLayout) HitTest(p image.Point) (Hit, image.Point) {
	return nil, image.Point{}
}

func (b *imageLayout) PendingImages() []string {
	return nil
}
