package whynot

import (
	"image"
	_ "image/jpeg" // registers the JPEG format with image.DecodeConfig
	"os"
	"sort"
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
		lineBoxes[i] = &LineBox{parts: []InlineBox{line.GetInlineBox(ctx)}}
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
		lines = append(lines, &LineBox{parts: boxes[:i]})
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
		lines = append(lines, &LineBox{parts: boxes[:i]})
		boxes = boxes[i:]
	}
	return &StackBox{slots: preResolvedSlots(lines)}
}

// tableFrameThickness is the frame/header-rule thickness; tableColumnGap
// and tableRowGap are the space between columns and between ordinary
// rows; tableHeaderGap is the (smaller) space between the header row and
// the rule under it - see TableBox for why that makes rowOffsets[1]
// exactly the rule's position, with no separate field needed for it.
// tableColumnRuleThickness is the thickness of the vertical rule drawn
// between columns, centered in the column gap. All in unscaled pixels,
// scaled by ctx.Scale like Margins.
const (
	tableFrameThickness      = 2
	tableColumnGap           = 12
	tableRowGap              = 6
	tableHeaderGap           = 4
	tableColumnRuleThickness = 1
)

// naturalWidthMeasure is an effectively-unbounded width passed to a
// cell's GetBox purely to measure its natural (unwrapped) width via the
// resulting Box's Bounds() - large enough that no realistic cell content
// would ever wrap against it.
const naturalWidthMeasure = 1 << 20

// resolveColumnWidths decides each column's final width from its natural
// (unwrapped) width, given the total width available for all columns
// combined (gaps and the frame are the caller's concern, not this
// function's). Deliberately isolated from measuring natural widths and
// from using the result, so alternative column-width policies can be
// swapped in later without touching either.
//
// Columns split into "narrow" (kept at natural width) and "wide" (shrunk,
// sharing the leftover space proportionally to natural width). The split
// point is the largest K such that the combined natural width of the K
// narrowest columns is less than what a single wide column would get if
// the remaining N-K columns split the rest equally:
//
//	sum(narrowest K) < available/(N-K+1)
//
// Checked as sum*(N-K+1) < available (exact with integers, and avoids
// assuming the condition is monotonic in K - it isn't, in general, since
// the sum grows and the divisor shrinks as K grows).
func resolveColumnWidths(natural []int, available int) []int {
	n := len(natural)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return natural[order[i]] < natural[order[j]] })

	prefixSum := make([]int, n+1)
	for i, idx := range order {
		prefixSum[i+1] = prefixSum[i] + natural[idx]
	}

	k := 0
	for candidate := 0; candidate <= n; candidate++ {
		if prefixSum[candidate]*(n-candidate+1) < available {
			k = candidate
		}
	}

	result := make([]int, n)
	for i := 0; i < k; i++ {
		result[order[i]] = natural[order[i]]
	}
	if k < n {
		wideNatural := prefixSum[n] - prefixSum[k]
		remaining := available - prefixSum[k]
		if remaining < 0 {
			remaining = 0
		}
		for i := k; i < n; i++ {
			idx := order[i]
			if wideNatural > 0 {
				result[idx] = remaining * natural[idx] / wideNatural
			}
		}
	}
	return result
}

func (b *TableBlock) GetBox(ctx RenderingContext, width int) Box {
	frameThickness := int(tableFrameThickness * ctx.Scale)
	columnGap := int(tableColumnGap * ctx.Scale)
	rowGap := int(tableRowGap * ctx.Scale)
	headerGap := int(tableHeaderGap * ctx.Scale)
	columnRuleThickness := maxInt(1, int(tableColumnRuleThickness*ctx.Scale))

	numCols := len(b.header)
	rows := append([][]tableCell{b.header}, b.rows...)

	natural := make([]int, numCols)
	for _, row := range rows {
		for c, cell := range row {
			if w := cell.content.GetBox(ctx, naturalWidthMeasure).Bounds().Dx(); w > natural[c] {
				natural[c] = w
			}
		}
	}

	// A column rule sits centered in the columnGap between two columns,
	// so the whitespace it leaves on either side is only columnGap/2 -
	// half against the frame would double that rhythm at the table's own
	// edges, which reads as a lopsided margin. So edgeGap (columnGap/2)
	// pads each side against the frame instead, matching a rule's own
	// half-gap; total gap budget is edgeGap*2 + (numCols-1)*columnGap,
	// which is exactly numCols*columnGap.
	edgeGap := columnGap / 2
	available := width - 2*frameThickness - numCols*columnGap
	columnWidths := resolveColumnWidths(natural, available)

	// columnWidths is the wrap constraint given to each cell, not
	// necessarily what it actually renders at: word-wrapped text rarely
	// uses every last pixel of the width it's given, so a wrapped cell's
	// real Bounds() is often narrower than the column it wrapped to.
	// Track each column's effective width - the widest a cell in it
	// actually rendered - and use that for offsets/alignment instead of
	// the looser allocated width, so a wide-but-wrapped column doesn't
	// leave a bigger gap before the next one than it needs to.
	rawCells := make([][]Box, len(rows))
	rowHeights := make([]int, len(rows))
	effectiveWidths := make([]int, numCols)
	for r, row := range rows {
		rowBoxes := make([]Box, numCols)
		rowHeight := 0
		for c, cell := range row {
			contentBox := cell.content.GetBox(ctx, columnWidths[c])
			rowBoxes[c] = contentBox
			if h := contentBox.Bounds().Dy(); h > rowHeight {
				rowHeight = h
			}
			if w := contentBox.Bounds().Dx(); w > effectiveWidths[c] {
				effectiveWidths[c] = w
			}
		}
		rawCells[r] = rowBoxes
		rowHeights[r] = rowHeight
	}

	columnOffsets := make([]int, numCols+1)
	columnOffsets[0] = frameThickness + edgeGap
	for c := range numCols {
		gap := columnGap
		if c == numCols-1 {
			gap = edgeGap
		}
		columnOffsets[c+1] = columnOffsets[c] + effectiveWidths[c] + gap
		if c == numCols-1 {
			columnOffsets[c+1] += frameThickness
		}
	}

	cells := make([][]Box, len(rows))
	for r, row := range rows {
		rowCells := make([]Box, numCols)
		for c, cell := range row {
			contentBox := rawCells[r][c]
			contentWidth := contentBox.Bounds().Dx()
			xOffset := 0
			switch cell.alignment {
			case alignCenter:
				xOffset = (effectiveWidths[c] - contentWidth) / 2
			case alignRight:
				xOffset = effectiveWidths[c] - contentWidth
			}
			rowCells[c] = NewContainerBox(contentBox, effectiveWidths[c], rowHeights[r], xOffset, 0)
		}
		cells[r] = rowCells
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
		columnOffsets:       columnOffsets,
		rowOffsets:          rowOffsets,
		frameThickness:      frameThickness,
		columnGap:           columnGap,
		columnRuleThickness: columnRuleThickness,
		frameColor:          b.frameColor,
		cells:               cells,
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
		// bounds.Dx(), not Max.X: a later word's own bounds can pull
		// Min.X away from 0 (e.g. left-side bearing), so Max.X alone
		// isn't the line's true width - comparing it directly against
		// width makes the wrap constraint tighter than intended, by
		// however far Min.X has drifted.
		if bounds.Dx() > width {
			return i + 1, bounds
		}
		advance += boxAdvance
	}
	return len(boxes), bounds
}
