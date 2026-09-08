package whynot

import (
	"image"
	"image/color"
	"testing"

	"golang.org/x/image/font"
)

// unwrap peels away MarginBlock wrapping to reach the concrete block doing
// the actual layout, for tests asserting on that type regardless of how
// many margin decorations wrap it (in practice at most one).
func unwrap(b Block) Block {
	for {
		mb, ok := b.(*MarginBlock)
		if !ok {
			return b
		}
		b = mb.Block
	}
}

// textOf collects the text of each InlineText in parts, in order. Since
// appendString splits on whitespace, a single source phrase becomes one
// InlineText per word.
func textOf(t *testing.T, parts []Inline) []string {
	t.Helper()
	words := make([]string, len(parts))
	for i, part := range parts {
		text, ok := part.(*InlineText)
		if !ok {
			t.Fatalf("part %d is a %T, not *InlineText", i, part)
		}
		words[i] = text.text
	}
	return words
}

// listItemParts returns item's ListItemHeadBlock and, if present, its
// trailing block - item is the StackBlock CompileListItem builds for a
// single list item (blocks: [head] or [head, trailing]).
func listItemParts(t *testing.T, item Block) (head *ListItemHeadBlock, trailing Block) {
	t.Helper()
	stack, ok := unwrap(item).(*StackBlock)
	if !ok || len(stack.blocks) == 0 || len(stack.blocks) > 2 {
		t.Fatalf("item = %#v, want a StackBlock with 1 or 2 blocks", item)
	}
	head, ok = unwrap(stack.blocks[0]).(*ListItemHeadBlock)
	if !ok {
		t.Fatalf("item's first block = %T, want *ListItemHeadBlock", stack.blocks[0])
	}
	if len(stack.blocks) == 2 {
		trailing = stack.blocks[1]
	}
	return head, trailing
}

func stringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParseParagraph(t *testing.T) {
	doc := Parse([]byte("Hello there world"))
	stack, ok := doc.(*StackBlock)
	if !ok || len(stack.blocks) != 1 {
		t.Fatalf("Parse result = %#v, want a single-block StackBlock", doc)
	}
	para, ok := unwrap(stack.blocks[0]).(*TextBlock)
	if !ok {
		t.Fatalf("block = %T, want *TextBlock", stack.blocks[0])
	}
	got := textOf(t, para.parts)
	want := []string{"Hello", "there", "world"}
	if !stringsEqual(got, want) {
		t.Errorf("words = %v, want %v", got, want)
	}
}

func TestParseHeading(t *testing.T) {
	doc := Parse([]byte("### Level three"))
	stack := doc.(*StackBlock)
	heading, ok := unwrap(stack.blocks[0]).(*TextBlock)
	if !ok {
		t.Fatalf("block = %T, want *TextBlock", stack.blocks[0])
	}
	got := textOf(t, heading.parts)
	want := []string{"Level", "three"}
	if !stringsEqual(got, want) {
		t.Errorf("words = %v, want %v", got, want)
	}
	// Heading weight/size/margins are resolved later, from StyleSheet
	// against the node's own tag (TagHeading3 here) - nothing further to
	// assert here structurally.
}

func TestParseTightList(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   []string
	}{
		{"dash", "- one\n- two\n- three", []string{"-", "-", "-"}},
		{"paren", "1) one\n2) two", []string{"1)", "2)"}},
		{"dot", "1. one\n2. two", []string{"1.", "2."}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := Parse([]byte(tc.source))
			stack := doc.(*StackBlock)
			list, ok := unwrap(stack.blocks[0]).(*StackBlock)
			if !ok || len(list.blocks) != len(tc.want) {
				t.Fatalf("list = %#v, want a %d-item StackBlock", stack.blocks[0], len(tc.want))
			}
			for i, block := range list.blocks {
				head, _ := listItemParts(t, block)
				marker, ok := head.marker.(*InlineText)
				if !ok || marker.text != tc.want[i] {
					t.Errorf("item %d marker = %#v, want %q", i, head.marker, tc.want[i])
				}
			}
		})
	}
}

func TestParseTaskList(t *testing.T) {
	doc := Parse([]byte("- [ ] todo item\n- [x] done item\n- plain item"))
	stack := doc.(*StackBlock)
	list, ok := unwrap(stack.blocks[0]).(*StackBlock)
	if !ok || len(list.blocks) != 3 {
		t.Fatalf("list = %#v, want a 3-item StackBlock", stack.blocks[0])
	}

	wantMarker := []string{"□", "■", "-"}
	wantWords := [][]string{{"todo", "item"}, {"done", "item"}, {"plain", "item"}}
	for i, block := range list.blocks {
		head, _ := listItemParts(t, block)
		marker, ok := head.marker.(*InlineText)
		if !ok || marker.text != wantMarker[i] {
			t.Errorf("item %d marker = %#v, want %q", i, head.marker, wantMarker[i])
		}
		// The checkbox syntax must be fully consumed by the task list
		// parser - it shouldn't leak into the item's own text as a
		// leftover "[ ]"/"[x]" word.
		got := textOf(t, head.parts)
		if !stringsEqual(got, wantWords[i]) {
			t.Errorf("item %d words = %v, want %v", i, got, wantWords[i])
		}
	}
}

// TestParseNestedList checks that a nested list becomes the parent item's
// trailing block, stacked below its own paragraph text, while a sibling
// item with no nested content gets no trailing block at all.
func TestParseNestedList(t *testing.T) {
	doc := Parse([]byte("- one\n  - nested\n- two"))
	stack := doc.(*StackBlock)
	list := unwrap(stack.blocks[0]).(*StackBlock)
	if len(list.blocks) != 2 {
		t.Fatalf("list = %#v, want 2 items", list.blocks)
	}

	oneHead, oneTrailing := listItemParts(t, list.blocks[0])
	if got := textOf(t, oneHead.parts); !stringsEqual(got, []string{"one"}) {
		t.Errorf("item 0 words = %v, want [one]", got)
	}
	nestedList, ok := unwrap(oneTrailing).(*StackBlock)
	if !ok || len(nestedList.blocks) != 1 {
		t.Fatalf("item 0 trailing = %#v, want a 1-item StackBlock", oneTrailing)
	}
	nestedHead, _ := listItemParts(t, nestedList.blocks[0])
	if got := textOf(t, nestedHead.parts); !stringsEqual(got, []string{"nested"}) {
		t.Errorf("nested item words = %v, want [nested]", got)
	}

	_, twoTrailing := listItemParts(t, list.blocks[1])
	if twoTrailing != nil {
		t.Errorf("item 1 trailing = %#v, want nil", twoTrailing)
	}
}

// TestListItemTrailingGap checks that a gap - the trailing block's own
// top margin - separates an item's own text from its trailing content
// (e.g. a nested list), rather than stacking them flush against each
// other.
func TestListItemTrailingGap(t *testing.T) {
	doc := Parse([]byte("- one\n  - nested"))
	stack := doc.(*StackBlock)
	list := unwrap(stack.blocks[0]).(*StackBlock)
	item := list.blocks[0]
	_, trailing := listItemParts(t, item)
	if trailing == nil {
		t.Fatal("trailing = nil, want the nested list")
	}

	ctx := RenderingContext{Scale: 1, FaceSelector: NewGoFontFaceSelector(72), StyleSheet: NewDefaultStyleSheet()}
	box := item.GetBox(ctx, 200).(*StackBox)
	if len(box.slots) != 3 {
		t.Fatalf("got %d slots, want 3 (head, gap, trailing): %#v", len(box.slots), box.slots)
	}
	gapBox, ok := box.boxAt(1).(*EmptyBox)
	if !ok {
		t.Fatalf("slot 1 = %T, want *EmptyBox", box.boxAt(1))
	}
	wantGap := int(ctx.ScaledMargins(trailing).Top)
	if wantGap == 0 {
		t.Fatal("test is meaningless if the nested list's own top margin is 0")
	}
	if got := gapBox.Bounds().Dy(); got != wantGap {
		t.Errorf("gap height = %d, want %d", got, wantGap)
	}
}

// TestLineBoxBoundsIncludesInterWordSpacing pins down a real bug found
// while building tables: LineBox.BoundsAndAdvance used to advance by its
// own always-zero space field between words, instead of each word's real
// SpaceWidth() (which drawContents and splitBoxes both correctly use) -
// so Bounds() silently under-reported a multi-word line's true width by
// one space-width per gap. Invisible for ordinary paragraphs (nothing
// else sits flush against their measured edge), but exactly what caused
// table cells to overlap the next column once their measured width was
// used to position it.
func TestLineBoxBoundsIncludesInterWordSpacing(t *testing.T) {
	ctx := RenderingContext{Scale: 1, FaceSelector: NewGoFontFaceSelector(72), StyleSheet: NewDefaultStyleSheet()}
	widthOf := func(source string) int {
		t.Helper()
		doc := Parse([]byte(source))
		para := unwrap(doc.(*StackBlock).blocks[0]).(*TextBlock)
		return para.GetBox(ctx, naturalWidthMeasure).Bounds().Dx()
	}

	widthA := widthOf("A")
	widthB := widthOf("B")
	widthAB := widthOf("A B")

	if widthAB <= widthA+widthB {
		t.Errorf(`width("A B") = %d, want > width("A")+width("B") = %d+%d=%d (no inter-word gap counted)`,
			widthAB, widthA, widthB, widthA+widthB)
	}
}

// TestSplitBoxesNaturalWidthFits pins down a second bug found while
// building tables, alongside the inter-word-spacing one: splitBoxes'
// wrap check compared bounds.Max.X directly against width, but a later
// word's own bounds can pull bounds.Min.X away from 0 (e.g. a small
// left-side bearing), making Max.X alone wider than the line's true span
// (Dx()). So a line built at exactly its own measured natural width -
// which should always fit on one line, by definition - would still wrap
// its last word. This specific sentence reliably drifts Min.X to 1.
func TestSplitBoxesNaturalWidthFits(t *testing.T) {
	ctx := RenderingContext{Scale: 1, FaceSelector: NewGoFontFaceSelector(72), StyleSheet: NewDefaultStyleSheet()}
	doc := Parse([]byte("Fast, cheap, and easy to set up with minimal configuration required"))
	para := unwrap(doc.(*StackBlock).blocks[0]).(*TextBlock)

	natWidth := para.GetBox(ctx, naturalWidthMeasure).Bounds().Dx()
	box := para.GetBox(ctx, natWidth).(*StackBox)
	if len(box.slots) != 1 {
		t.Errorf("built at its own natural width (%d), got %d lines, want 1", natWidth, len(box.slots))
	}
}

func TestResolveColumnWidths(t *testing.T) {
	cases := []struct {
		name      string
		natural   []int
		available int
		want      []int
	}{
		{
			name:      "fits naturally",
			natural:   []int{10, 20, 30},
			available: 100,
			want:      []int{10, 20, 30},
		},
		{
			// Worked by hand: sorted [10,20,100], K=1 (10 < 60/3=20;
			// 10+20=30 is not < 60/2=30), so column 0 stays narrow at
			// 10, and columns 1/2 share the remaining 50 proportionally
			// to their natural width (20:100).
			name:      "one narrow, two wide, sharing proportionally",
			natural:   []int{10, 20, 100},
			available: 60,
			want:      []int{10, 8, 41},
		},
		{
			// Same values, shuffled column order, checking the result
			// maps back to the original (not sorted) positions.
			name:      "shuffled order maps back correctly",
			natural:   []int{100, 10, 20},
			available: 60,
			want:      []int{41, 10, 8},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveColumnWidths(tc.natural, tc.available)
			if len(got) != len(tc.want) {
				t.Fatalf("resolveColumnWidths(%v, %d) = %v, want %v", tc.natural, tc.available, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("resolveColumnWidths(%v, %d) = %v, want %v", tc.natural, tc.available, got, tc.want)
					break
				}
			}
		})
	}
}

// TestParseListItemNoLeadingParagraph checks that a list item opening
// directly with a nested list (no text of its own) leaves the marker
// standing alone rather than panicking - the item's parts end up empty,
// and the nested list becomes its trailing block.
func TestParseListItemNoLeadingParagraph(t *testing.T) {
	doc := Parse([]byte("- - nested only\n"))
	stack := doc.(*StackBlock)
	outer := unwrap(stack.blocks[0]).(*StackBlock)
	head, trailing := listItemParts(t, outer.blocks[0])
	if len(head.parts) != 0 {
		t.Errorf("parts = %#v, want none", head.parts)
	}
	nestedList, ok := unwrap(trailing).(*StackBlock)
	if !ok || len(nestedList.blocks) != 1 {
		t.Fatalf("trailing = %#v, want a 1-item StackBlock", trailing)
	}
}

// TestParseLooseList checks that a loose list (items separated by a blank
// line) no longer panics, and that each item's own leading text picks up
// real paragraph margins instead of a tight item's zero margins.
func TestParseLooseList(t *testing.T) {
	ctx := RenderingContext{StyleSheet: NewDefaultStyleSheet()}
	doc := Parse([]byte("- one\n\n- two"))
	stack := doc.(*StackBlock)
	list, ok := unwrap(stack.blocks[0]).(*StackBlock)
	if !ok || len(list.blocks) != 2 {
		t.Fatalf("list = %#v, want a 2-item StackBlock", stack.blocks[0])
	}

	wantWords := []string{"one", "two"}
	for i, item := range list.blocks {
		itemStack, ok := unwrap(item).(*StackBlock)
		if !ok || len(itemStack.blocks) != 1 {
			t.Fatalf("item %d = %#v, want a 1-block StackBlock (head only)", i, item)
		}
		headBlock := itemStack.blocks[0]
		head, ok := unwrap(headBlock).(*ListItemHeadBlock)
		if !ok {
			t.Fatalf("item %d head = %T, want *ListItemHeadBlock", i, headBlock)
		}
		if got := textOf(t, head.parts); !stringsEqual(got, []string{wantWords[i]}) {
			t.Errorf("item %d words = %v, want [%s]", i, got, wantWords[i])
		}
		if got := headBlock.Margins(ctx); got != (Margins{Top: 10, Bottom: 10}) {
			t.Errorf("item %d head margins = %+v, want {Top: 10, Bottom: 10} (paragraphStyle)", i, got)
		}
	}
}

// TestParseLooseListMultiParagraphItem checks that a loose item's second
// paragraph gets real paragraph margins via the same generic
// trailing-block path already used for a nested list - no special-casing
// needed.
func TestParseLooseListMultiParagraphItem(t *testing.T) {
	ctx := RenderingContext{StyleSheet: NewDefaultStyleSheet()}
	doc := Parse([]byte("- first paragraph\n\n  second paragraph\n"))
	stack := doc.(*StackBlock)
	list, ok := unwrap(stack.blocks[0]).(*StackBlock)
	if !ok || len(list.blocks) != 1 {
		t.Fatalf("list = %#v, want a 1-item StackBlock", stack.blocks[0])
	}

	itemStack, ok := unwrap(list.blocks[0]).(*StackBlock)
	if !ok || len(itemStack.blocks) != 2 {
		t.Fatalf("item = %#v, want a 2-block StackBlock (head, second paragraph)", list.blocks[0])
	}

	head, ok := unwrap(itemStack.blocks[0]).(*ListItemHeadBlock)
	if !ok {
		t.Fatalf("item block 0 = %T, want *ListItemHeadBlock", itemStack.blocks[0])
	}
	if got := textOf(t, head.parts); !stringsEqual(got, []string{"first", "paragraph"}) {
		t.Errorf("head words = %v, want [first paragraph]", got)
	}

	second, ok := unwrap(itemStack.blocks[1]).(*TextBlock)
	if !ok {
		t.Fatalf("item block 1 = %T, want *TextBlock", itemStack.blocks[1])
	}
	if got := textOf(t, second.parts); !stringsEqual(got, []string{"second", "paragraph"}) {
		t.Errorf("second paragraph words = %v, want [second paragraph]", got)
	}
	if got := itemStack.blocks[1].Margins(ctx); got != (Margins{Top: 10, Bottom: 10}) {
		t.Errorf("second paragraph margins = %+v, want {Top: 10, Bottom: 10}", got)
	}
}

func TestParseFencedCodeBlock(t *testing.T) {
	doc := Parse([]byte("```\nline one\nline two\n```"))
	stack := doc.(*StackBlock)
	code, ok := unwrap(stack.blocks[0]).(*CodeBlock)
	if !ok {
		t.Fatalf("block = %T, want *CodeBlock", stack.blocks[0])
	}
	if len(code.lines) != 2 {
		t.Fatalf("got %d lines, want 2: %#v", len(code.lines), code.lines)
	}
	want := []string{"line one\n", "line two\n"}
	for i, line := range code.lines {
		text, ok := line.(*InlineText)
		if !ok || text.text != want[i] {
			t.Errorf("line %d = %#v, want %q", i, line, want[i])
		}
	}
}

func TestParseIndentedCodeBlock(t *testing.T) {
	doc := Parse([]byte("    line one\n    line two"))
	stack := doc.(*StackBlock)
	code, ok := unwrap(stack.blocks[0]).(*CodeBlock)
	if !ok {
		t.Fatalf("block = %T, want *CodeBlock", stack.blocks[0])
	}
	want := []string{"line one\n", "line two\n"}
	if len(code.lines) != len(want) {
		t.Fatalf("got %d lines, want %d: %#v", len(code.lines), len(want), code.lines)
	}
	for i, line := range code.lines {
		text, ok := line.(*InlineText)
		if !ok || text.text != want[i] {
			t.Errorf("line %d = %#v, want %q", i, line, want[i])
		}
	}
}

// TestParseCodeBlockExpandsTabs checks that a literal tab in a fenced code
// block's source - preserved verbatim by goldmark, unlike leading
// indentation elsewhere in the document - is expanded to spaces rather
// than reaching the font as a raw tab character, which renders as a
// placeholder box instead of whitespace.
func TestParseCodeBlockExpandsTabs(t *testing.T) {
	doc := Parse([]byte("```\n\tindented\n```"))
	stack := doc.(*StackBlock)
	code, ok := unwrap(stack.blocks[0]).(*CodeBlock)
	if !ok {
		t.Fatalf("block = %T, want *CodeBlock", stack.blocks[0])
	}
	if len(code.lines) != 1 {
		t.Fatalf("got %d lines, want 1: %#v", len(code.lines), code.lines)
	}
	text, ok := code.lines[0].(*InlineText)
	if !ok {
		t.Fatalf("line = %T, want *InlineText", code.lines[0])
	}
	want := codeBlockTabExpansion + "indented\n"
	if text.text != want {
		t.Errorf("text = %q, want %q", text.text, want)
	}
}

func TestParseThematicBreak(t *testing.T) {
	doc := Parse([]byte("---"))
	stack := doc.(*StackBlock)
	wrapper, ok := stack.blocks[0].(*MarginBlock)
	if !ok {
		t.Fatalf("block = %T, want *MarginBlock", stack.blocks[0])
	}
	rule, ok := wrapper.Block.(*ThematicBreakBlock)
	if !ok {
		t.Fatalf("wrapper.Block = %T, want *ThematicBreakBlock", wrapper.Block)
	}

	styleSheet := NewDefaultStyleSheet()
	ctx := RenderingContext{Scale: 1, StyleSheet: styleSheet}
	if got := wrapper.Margins(ctx); got != (Margins{Top: 20, Bottom: 20}) {
		t.Errorf("Margins(ctx) = %+v, want {Top: 20, Bottom: 20}", got)
	}
	box := rule.GetBox(ctx, 100)
	ruleBox, ok := box.(*RuleBox)
	if !ok {
		t.Fatalf("GetBox = %T, want *RuleBox", box)
	}
	want := image.Rect(0, 0, 100, int(styleSheet.ThematicBreakThickness(nil)))
	if got := ruleBox.Bounds(); got != want {
		t.Errorf("Bounds() = %v, want %v", got, want)
	}
}

// TestParseBlockquote checks that a single-block blockquote's inner is
// that block directly, not wrapped in a StackBlock (wrapBlocks only wraps
// when there's more than one).
func TestParseBlockquote(t *testing.T) {
	doc := Parse([]byte("> quoted text"))
	stack := doc.(*StackBlock)
	bq, ok := unwrap(stack.blocks[0]).(*BlockquoteBlock)
	if !ok {
		t.Fatalf("block = %T, want *BlockquoteBlock", stack.blocks[0])
	}
	para, ok := unwrap(bq.inner).(*TextBlock)
	if !ok {
		t.Fatalf("inner = %T, want *TextBlock", bq.inner)
	}
	got := textOf(t, para.parts)
	want := []string{"quoted", "text"}
	if !stringsEqual(got, want) {
		t.Errorf("words = %v, want %v", got, want)
	}
}

// TestParseBlockquoteMultipleBlocks checks that a blockquote spanning more
// than one block wraps them in a StackBlock, unlike the single-block case.
func TestParseBlockquoteMultipleBlocks(t *testing.T) {
	doc := Parse([]byte("> first\n>\n> second"))
	stack := doc.(*StackBlock)
	bq := unwrap(stack.blocks[0]).(*BlockquoteBlock)
	inner, ok := bq.inner.(*StackBlock)
	if !ok || len(inner.blocks) != 2 {
		t.Fatalf("inner = %#v, want a 2-block StackBlock", bq.inner)
	}
}

// TestParseNestedBlockquote checks that a blockquote inside a blockquote
// (`> > ...`) compiles recursively - each level gets its own bar when
// drawn, with no special-casing needed since BlockquoteBlock positions
// itself rather than relying on its parent.
func TestParseNestedBlockquote(t *testing.T) {
	doc := Parse([]byte("> > nested quote"))
	stack := doc.(*StackBlock)
	outer := unwrap(stack.blocks[0]).(*BlockquoteBlock)
	inner, ok := unwrap(outer.inner).(*BlockquoteBlock)
	if !ok {
		t.Fatalf("inner = %T, want *BlockquoteBlock", outer.inner)
	}
	para, ok := unwrap(inner.inner).(*TextBlock)
	if !ok {
		t.Fatalf("innermost = %T, want *TextBlock", inner.inner)
	}
	got := textOf(t, para.parts)
	want := []string{"nested", "quote"}
	if !stringsEqual(got, want) {
		t.Errorf("words = %v, want %v", got, want)
	}
}

func TestBlockquoteBoxIndent(t *testing.T) {
	bq := &BlockquoteBlock{
		inner: &fixedHeightBlock{height: 10},
	}
	styleSheet := NewDefaultStyleSheet()
	ctx := RenderingContext{Scale: 1, StyleSheet: styleSheet}
	box := bq.GetBox(ctx, 100)
	bqBox, ok := box.(*BlockquoteBox)
	if !ok {
		t.Fatalf("GetBox = %T, want *BlockquoteBox", box)
	}
	blockquoteIndent := int(styleSheet.BlockquoteGeometry(nil).Indent)
	if bqBox.indent != blockquoteIndent {
		t.Errorf("indent = %d, want %d", bqBox.indent, blockquoteIndent)
	}
	wantInnerWidth := 100 - blockquoteIndent
	if got := bqBox.inner.Bounds().Dx(); got != wantInnerWidth {
		t.Errorf("inner width = %d, want %d", got, wantInnerWidth)
	}
	want := image.Rect(0, 0, 100, 10)
	if got := bqBox.Bounds(); got != want {
		t.Errorf("Bounds() = %v, want %v", got, want)
	}
}

func TestParseLink(t *testing.T) {
	doc := Parse([]byte(`[click *here*](https://example.com "a title")`))
	stack := doc.(*StackBlock)
	para := unwrap(stack.blocks[0]).(*TextBlock)
	got := textOf(t, para.parts)
	want := []string{"click", "here"}
	if !stringsEqual(got, want) {
		t.Fatalf("words = %v, want %v", got, want)
	}
	ctx := RenderingContext{StyleSheet: NewDefaultStyleSheet()}
	for i, part := range para.parts {
		text := part.(*InlineText)
		if ctx.ResolvedColor(text.node) == color.White {
			t.Errorf("part %d color = white, want link color", i)
		}
	}
	// Nested emphasis inside the link text should still apply on top of
	// the link's color.
	if style := ctx.ResolvedTextStyle(para.parts[1].(*InlineText).node); style.Style != font.StyleItalic {
		t.Errorf("style of %q = %+v, want italic", "here", style)
	}
}

func TestParseAutoLink(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{"url", "<https://example.com>", "https://example.com"},
		{"email", "<user@example.com>", "user@example.com"},
	}
	ctx := RenderingContext{StyleSheet: NewDefaultStyleSheet()}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := Parse([]byte(tc.source))
			stack := doc.(*StackBlock)
			para := unwrap(stack.blocks[0]).(*TextBlock)
			if len(para.parts) != 1 {
				t.Fatalf("parts = %#v, want 1 part", para.parts)
			}
			text := para.parts[0].(*InlineText)
			if text.text != tc.want {
				t.Errorf("text = %q, want %q", text.text, tc.want)
			}
			if ctx.ResolvedColor(text.node) == color.White {
				t.Errorf("color = white, want link color")
			}
		})
	}
}

func TestParseEmphasisAndStrong(t *testing.T) {
	cases := []struct {
		name       string
		source     string
		wantStyle  font.Style
		wantWeight font.Weight
	}{
		{"emphasis", "*word*", font.StyleItalic, font.WeightNormal},
		{"strong", "**word**", font.StyleNormal, font.WeightBold},
		{"both", "***word***", font.StyleItalic, font.WeightBold},
	}
	ctx := RenderingContext{StyleSheet: NewDefaultStyleSheet()}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := Parse([]byte(tc.source))
			stack := doc.(*StackBlock)
			para := unwrap(stack.blocks[0]).(*TextBlock)
			if len(para.parts) != 1 {
				t.Fatalf("parts = %#v, want 1 part", para.parts)
			}
			text := para.parts[0].(*InlineText)
			if text.text != "word" {
				t.Errorf("text = %q, want %q", text.text, "word")
			}
			style := ctx.ResolvedTextStyle(text.node)
			if style.Style != tc.wantStyle || style.Weight != tc.wantWeight {
				t.Errorf("style = %+v, want Style=%v Weight=%v", style, tc.wantStyle, tc.wantWeight)
			}
		})
	}
}

func TestParseCodeSpan(t *testing.T) {
	doc := Parse([]byte("see `code` here"))
	stack := doc.(*StackBlock)
	para := unwrap(stack.blocks[0]).(*TextBlock)
	got := textOf(t, para.parts)
	want := []string{"see", "code", "here"}
	if !stringsEqual(got, want) {
		t.Fatalf("words = %v, want %v", got, want)
	}
	ctx := RenderingContext{StyleSheet: NewDefaultStyleSheet()}
	code := para.parts[1].(*InlineText)
	if style := ctx.ResolvedTextStyle(code.node); style.Family != Monospace {
		t.Errorf("code span family = %v, want Monospace", style.Family)
	}
}

// TestParseStrikethrough checks that being struck composes with nested
// styling (bold from Strong here) rather than replacing it - struck-ness
// is a structural ancestry fact (HasAncestorTag), resolved independently
// of TextStyle.
func TestParseStrikethrough(t *testing.T) {
	doc := Parse([]byte("plain ~~struck **and bold**~~ text"))
	stack := doc.(*StackBlock)
	para := unwrap(stack.blocks[0]).(*TextBlock)
	got := textOf(t, para.parts)
	want := []string{"plain", "struck", "and", "bold", "text"}
	if !stringsEqual(got, want) {
		t.Fatalf("words = %v, want %v", got, want)
	}

	plain := para.parts[0].(*InlineText)
	if plain.node.HasAncestorTag(TagStrikethrough) {
		t.Errorf("part %q: struck = true, want false", plain.text)
	}

	struckWords := para.parts[1:4]
	for _, part := range struckWords {
		text := part.(*InlineText)
		if !text.node.HasAncestorTag(TagStrikethrough) {
			t.Errorf("part %q: struck = false, want true", text.text)
		}
	}
	ctx := RenderingContext{StyleSheet: NewDefaultStyleSheet()}
	bold := para.parts[3].(*InlineText)
	if style := ctx.ResolvedTextStyle(bold.node); style.Weight != font.WeightBold {
		t.Errorf("part %q: weight = %v, want bold", bold.text, style.Weight)
	}

	trailing := para.parts[4].(*InlineText)
	if trailing.node.HasAncestorTag(TagStrikethrough) {
		t.Errorf("part %q: struck = true, want false", trailing.text)
	}
}

func TestInlineTextStrikeThickness(t *testing.T) {
	styleSheet := NewDefaultStyleSheet()
	ctx := RenderingContext{Scale: 2, FaceSelector: NewGoFontFaceSelector(72), StyleSheet: styleSheet}

	plainNode := (*ASTNode)(nil).AddChild(TagParagraph).AddChild(TagEmphasis)
	plain := (&InlineText{text: "x", node: plainNode}).GetInlineBox(ctx).(*TextBox)
	if plain.StrikeThickness != 0 {
		t.Errorf("non-struck StrikeThickness = %d, want 0", plain.StrikeThickness)
	}

	struckNode := (*ASTNode)(nil).AddChild(TagParagraph).AddChild(TagStrikethrough)
	struck := (&InlineText{text: "x", node: struckNode}).GetInlineBox(ctx).(*TextBox)
	want := int(styleSheet.StrikeThickness(struckNode) * ctx.Scale)
	if struck.StrikeThickness != want {
		t.Errorf("struck StrikeThickness = %d, want %d", struck.StrikeThickness, want)
	}
}

func TestParseImageWithTitle(t *testing.T) {
	doc := Parse([]byte(`![alt](cat.jpeg "a lovely cat")`))
	stack := doc.(*StackBlock)
	para := unwrap(stack.blocks[0]).(*TextBlock)
	if len(para.parts) != 1 {
		t.Fatalf("parts = %#v, want 1 part", para.parts)
	}
	img, ok := para.parts[0].(*InlineImage)
	if !ok {
		t.Fatalf("part = %T, want *InlineImage", para.parts[0])
	}
	if img.src != "cat.jpeg" {
		t.Errorf("src = %q, want %q", img.src, "cat.jpeg")
	}
	if img.title != "a lovely cat" {
		t.Errorf("title = %q, want %q", img.title, "a lovely cat")
	}
}

// TestParseResolvesEntitiesAndEscapes checks the v2 migration's behavior
// change noted in the migration plan: text values are resolved (entity
// references and backslash escapes decoded), unlike v1's raw Text().
func TestParseResolvesEntitiesAndEscapes(t *testing.T) {
	doc := Parse([]byte(`Fish \& chips and &amp; and \*literal\*`))
	stack := doc.(*StackBlock)
	para := unwrap(stack.blocks[0]).(*TextBlock)
	got := textOf(t, para.parts)
	want := []string{"Fish", "&", "chips", "and", "&", "and", "*literal*"}
	if !stringsEqual(got, want) {
		t.Errorf("words = %v, want %v", got, want)
	}
}
