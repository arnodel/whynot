package whynot

import (
	"image"
	_ "image/jpeg" // registers the JPEG format with image.DecodeConfig
	"os"
)

type RenderingContext struct {
	Scale float64
	FaceSelector
}

func (c RenderingContext) ScaleMargins(m Margins) Margins {
	m.Left *= c.Scale
	m.Right *= c.Scale
	m.Top *= c.Scale
	m.Bottom *= c.Scale
	return m
}

func (t *InlineText) GetInlineBox(ctx RenderingContext) InlineBox {
	face, err := ctx.SelectFace(t.style)
	if err != nil {
		panic(err)
	}
	return &TextBox{
		Text:  t.text,
		Face:  face,
		Color: t.color,
	}
}

// GetInlineBox probes src's dimensions via a cheap header-only read (no
// full decode, no rendering backend involved - reading an image's size
// isn't a backend-specific operation the way loading its pixels for
// drawing is). A missing or unreadable file yields a zero-size box rather
// than failing layout outright.
func (i *InlineImage) GetInlineBox(ctx RenderingContext) InlineBox {
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
	}
}

func (b *CodeBlock) GetBounds(ctx RenderingContext, width int) image.Rectangle {
	height := 0
	for _, line := range b.lines {
		box := line.GetInlineBox(ctx)
		bounds, _ := box.BoundsAndAdvance()
		height += bounds.Dy()
	}
	return image.Rect(0, 0, width, height)
}

func (b *CodeBlock) GetBox(ctx RenderingContext, width int) Box {
	lineBoxes := make([]Box, len(b.lines))
	for i, line := range b.lines {
		lineBoxes[i] = &LineBox{parts: []InlineBox{line.GetInlineBox(ctx)}, space: b.space}
	}
	return &StackBox{boxes: lineBoxes}
}

func (b *TextBlock) GetBounds(ctx RenderingContext, width int) image.Rectangle {
	height := 0
	boxes := make([]InlineBox, len(b.parts))
	for i, part := range b.parts {
		boxes[i] = part.GetInlineBox(ctx)
	}
	for len(boxes) > 0 {
		i, lineBounds := splitBoxes(boxes, width)
		height += lineBounds.Dy()
		boxes = boxes[i:]
	}
	return image.Rect(0, 0, width, height)
}

func (b *TextBlock) GetBox(ctx RenderingContext, width int) Box {
	lines := []Box{}
	boxes := make([]InlineBox, len(b.parts))
	for i, part := range b.parts {
		boxes[i] = part.GetInlineBox(ctx)
	}
	for len(boxes) > 0 {
		i, _ := splitBoxes(boxes, width)
		lines = append(lines, &LineBox{parts: boxes[:i], space: b.space})
		boxes = boxes[i:]
	}
	return &StackBox{boxes: lines}
}

func (b *ListItemBlock) GetBounds(ctx RenderingContext, width int) image.Rectangle {
	height := 0
	boxes := make([]InlineBox, len(b.parts))
	for i, part := range b.parts {
		boxes[i] = part.GetInlineBox(ctx)
	}
	for len(boxes) > 0 {
		i, lineBounds := splitBoxes(boxes, width)
		height += lineBounds.Dy()
		boxes = boxes[i:]
	}
	return image.Rect(0, 0, width, height)
}

func (b *ListItemBlock) GetBox(ctx RenderingContext, width int) Box {
	lines := []Box{}
	boxes := make([]InlineBox, len(b.parts)+1)

	boxes[0] = &ListItemMarkerBox{
		Marker: b.marker.GetInlineBox(ctx),
	}
	for i, part := range b.parts {
		boxes[i+1] = part.GetInlineBox(ctx)
	}
	for len(boxes) > 0 {
		i, _ := splitBoxes(boxes, width)
		lines = append(lines, &LineBox{parts: boxes[:i], space: b.space})
		boxes = boxes[i:]
	}
	return &StackBox{boxes: lines}
}

func (b *StackBlock) GetBounds(ctx RenderingContext, width int) image.Rectangle {
	return image.Rectangle{}
}

func (b *StackBlock) GetBox(ctx RenderingContext, width int) Box {
	boxes := make([]Box, 0, len(b.blocks))
	bottomMargin := 0
	for i, block := range b.blocks {
		margins := ctx.ScaleMargins(block.Margins())
		if i > 0 {
			gap := maxInt(bottomMargin, int(margins.Top))
			if gap > 0 {
				boxes = append(boxes, NewEmptyBox(width, gap))
			}
		}
		if margins.Left > 0 || margins.Right > 0 {
			box := block.GetBox(ctx, width-int(margins.Left+margins.Right))
			boxes = append(boxes, NewContainerBox(box, width, box.Bounds().Dy(), int(margins.Left), 0))
		} else {
			boxes = append(boxes, block.GetBox(ctx, width))
		}
		bottomMargin = int(margins.Bottom)
	}
	return &StackBox{boxes: boxes}
}

func splitBoxes(boxes []InlineBox, width int) (int, image.Rectangle) {
	if len(boxes) == 0 {
		return 0, image.Rectangle{}
	}
	bounds, advance := boxes[0].BoundsAndAdvance()
	left := bounds.Min.X
	if left < 0 {
		bounds = bounds.Add(image.Pt(-left, 0))
		advance -= left
	}
	prevSpace := boxes[0].SpaceWidth()
	for i, box := range boxes[1:] {
		boxBounds, boxAdvance := box.BoundsAndAdvance()

		space := box.SpaceWidth()
		advance += maxInt(space, prevSpace)
		prevSpace = space

		movedBoxBounds := boxBounds.Add(image.Pt(advance, 0))
		bounds = bounds.Union(movedBoxBounds)
		if bounds.Max.X > width {
			return i + 1, bounds
		}
		advance += boxAdvance
	}
	return len(boxes), bounds
}
