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

// strikeThickness is the strikethrough line's thickness, in unscaled
// pixels - scaled by ctx.Scale like Margins, so it stays proportionate at
// higher DPI.
const strikeThickness = 1

func (t *InlineText) GetInlineBox(ctx RenderingContext) InlineBox {
	face, err := ctx.SelectFace(t.style)
	if err != nil {
		panic(err)
	}
	box := &TextBox{
		Text:  t.text,
		Face:  face,
		Color: t.color,
	}
	if t.strike {
		box.StrikeThickness = int(strikeThickness * ctx.Scale)
	}
	return box
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

// thematicBreakThickness is the rule's height, in unscaled pixels - scaled
// by ctx.Scale the same way Margins are, so it stays proportionate at
// higher DPI.
const thematicBreakThickness = 2

func (b *ThematicBreakBlock) GetBox(ctx RenderingContext, width int) Box {
	return &RuleBox{
		width:     width,
		thickness: int(thematicBreakThickness * ctx.Scale),
		color:     b.color,
	}
}

// blockquoteIndent is how far a blockquote's content sits to the right of
// its bar, and blockquoteBarWidth is the bar's own width - both in
// unscaled pixels, scaled by ctx.Scale like Margins.
const (
	blockquoteIndent   = 16
	blockquoteBarWidth = 3
)

func (b *BlockquoteBlock) GetBox(ctx RenderingContext, width int) Box {
	indent := int(blockquoteIndent * ctx.Scale)
	return &BlockquoteBox{
		width:    width,
		indent:   indent,
		barWidth: int(blockquoteBarWidth * ctx.Scale),
		barColor: b.barColor,
		inner:    b.inner.GetBox(ctx, width-indent),
	}
}

func (b *CodeBlock) GetBox(ctx RenderingContext, width int) Box {
	lineBoxes := make([]Box, len(b.lines))
	for i, line := range b.lines {
		lineBoxes[i] = &LineBox{parts: []InlineBox{line.GetInlineBox(ctx)}, space: b.space}
	}
	return &StackBox{slots: preResolvedSlots(lineBoxes)}
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
	return &StackBox{slots: preResolvedSlots(lines)}
}

func (b *ListItemHeadBlock) GetBox(ctx RenderingContext, width int) Box {
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
	return &StackBox{slots: preResolvedSlots(lines)}
}

// tableFrameThickness is the frame/header-rule thickness; tableColumnGap
// and tableRowGap are the space between columns and between ordinary
// rows; tableHeaderGap is the (smaller) space between the header row and
// the rule under it - see TableBox for why that makes rowOffsets[1]
// exactly the rule's position, with no separate field needed for it. All
// in unscaled pixels, scaled by ctx.Scale like Margins.
const (
	tableFrameThickness = 2
	tableColumnGap      = 12
	tableRowGap         = 6
	tableHeaderGap      = 4
)

// GetBox lays out columns at equal width (width/numCols) - a starting
// point, not the real column-width negotiation (natural width per
// column, shrunk to fit if it overflows) planned next.
func (b *TableBlock) GetBox(ctx RenderingContext, width int) Box {
	frameThickness := int(tableFrameThickness * ctx.Scale)
	columnGap := int(tableColumnGap * ctx.Scale)
	rowGap := int(tableRowGap * ctx.Scale)
	headerGap := int(tableHeaderGap * ctx.Scale)

	// numCols+1 gaps total: one on each side of the frame, plus one
	// between each pair of adjacent columns - same gap value reused
	// symmetrically rather than a separate border-padding constant.
	numCols := len(b.header)
	innerWidth := width - 2*frameThickness - (numCols+1)*columnGap
	columnWidth := innerWidth / numCols

	columnOffsets := make([]int, numCols+1)
	columnOffsets[0] = frameThickness + columnGap
	for c := range numCols {
		columnOffsets[c+1] = columnOffsets[c] + columnWidth + columnGap
		if c == numCols-1 {
			columnOffsets[c+1] += frameThickness
		}
	}

	rows := append([][]tableCell{b.header}, b.rows...)
	cells := make([][]Box, len(rows))
	rowHeights := make([]int, len(rows))
	for r, row := range rows {
		rowCells := make([]Box, numCols)
		rowHeight := 0
		for c, cell := range row {
			contentBox := cell.content.GetBox(ctx, columnWidth)
			rowCells[c] = contentBox
			if h := contentBox.Bounds().Dy(); h > rowHeight {
				rowHeight = h
			}
		}
		for c, cell := range row {
			contentWidth := rowCells[c].Bounds().Dx()
			xOffset := 0
			switch cell.alignment {
			case alignCenter:
				xOffset = (columnWidth - contentWidth) / 2
			case alignRight:
				xOffset = columnWidth - contentWidth
			}
			rowCells[c] = NewContainerBox(rowCells[c], columnWidth, rowHeight, xOffset, 0)
		}
		cells[r] = rowCells
		rowHeights[r] = rowHeight
	}

	// Same reasoning as columnOffsets: rowGap pads the top and bottom
	// against the frame too, not just between rows. The header->body
	// transition keeps its own smaller headerGap either way.
	rowOffsets := make([]int, len(rows)+1)
	rowOffsets[0] = frameThickness + rowGap
	for r, h := range rowHeights {
		rowOffsets[r+1] = rowOffsets[r] + h
		switch {
		case r == len(rowHeights)-1:
			rowOffsets[r+1] += rowGap + frameThickness
		case r == 0:
			rowOffsets[r+1] += headerGap
		default:
			rowOffsets[r+1] += rowGap
		}
	}

	return &TableBox{
		columnOffsets:  columnOffsets,
		rowOffsets:     rowOffsets,
		frameThickness: frameThickness,
		frameColor:     b.frameColor,
		cells:          cells,
	}
}

// GetBox builds the slot skeleton only - gap sizes from Margins(), which is
// cheap (no text measurement) - deferring each block's own GetBox call to
// StackBox.boxAt, on first access to that slot. A resize only needs to
// re-derive gap sizes and widths up front; the expensive part (actually
// laying out each block's content) only happens for slots something later
// asks for, e.g. those near a scroll anchor.
func (b *StackBlock) GetBox(ctx RenderingContext, width int) Box {
	slots := make([]stackSlot, 0, len(b.blocks))
	bottomMargin := 0
	for i, block := range b.blocks {
		margins := ctx.ScaleMargins(block.Margins())
		if i > 0 {
			gap := maxInt(bottomMargin, int(margins.Top))
			if gap > 0 {
				slots = append(slots, stackSlot{box: NewEmptyBox(width, gap)})
			}
		}
		leftMargin := int(margins.Left)
		rightMargin := int(margins.Right)
		slots = append(slots, stackSlot{
			block:      block,
			width:      width - leftMargin - rightMargin,
			leftMargin: leftMargin,
			wrap:       leftMargin > 0 || rightMargin > 0,
		})
		bottomMargin = int(margins.Bottom)
	}
	return &StackBox{slots: slots, ctx: ctx, width: width}
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
