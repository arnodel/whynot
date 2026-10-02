package whynot

import (
	"image"
	"image/color"
	"testing"

	"golang.org/x/image/font"

	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/images"
	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/engine"
	"github.com/arnodel/whynot/internal/imagecache"
	"github.com/arnodel/whynot/internal/styling/stylingtest"
)

// unwrap peels away MarginBlock wrapping to reach the concrete block doing
// the actual layout, for tests asserting on that type regardless of how
// many margin decorations wrap it (in practice at most one).
func unwrap(b engine.Block) engine.Block {
	for {
		mb, ok := b.(*engine.MarginBlock)
		if !ok {
			return b
		}
		b = mb.Block
	}
}

// textOf collects the text of each InlineText in parts, in order. Since
// appendString splits on whitespace, a single source phrase becomes one
// InlineText per word.
func textOf(t *testing.T, parts []engine.Inline) []string {
	t.Helper()
	words := make([]string, len(parts))
	for i, part := range parts {
		text, ok := part.(*engine.InlineText)
		if !ok {
			t.Fatalf("part %d is a %T, not *InlineText", i, part)
		}
		words[i] = text.Text
	}
	return words
}

// listItemParts returns item's ListItemHeadBlock and, if present, its
// trailing block - item is the StackBlock compileListItem builds for a
// single list item (blocks: [head] or [head, trailing]).
func listItemParts(t *testing.T, item engine.Block) (head *engine.ListItemHeadBlock, trailing engine.Block) {
	t.Helper()
	stack, ok := unwrap(item).(*engine.StackBlock)
	if !ok || len(stack.Blocks) == 0 || len(stack.Blocks) > 2 {
		t.Fatalf("item = %#v, want a StackBlock with 1 or 2 blocks", item)
	}
	head, ok = unwrap(stack.Blocks[0]).(*engine.ListItemHeadBlock)
	if !ok {
		t.Fatalf("item's first block = %T, want *ListItemHeadBlock", stack.Blocks[0])
	}
	if len(stack.Blocks) == 2 {
		trailing = stack.Blocks[1]
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
	stack := doc.root
	if len(stack.Blocks) != 1 {
		t.Fatalf("Parse result = %#v, want a single-block StackBlock", doc)
	}
	para, ok := unwrap(stack.Blocks[0]).(*engine.TextBlock)
	if !ok {
		t.Fatalf("block = %T, want *TextBlock", stack.Blocks[0])
	}
	got := textOf(t, para.Parts)
	want := []string{"Hello", "there", "world"}
	if !stringsEqual(got, want) {
		t.Errorf("words = %v, want %v", got, want)
	}
}

func TestParseHeading(t *testing.T) {
	doc := Parse([]byte("### Level three"))
	stack := doc.root
	heading, ok := unwrap(stack.Blocks[0]).(*engine.TextBlock)
	if !ok {
		t.Fatalf("block = %T, want *TextBlock", stack.Blocks[0])
	}
	got := textOf(t, heading.Parts)
	want := []string{"Level", "three"}
	if !stringsEqual(got, want) {
		t.Errorf("words = %v, want %v", got, want)
	}
	// Heading weight/size/margins are resolved later, from StyleSheet
	// against the node's own tag (ast.TagHeading3 here) - nothing further to
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
		{"start", "3. three\n4. four", []string{"3.", "4."}},
		{"plus", "+ one\n+ two", []string{"+", "+"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := Parse([]byte(tc.source))
			stack := doc.root
			list, ok := unwrap(stack.Blocks[0]).(*engine.StackBlock)
			if !ok || len(list.Blocks) != len(tc.want) {
				t.Fatalf("list = %#v, want a %d-item StackBlock", stack.Blocks[0], len(tc.want))
			}
			for i, block := range list.Blocks {
				head, _ := listItemParts(t, block)
				marker, ok := head.Marker.(*engine.InlineText)
				if !ok || marker.Text != tc.want[i] {
					t.Errorf("item %d marker = %#v, want %q", i, head.Marker, tc.want[i])
				}
			}
		})
	}
}

func TestParseTaskList(t *testing.T) {
	doc := Parse([]byte("- [ ] todo item\n- [x] done item\n- plain item"))
	stack := doc.root
	list, ok := unwrap(stack.Blocks[0]).(*engine.StackBlock)
	if !ok || len(list.Blocks) != 3 {
		t.Fatalf("list = %#v, want a 3-item StackBlock", stack.Blocks[0])
	}

	wantChecked := []bool{false, true}
	wantWords := [][]string{{"todo", "item"}, {"done", "item"}, {"plain", "item"}}
	for i, block := range list.Blocks {
		head, _ := listItemParts(t, block)
		if i < 2 {
			marker, ok := head.Marker.(*engine.TaskCheckbox)
			if !ok || marker.Checked != wantChecked[i] {
				t.Errorf("item %d marker = %#v, want *TaskCheckbox{checked: %v}", i, head.Marker, wantChecked[i])
			}
		} else {
			marker, ok := head.Marker.(*engine.InlineText)
			if !ok || marker.Text != "-" {
				t.Errorf("item %d marker = %#v, want %q", i, head.Marker, "-")
			}
		}
		// The checkbox syntax must be fully consumed by the task list
		// parser - it shouldn't leak into the item's own text as a
		// leftover "[ ]"/"[x]" word.
		got := textOf(t, head.Parts)
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
	stack := doc.root
	list := unwrap(stack.Blocks[0]).(*engine.StackBlock)
	if len(list.Blocks) != 2 {
		t.Fatalf("list = %#v, want 2 items", list.Blocks)
	}

	oneHead, oneTrailing := listItemParts(t, list.Blocks[0])
	if got := textOf(t, oneHead.Parts); !stringsEqual(got, []string{"one"}) {
		t.Errorf("item 0 words = %v, want [one]", got)
	}
	nestedList, ok := unwrap(oneTrailing).(*engine.StackBlock)
	if !ok || len(nestedList.Blocks) != 1 {
		t.Fatalf("item 0 trailing = %#v, want a 1-item StackBlock", oneTrailing)
	}
	nestedHead, _ := listItemParts(t, nestedList.Blocks[0])
	if got := textOf(t, nestedHead.Parts); !stringsEqual(got, []string{"nested"}) {
		t.Errorf("nested item words = %v, want [nested]", got)
	}

	_, twoTrailing := listItemParts(t, list.Blocks[1])
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
	stack := doc.root
	list := unwrap(stack.Blocks[0]).(*engine.StackBlock)
	item := list.Blocks[0]
	_, trailing := listItemParts(t, item)
	if trailing == nil {
		t.Fatal("trailing = nil, want the nested list")
	}

	ctx := engine.Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: stylingtest.Basic()}
	box := item.GetBlockLayout(ctx, 200).(*engine.StackBox)
	if len(box.Slots) != 3 {
		t.Fatalf("got %d slots, want 3 (head, gap, trailing): %#v", len(box.Slots), box.Slots)
	}
	gapBox, ok := box.BoxAt(1).(*engine.EmptyBox)
	if !ok {
		t.Fatalf("slot 1 = %T, want *EmptyBox", box.BoxAt(1))
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
// while building tables: a line's measured width used to leave out the
// inter-word gaps its drawing inserted, so Bounds() under-reported a
// multi-word line's true width by one space-width per gap. Invisible for ordinary paragraphs (nothing
// else sits flush against their measured edge), but exactly what caused
// table cells to overlap the next column once their measured width was
// used to position it.
func TestLineBoxBoundsIncludesInterWordSpacing(t *testing.T) {
	ctx := engine.Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: stylingtest.Basic()}
	widthOf := func(source string) int {
		t.Helper()
		doc := Parse([]byte(source))
		para := unwrap(doc.root.Blocks[0]).(*engine.TextBlock)
		return para.GetBlockLayout(ctx, engine.NaturalWidthMeasure).Bounds().Dx()
	}

	widthA := widthOf("A")
	widthB := widthOf("B")
	widthAB := widthOf("A B")

	if widthAB <= widthA+widthB {
		t.Errorf(`width("A B") = %d, want > width("A")+width("B") = %d+%d=%d (no inter-word gap counted)`,
			widthAB, widthA, widthB, widthA+widthB)
	}
}

// TestWrapLinesNaturalWidthFits pins down a second bug found while
// building tables, alongside the inter-word-spacing one: the line-wrapping
// check compared bounds.Max.X directly against width, but a later
// word's own bounds can pull bounds.Min.X away from 0 (e.g. a small
// left-side bearing), making Max.X alone wider than the line's true span
// (Dx()). So a line built at exactly its own measured natural width -
// which should always fit on one line, by definition - would still wrap
// its last word. This specific sentence reliably drifts Min.X to 1.
func TestWrapLinesNaturalWidthFits(t *testing.T) {
	ctx := engine.Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: stylingtest.Basic()}
	doc := Parse([]byte("Fast, cheap, and easy to set up with minimal configuration required"))
	para := unwrap(doc.root.Blocks[0]).(*engine.TextBlock)

	natWidth := para.GetBlockLayout(ctx, engine.NaturalWidthMeasure).Bounds().Dx()
	box := para.GetBlockLayout(ctx, natWidth).(*engine.StackBox)
	if len(box.Slots) != 1 {
		t.Errorf("built at its own natural width (%d), got %d lines, want 1", natWidth, len(box.Slots))
	}
}

// TestParseAdjacentCodeSpanGluedFlags checks that "a(`b`)" - no source
// whitespace around the code span - marks the code span's word and the
// trailing ")" as glued to what comes before them (see InlineLayout.
// Glued and compiler.pendingSpace), so no gap gets rendered and
// neither boundary can become a line break.
func TestParseAdjacentCodeSpanGluedFlags(t *testing.T) {
	doc := Parse([]byte("a(`b`)"))
	para := unwrap(doc.root.Blocks[0]).(*engine.TextBlock)
	got := textOf(t, para.Parts)
	want := []string{"a(", "b", ")"}
	if !stringsEqual(got, want) {
		t.Fatalf("words = %v, want %v", got, want)
	}
	if para.Parts[0].(*engine.InlineText).Glued {
		t.Errorf("parts[0] (%q) glued = true, want false (nothing precedes it)", got[0])
	}
	if !para.Parts[1].(*engine.InlineText).Glued {
		t.Errorf("parts[1] (%q) glued = false, want true (no source space before the code span)", got[1])
	}
	if !para.Parts[2].(*engine.InlineText).Glued {
		t.Errorf("parts[2] (%q) glued = false, want true (no source space before it)", got[2])
	}
}

// TestParseNoGapAroundAdjacentCodeSpan is the layout-level counterpart of
// TestParseAdjacentCodeSpanGluedFlags: no source whitespace means no
// added gap, so "a(`b`)" must measure narrower than "a( `b` )" - contrast
// TestLineBoxBoundsIncludesInterWordSpacing, which checks the opposite
// (a real source space DOES add a gap).
func TestParseNoGapAroundAdjacentCodeSpan(t *testing.T) {
	ctx := engine.Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: stylingtest.Basic()}
	widthOf := func(source string) int {
		t.Helper()
		doc := Parse([]byte(source))
		para := unwrap(doc.root.Blocks[0]).(*engine.TextBlock)
		return para.GetBlockLayout(ctx, engine.NaturalWidthMeasure).Bounds().Dx()
	}

	adjacent := widthOf("a(`b`)")
	spaced := widthOf("a( `b` )")
	if adjacent >= spaced {
		t.Errorf("width(%q) = %d, want < width(%q) = %d (no source whitespace, so no added gap)",
			"a(`b`)", adjacent, "a( `b` )", spaced)
	}
}

// TestParseAdjacentPunctuationStaysOnOneLine checks that "(**bold**)" -
// open paren directly against Strong, Strong directly against close
// paren, no source space anywhere - can't be split across a line break,
// even at a width far too narrow for it to fit.
func TestParseAdjacentPunctuationStaysOnOneLine(t *testing.T) {
	ctx := engine.Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: stylingtest.Basic()}
	doc := Parse([]byte("(**bold**)"))
	para := unwrap(doc.root.Blocks[0]).(*engine.TextBlock)

	box := para.GetBlockLayout(ctx, 1).(*engine.StackBox)
	if len(box.Slots) != 1 {
		t.Errorf("built at width 1, got %d lines, want 1 (no breakable boundary anywhere in \"(**bold**)\")", len(box.Slots))
	}
}

// TestParseSpaceBetweenNonTextSiblings checks that a source space
// between two non-text inline nodes ("**a** *b*") - which goldmark gives
// its own whitespace-only Text node, producing zero Inline items on its
// own (confirmed directly against goldmark v2) - still results in a
// normal, breakable space between "a" and "b". This is what needs
// compiler.pendingSpace to carry across appendString calls,
// rather than each call only looking at its own string's edges.
func TestParseSpaceBetweenNonTextSiblings(t *testing.T) {
	doc := Parse([]byte("**a** *b*"))
	para := unwrap(doc.root.Blocks[0]).(*engine.TextBlock)
	got := textOf(t, para.Parts)
	want := []string{"a", "b"}
	if !stringsEqual(got, want) {
		t.Fatalf("words = %v, want %v", got, want)
	}
	if para.Parts[1].(*engine.InlineText).Glued {
		t.Errorf("second word glued = true, want false (real source space between them)")
	}
}

// TestParseSoftLineBreakIsASpace is a regression test: a soft line break
// (a plain newline inside a paragraph, e.g. "laid\nout") is never part of
// either surrounding Text node's own Value - goldmark represents it
// purely as a SoftLineBreak flag on the first one, with the newline
// itself excluded from both segments (confirmed directly against
// goldmark v2) - so it must still be treated as a normal breakable
// space, not silently glue "laid" and "out" together into "laidout".
func TestParseSoftLineBreakIsASpace(t *testing.T) {
	// "out there", not "out, so" - a trailing comma would also split off
	// its own Inline item once the Typographer extension is enabled (see
	// TestParseTypographerDashes and friends), which is beside this
	// test's point.
	doc := Parse([]byte("ever laid\nout there"))
	para := unwrap(doc.root.Blocks[0]).(*engine.TextBlock)
	got := textOf(t, para.Parts)
	want := []string{"ever", "laid", "out", "there"}
	if !stringsEqual(got, want) {
		t.Fatalf("words = %v, want %v", got, want)
	}
	if para.Parts[2].(*engine.InlineText).Glued {
		t.Errorf("%q glued = true, want false (a soft line break is a space, not adjacency)", got[2])
	}
}

// TestParseNonBreakingSpace checks that a literal NBSP (U+00A0) or an
// &nbsp; entity - goldmark normalizes both to the same rune, with no
// AST-level distinction from an ordinary space - becomes its own atomic
// item: same rendered width as an ordinary space, but glued on both
// sides so it can never itself, or its neighbor, end up at a line break.
func TestParseNonBreakingSpace(t *testing.T) {
	ctx := engine.Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: stylingtest.Basic()}
	widthOf := func(source string) int {
		t.Helper()
		doc := Parse([]byte(source))
		para := unwrap(doc.root.Blocks[0]).(*engine.TextBlock)
		return para.GetBlockLayout(ctx, engine.NaturalWidthMeasure).Bounds().Dx()
	}
	spaceWidth := widthOf("a b")

	for _, source := range []string{"a\u00a0b", "a&nbsp;b"} {
		t.Run(source, func(t *testing.T) {
			doc := Parse([]byte(source))
			para := unwrap(doc.root.Blocks[0]).(*engine.TextBlock)
			if len(para.Parts) != 3 {
				t.Fatalf("parts = %#v, want 3 (\"a\", nbsp, \"b\")", para.Parts)
			}
			a, nb, b := para.Parts[0].(*engine.InlineText), para.Parts[1].(*engine.InlineText), para.Parts[2].(*engine.InlineText)
			if a.Text != "a" || nb.Text != "\u00a0" || b.Text != "b" {
				t.Fatalf("texts = %q, %q, %q, want \"a\", \"\\u00a0\", \"b\"", a.Text, nb.Text, b.Text)
			}
			if !nb.Glued {
				t.Errorf("nbsp glued = false, want true (no break before it)")
			}
			if !b.Glued {
				t.Errorf("%q glued = false, want true (no break between it and the nbsp)", b.Text)
			}

			if got := widthOf(source); got != spaceWidth {
				t.Errorf("width(%q) = %d, want %d (same as an ordinary space, %q)", source, got, spaceWidth, "a b")
			}

			box := para.GetBlockLayout(ctx, 1).(*engine.StackBox)
			if len(box.Slots) != 1 {
				t.Errorf("built at width 1, got %d lines, want 1 (nbsp boundaries can't break)", len(box.Slots))
			}
		})
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
			got := engine.ResolveColumnWidths(tc.natural, tc.available)
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
	stack := doc.root
	outer := unwrap(stack.Blocks[0]).(*engine.StackBlock)
	head, trailing := listItemParts(t, outer.Blocks[0])
	if len(head.Parts) != 0 {
		t.Errorf("parts = %#v, want none", head.Parts)
	}
	nestedList, ok := unwrap(trailing).(*engine.StackBlock)
	if !ok || len(nestedList.Blocks) != 1 {
		t.Fatalf("trailing = %#v, want a 1-item StackBlock", trailing)
	}
}

// TestParseLooseList checks that a loose list (items separated by a blank
// line) no longer panics, and that each item's own leading text picks up
// real paragraph margins instead of a tight item's zero margins.
func TestParseLooseList(t *testing.T) {
	ctx := engine.Context{Styles: stylingtest.Basic()}
	doc := Parse([]byte("- one\n\n- two"))
	stack := doc.root
	list, ok := unwrap(stack.Blocks[0]).(*engine.StackBlock)
	if !ok || len(list.Blocks) != 2 {
		t.Fatalf("list = %#v, want a 2-item StackBlock", stack.Blocks[0])
	}

	wantWords := []string{"one", "two"}
	for i, item := range list.Blocks {
		itemStack, ok := unwrap(item).(*engine.StackBlock)
		if !ok || len(itemStack.Blocks) != 1 {
			t.Fatalf("item %d = %#v, want a 1-block StackBlock (head only)", i, item)
		}
		headBlock := itemStack.Blocks[0]
		head, ok := unwrap(headBlock).(*engine.ListItemHeadBlock)
		if !ok {
			t.Fatalf("item %d head = %T, want *ListItemHeadBlock", i, headBlock)
		}
		if got := textOf(t, head.Parts); !stringsEqual(got, []string{wantWords[i]}) {
			t.Errorf("item %d words = %v, want [%s]", i, got, wantWords[i])
		}
		if got := headBlock.Margins(ctx); got != (engine.Margins{Top: 10, Bottom: 10}) {
			t.Errorf("item %d head margins = %+v, want {Top: 10, Bottom: 10} (paragraphStyle)", i, got)
		}
	}
}

// TestParseLooseListMultiParagraphItem checks that a loose item's second
// paragraph gets real paragraph margins via the same generic
// trailing-block path already used for a nested list - no special-casing
// needed.
func TestParseLooseListMultiParagraphItem(t *testing.T) {
	ctx := engine.Context{Styles: stylingtest.Basic()}
	doc := Parse([]byte("- first paragraph\n\n  second paragraph\n"))
	stack := doc.root
	list, ok := unwrap(stack.Blocks[0]).(*engine.StackBlock)
	if !ok || len(list.Blocks) != 1 {
		t.Fatalf("list = %#v, want a 1-item StackBlock", stack.Blocks[0])
	}

	itemStack, ok := unwrap(list.Blocks[0]).(*engine.StackBlock)
	if !ok || len(itemStack.Blocks) != 2 {
		t.Fatalf("item = %#v, want a 2-block StackBlock (head, second paragraph)", list.Blocks[0])
	}

	head, ok := unwrap(itemStack.Blocks[0]).(*engine.ListItemHeadBlock)
	if !ok {
		t.Fatalf("item block 0 = %T, want *ListItemHeadBlock", itemStack.Blocks[0])
	}
	if got := textOf(t, head.Parts); !stringsEqual(got, []string{"first", "paragraph"}) {
		t.Errorf("head words = %v, want [first paragraph]", got)
	}

	second, ok := unwrap(itemStack.Blocks[1]).(*engine.TextBlock)
	if !ok {
		t.Fatalf("item block 1 = %T, want *TextBlock", itemStack.Blocks[1])
	}
	if got := textOf(t, second.Parts); !stringsEqual(got, []string{"second", "paragraph"}) {
		t.Errorf("second paragraph words = %v, want [second paragraph]", got)
	}
	if got := itemStack.Blocks[1].Margins(ctx); got != (engine.Margins{Top: 10, Bottom: 10}) {
		t.Errorf("second paragraph margins = %+v, want {Top: 10, Bottom: 10}", got)
	}
}

func TestParseFencedCodeBlock(t *testing.T) {
	doc := Parse([]byte("```\nline one\nline two\n```"))
	stack := doc.root
	code, ok := unwrap(stack.Blocks[0]).(*engine.CodeBlock)
	if !ok {
		t.Fatalf("block = %T, want *CodeBlock", stack.Blocks[0])
	}
	if len(code.Lines) != 2 {
		t.Fatalf("got %d lines, want 2: %#v", len(code.Lines), code.Lines)
	}
	want := []string{"line one\n", "line two\n"}
	for i, line := range code.Lines {
		text, ok := line[0].(*engine.InlineText)
		if !ok || text.Text != want[i] {
			t.Errorf("line %d = %#v, want %q", i, line, want[i])
		}
	}
}

func TestParseIndentedCodeBlock(t *testing.T) {
	doc := Parse([]byte("    line one\n    line two"))
	stack := doc.root
	code, ok := unwrap(stack.Blocks[0]).(*engine.CodeBlock)
	if !ok {
		t.Fatalf("block = %T, want *CodeBlock", stack.Blocks[0])
	}
	want := []string{"line one\n", "line two\n"}
	if len(code.Lines) != len(want) {
		t.Fatalf("got %d lines, want %d: %#v", len(code.Lines), len(want), code.Lines)
	}
	for i, line := range code.Lines {
		text, ok := line[0].(*engine.InlineText)
		if !ok || text.Text != want[i] {
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
	stack := doc.root
	code, ok := unwrap(stack.Blocks[0]).(*engine.CodeBlock)
	if !ok {
		t.Fatalf("block = %T, want *CodeBlock", stack.Blocks[0])
	}
	if len(code.Lines) != 1 {
		t.Fatalf("got %d lines, want 1: %#v", len(code.Lines), code.Lines)
	}
	text, ok := code.Lines[0][0].(*engine.InlineText)
	if !ok {
		t.Fatalf("line = %T, want *InlineText", code.Lines[0])
	}
	want := codeBlockTabExpansion + "indented\n"
	if text.Text != want {
		t.Errorf("text = %q, want %q", text.Text, want)
	}
}

// TestParseWithSyntaxHighlighter checks that Parse threads a
// WithSyntaxHighlighter option into the KindCodeBlock case, producing
// classified child nodes for spans the Highlighter labels.
func TestParseWithSyntaxHighlighter(t *testing.T) {
	h := fakeHighlighter{highlight: func(language, code string) []HighlightSpan {
		if language != "go" {
			t.Errorf("language = %q, want %q", language, "go")
		}
		return []HighlightSpan{
			{Text: "func", Class: TokenKeyword},
			{Text: " f()", Class: TokenPlain},
		}
	}}
	doc := Parse([]byte("```go\nfunc f()\n```"), WithSyntaxHighlighter(h))
	stack := doc.root
	code, ok := unwrap(stack.Blocks[0]).(*engine.CodeBlock)
	if !ok {
		t.Fatalf("block = %T, want *CodeBlock", stack.Blocks[0])
	}
	if len(code.Lines) != 1 || len(code.Lines[0]) != 2 {
		t.Fatalf("lines = %#v, want one line with 2 parts", code.Lines)
	}
	keyword, ok := code.Lines[0][0].(*engine.InlineText)
	if !ok || keyword.Text != "func" {
		t.Fatalf("lines[0][0] = %#v, want InlineText %q", code.Lines[0][0], "func")
	}
	if keyword.ASTNode.Tag != ast.TagCodeKeyword {
		t.Errorf("keyword node tag = %v, want ast.TagCodeKeyword", keyword.ASTNode.Tag)
	}
	plain, ok := code.Lines[0][1].(*engine.InlineText)
	if !ok || plain.Text != " f()" {
		t.Fatalf("lines[0][1] = %#v, want InlineText %q", code.Lines[0][1], " f()")
	}
	if plain.ASTNode != code.ASTNode {
		t.Errorf("plain node = %v, want the block's own node", plain.ASTNode)
	}
}

// fakeCodeBlockPlugin is a CodeBlockPlugin test double - handles is the
// set of languages CanHandle accepts; canHandleCalls counts how many
// times CanHandle actually ran, for tests checking the compiler's
// per-language caching (see pluginFor).
type fakeCodeBlockPlugin struct {
	name           string
	handles        map[string]bool
	canHandleCalls *int
	image          func(language, code string) images.AsyncImage
}

func (p fakeCodeBlockPlugin) CanHandle(language string) bool {
	if p.canHandleCalls != nil {
		*p.canHandleCalls++
	}
	return p.handles[language]
}

func (p fakeCodeBlockPlugin) Image(language, code string) images.AsyncImage {
	return p.image(language, code)
}

// TestParseWithCodeBlockPlugin checks that a fenced code block in a
// language the plugin handles compiles via NewDiagramBlock instead of a
// plain CodeBlock - with the plugin's own images.AsyncImage, and a fallback
// that's exactly what today's highlighter/plain-text rendering would
// have produced.
func TestParseWithCodeBlockPlugin(t *testing.T) {
	wantImg := images.AsyncImage{Key: "diagram-key"}
	plugin := fakeCodeBlockPlugin{
		handles: map[string]bool{"mermaid": true},
		image: func(language, code string) images.AsyncImage {
			if language != "mermaid" || code != "graph TD; A-->B;\n" {
				t.Errorf("Image(%q, %q) called, want (\"mermaid\", \"graph TD; A-->B;\\n\")", language, code)
			}
			return wantImg
		},
	}
	doc := Parse([]byte("```mermaid\ngraph TD; A-->B;\n```"), WithCodeBlockPlugin(plugin))
	stack := doc.root
	diagram, ok := unwrap(stack.Blocks[0]).(*engine.DiagramBlock)
	if !ok {
		t.Fatalf("block = %T, want *diagramBlock", unwrap(stack.Blocks[0]))
	}
	if diagram.Img.Key != wantImg.Key {
		t.Errorf("img.Key = %q, want the plugin's own %q", diagram.Img.Key, wantImg.Key)
	}
	fallback, ok := diagram.Fallback.(*engine.CodeBlock)
	if !ok {
		t.Fatalf("fallback = %T, want *CodeBlock", diagram.Fallback)
	}
	if len(fallback.Lines) != 1 {
		t.Fatalf("fallback lines = %#v, want 1 line", fallback.Lines)
	}
	text, ok := fallback.Lines[0][0].(*engine.InlineText)
	if !ok || text.Text != "graph TD; A-->B;\n" {
		t.Errorf("fallback line = %#v, want the raw source text", fallback.Lines[0])
	}
}

// TestParseCodeBlockPluginFallsThroughForUnrecognizedLanguage checks
// that a language no registered plugin handles compiles exactly as it
// would with no plugin configured at all.
func TestParseCodeBlockPluginFallsThroughForUnrecognizedLanguage(t *testing.T) {
	plugin := fakeCodeBlockPlugin{handles: map[string]bool{"mermaid": true}}
	doc := Parse([]byte("```go\nfunc f()\n```"), WithCodeBlockPlugin(plugin))
	stack := doc.root
	if _, ok := unwrap(stack.Blocks[0]).(*engine.CodeBlock); !ok {
		t.Fatalf("block = %T, want *CodeBlock (plugin shouldn't have handled \"go\")", unwrap(stack.Blocks[0]))
	}
}

// TestParseCodeBlockPluginRegistrationOrderAndCaching checks that with
// several plugins registered, the first (in registration order) whose
// CanHandle matches wins, and that CanHandle is called at most once per
// plugin per distinct language, however many fences share that language.
func TestParseCodeBlockPluginRegistrationOrderAndCaching(t *testing.T) {
	var firstCalls, secondCalls int
	first := fakeCodeBlockPlugin{
		handles:        map[string]bool{"mermaid": false},
		canHandleCalls: &firstCalls,
	}
	second := fakeCodeBlockPlugin{
		handles:        map[string]bool{"mermaid": true},
		canHandleCalls: &secondCalls,
		image:          func(language, code string) images.AsyncImage { return images.AsyncImage{Key: "k"} },
	}
	source := []byte("```mermaid\na\n```\n\n```mermaid\nb\n```\n\n```mermaid\nc\n```")
	doc := Parse(source, WithCodeBlockPlugin(first), WithCodeBlockPlugin(second))
	stack := doc.root
	if len(stack.Blocks) != 3 {
		t.Fatalf("got %d top-level blocks, want 3", len(stack.Blocks))
	}
	for i, b := range stack.Blocks {
		if _, ok := unwrap(b).(*engine.DiagramBlock); !ok {
			t.Errorf("block %d = %T, want *diagramBlock (second plugin should have handled it)", i, unwrap(b))
		}
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Errorf("CanHandle calls = first:%d second:%d, want 1 each (cached after the first \"mermaid\" fence)", firstCalls, secondCalls)
	}
}

func TestParseThematicBreak(t *testing.T) {
	doc := Parse([]byte("---"))
	stack := doc.root
	wrapper, ok := stack.Blocks[0].(*engine.MarginBlock)
	if !ok {
		t.Fatalf("block = %T, want *MarginBlock", stack.Blocks[0])
	}
	rule, ok := wrapper.Block.(*engine.ThematicBreakBlock)
	if !ok {
		t.Fatalf("wrapper.Block = %T, want *ThematicBreakBlock", wrapper.Block)
	}

	styleSheet := stylingtest.Basic()
	ctx := engine.Context{Scale: 1, Styles: styleSheet}
	if got := wrapper.Margins(ctx); got != (engine.Margins{Top: 20, Bottom: 20}) {
		t.Errorf("Margins(ctx) = %+v, want {Top: 20, Bottom: 20}", got)
	}
	box := rule.GetBlockLayout(ctx, 100)
	ruleBox, ok := box.(*engine.RuleBox)
	if !ok {
		t.Fatalf("GetBlockLayout = %T, want *RuleBox", box)
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
	stack := doc.root
	bq, ok := unwrap(stack.Blocks[0]).(*engine.BlockquoteBlock)
	if !ok {
		t.Fatalf("block = %T, want *BlockquoteBlock", stack.Blocks[0])
	}
	para, ok := unwrap(bq.Inner).(*engine.TextBlock)
	if !ok {
		t.Fatalf("inner = %T, want *TextBlock", bq.Inner)
	}
	got := textOf(t, para.Parts)
	want := []string{"quoted", "text"}
	if !stringsEqual(got, want) {
		t.Errorf("words = %v, want %v", got, want)
	}
}

// TestParseBlockquoteMultipleBlocks checks that a blockquote spanning more
// than one block wraps them in a StackBlock, unlike the single-block case.
func TestParseBlockquoteMultipleBlocks(t *testing.T) {
	doc := Parse([]byte("> first\n>\n> second"))
	stack := doc.root
	bq := unwrap(stack.Blocks[0]).(*engine.BlockquoteBlock)
	inner, ok := bq.Inner.(*engine.StackBlock)
	if !ok || len(inner.Blocks) != 2 {
		t.Fatalf("inner = %#v, want a 2-block StackBlock", bq.Inner)
	}
}

// TestParseNestedBlockquote checks that a blockquote inside a blockquote
// (`> > ...`) compiles recursively - each level gets its own bar when
// drawn, with no special-casing needed since BlockquoteBlock positions
// itself rather than relying on its parent.
func TestParseNestedBlockquote(t *testing.T) {
	doc := Parse([]byte("> > nested quote"))
	stack := doc.root
	outer := unwrap(stack.Blocks[0]).(*engine.BlockquoteBlock)
	inner, ok := unwrap(outer.Inner).(*engine.BlockquoteBlock)
	if !ok {
		t.Fatalf("inner = %T, want *BlockquoteBlock", outer.Inner)
	}
	para, ok := unwrap(inner.Inner).(*engine.TextBlock)
	if !ok {
		t.Fatalf("innermost = %T, want *TextBlock", inner.Inner)
	}
	got := textOf(t, para.Parts)
	want := []string{"nested", "quote"}
	if !stringsEqual(got, want) {
		t.Errorf("words = %v, want %v", got, want)
	}
}

func TestBlockquoteBoxIndent(t *testing.T) {
	bq := &engine.BlockquoteBlock{
		Inner: &fixedHeightBlock{height: 10},
	}
	styleSheet := stylingtest.Basic()
	ctx := engine.Context{Scale: 1, Styles: styleSheet}
	box := bq.GetBlockLayout(ctx, 100)
	bqBox, ok := box.(*engine.BlockquoteBox)
	if !ok {
		t.Fatalf("GetBlockLayout = %T, want *BlockquoteBox", box)
	}
	blockquoteIndent := int(styleSheet.BlockquoteGeometry(nil).Indent)
	if bqBox.Indent != blockquoteIndent {
		t.Errorf("indent = %d, want %d", bqBox.Indent, blockquoteIndent)
	}
	wantInnerWidth := 100 - blockquoteIndent
	if got := bqBox.Inner.Bounds().Dx(); got != wantInnerWidth {
		t.Errorf("inner width = %d, want %d", got, wantInnerWidth)
	}
	want := image.Rect(0, 0, 100, 10)
	if got := bqBox.Bounds(); got != want {
		t.Errorf("Bounds() = %v, want %v", got, want)
	}
}

func TestParseLink(t *testing.T) {
	doc := Parse([]byte(`[click *here*](https://example.com "a title")`))
	stack := doc.root
	para := unwrap(stack.Blocks[0]).(*engine.TextBlock)
	got := textOf(t, para.Parts)
	want := []string{"click", "here"}
	if !stringsEqual(got, want) {
		t.Fatalf("words = %v, want %v", got, want)
	}
	ctx := engine.Context{Styles: stylingtest.Basic()}
	for i, part := range para.Parts {
		text := part.(*engine.InlineText)
		if ctx.ResolvedColor(text.ASTNode) == color.White {
			t.Errorf("part %d color = white, want link color", i)
		}
	}
	// Nested emphasis inside the link text should still apply on top of
	// the link's color.
	if style := ctx.ResolvedTextStyle(para.Parts[1].(*engine.InlineText).ASTNode); style.Style != font.StyleItalic {
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
	ctx := engine.Context{Styles: stylingtest.Basic()}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := Parse([]byte(tc.source))
			stack := doc.root
			para := unwrap(stack.Blocks[0]).(*engine.TextBlock)
			if len(para.Parts) != 1 {
				t.Fatalf("parts = %#v, want 1 part", para.Parts)
			}
			text := para.Parts[0].(*engine.InlineText)
			if text.Text != tc.want {
				t.Errorf("text = %q, want %q", text.Text, tc.want)
			}
			if ctx.ResolvedColor(text.ASTNode) == color.White {
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
	ctx := engine.Context{Styles: stylingtest.Basic()}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := Parse([]byte(tc.source))
			stack := doc.root
			para := unwrap(stack.Blocks[0]).(*engine.TextBlock)
			if len(para.Parts) != 1 {
				t.Fatalf("parts = %#v, want 1 part", para.Parts)
			}
			text := para.Parts[0].(*engine.InlineText)
			if text.Text != "word" {
				t.Errorf("text = %q, want %q", text.Text, "word")
			}
			style := ctx.ResolvedTextStyle(text.ASTNode)
			if style.Style != tc.wantStyle || style.Weight != tc.wantWeight {
				t.Errorf("style = %+v, want Style=%v Weight=%v", style, tc.wantStyle, tc.wantWeight)
			}
		})
	}
}

func TestParseCodeSpan(t *testing.T) {
	doc := Parse([]byte("see `code` here"))
	stack := doc.root
	para := unwrap(stack.Blocks[0]).(*engine.TextBlock)
	got := textOf(t, para.Parts)
	want := []string{"see", "code", "here"}
	if !stringsEqual(got, want) {
		t.Fatalf("words = %v, want %v", got, want)
	}
	ctx := engine.Context{Styles: stylingtest.Basic()}
	code := para.Parts[1].(*engine.InlineText)
	if style := ctx.ResolvedTextStyle(code.ASTNode); style.Family != fonts.Monospace {
		t.Errorf("code span family = %v, want fonts.Monospace", style.Family)
	}
}

// TestParseStrikethrough checks that being struck composes with nested
// styling (bold from Strong here) rather than replacing it - struck-ness
// is a structural ancestry fact (HasAncestorTag), resolved independently
// of TextStyle.
func TestParseStrikethrough(t *testing.T) {
	doc := Parse([]byte("plain ~~struck **and bold**~~ text"))
	stack := doc.root
	para := unwrap(stack.Blocks[0]).(*engine.TextBlock)
	got := textOf(t, para.Parts)
	want := []string{"plain", "struck", "and", "bold", "text"}
	if !stringsEqual(got, want) {
		t.Fatalf("words = %v, want %v", got, want)
	}

	plain := para.Parts[0].(*engine.InlineText)
	if plain.ASTNode.HasAncestorTag(ast.TagStrikethrough) {
		t.Errorf("part %q: struck = true, want false", plain.Text)
	}

	struckWords := para.Parts[1:4]
	for _, part := range struckWords {
		text := part.(*engine.InlineText)
		if !text.ASTNode.HasAncestorTag(ast.TagStrikethrough) {
			t.Errorf("part %q: struck = false, want true", text.Text)
		}
	}
	ctx := engine.Context{Styles: stylingtest.Basic()}
	bold := para.Parts[3].(*engine.InlineText)
	if style := ctx.ResolvedTextStyle(bold.ASTNode); style.Weight != font.WeightBold {
		t.Errorf("part %q: weight = %v, want bold", bold.Text, style.Weight)
	}

	trailing := para.Parts[4].(*engine.InlineText)
	if trailing.ASTNode.HasAncestorTag(ast.TagStrikethrough) {
		t.Errorf("part %q: struck = true, want false", trailing.Text)
	}
}

func TestInlineTextStrikeThickness(t *testing.T) {
	styleSheet := stylingtest.Basic()
	ctx := engine.Context{Scale: 2, FaceSelector: fonts.NewGoSelector(), Styles: styleSheet}

	plainNode := (*ast.Node)(nil).AddChild(ast.TagParagraph).AddChild(ast.TagEmphasis)
	plain := (&engine.InlineText{Text: "x", ASTNode: plainNode}).GetInlineLayout(ctx, engine.NaturalWidthMeasure).(*engine.TextBox)
	if plain.StrikeThickness != 0 {
		t.Errorf("non-struck StrikeThickness = %d, want 0", plain.StrikeThickness)
	}

	struckNode := (*ast.Node)(nil).AddChild(ast.TagParagraph).AddChild(ast.TagStrikethrough)
	struck := (&engine.InlineText{Text: "x", ASTNode: struckNode}).GetInlineLayout(ctx, engine.NaturalWidthMeasure).(*engine.TextBox)
	want := int(styleSheet.StrikeThickness(struckNode) * ctx.Scale)
	if struck.StrikeThickness != want {
		t.Errorf("struck StrikeThickness = %d, want %d", struck.StrikeThickness, want)
	}
}

func TestParseImageWithTitle(t *testing.T) {
	doc := Parse([]byte(`![alt](cat.jpeg "a lovely cat")`))
	stack := doc.root
	para := unwrap(stack.Blocks[0]).(*engine.TextBlock)
	if len(para.Parts) != 1 {
		t.Fatalf("parts = %#v, want 1 part", para.Parts)
	}
	img, ok := para.Parts[0].(*engine.InlineImage)
	if !ok {
		t.Fatalf("part = %T, want *InlineImage", para.Parts[0])
	}
	if img.Src != "cat.jpeg" {
		t.Errorf("src = %q, want %q", img.Src, "cat.jpeg")
	}
	if img.Alt != "alt" {
		t.Errorf("alt = %q, want %q", img.Alt, "alt")
	}
	if img.Title != "a lovely cat" {
		t.Errorf("title = %q, want %q", img.Title, "a lovely cat")
	}
	if img.ASTNode.Tag != ast.TagImage {
		t.Errorf("node.Tag = %v, want ast.TagImage", img.ASTNode.Tag)
	}
	if img.FallbackNode == nil || img.FallbackNode.Tag != ast.TagUnsupported {
		t.Errorf("fallbackNode = %#v, want a ast.TagUnsupported node", img.FallbackNode)
	}
	if img.FallbackNode.Parent != img.ASTNode {
		t.Errorf("fallbackNode.Parent = %#v, want img.node", img.FallbackNode.Parent)
	}
}

// TestParseImageAltTextFlattensMarkup checks alt text is captured even
// when it contains nested inline markup - CommonMark allows arbitrary
// inline content in an image's description, flattened to plain text
// the same way HTML rendering flattens it into an alt attribute.
func TestParseImageAltTextFlattensMarkup(t *testing.T) {
	doc := Parse([]byte("![a *b* c](x.png)"))
	stack := doc.root
	para := unwrap(stack.Blocks[0]).(*engine.TextBlock)
	img := para.Parts[0].(*engine.InlineImage)
	if img.Alt != "a b c" {
		t.Errorf("alt = %q, want %q", img.Alt, "a b c")
	}
}

// TestImageGetInlineLayoutWithoutImageCache checks that with no
// image cache the image isn't loaded at all: it renders as its alt text,
// with nothing pending to revisit.
func TestImageGetInlineLayoutWithoutImageCache(t *testing.T) {
	img := &engine.InlineImage{Src: "testdata/cat.jpeg", Alt: "a cat"}
	ctx := engine.Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: stylingtest.Basic()}

	box, ok := img.GetInlineLayout(ctx, engine.NaturalWidthMeasure).(*engine.TextBox)
	if !ok {
		t.Fatalf("GetInlineLayout returned %T, want *TextBox", img.GetInlineLayout(ctx, engine.NaturalWidthMeasure))
	}
	if box.Text != "a cat" {
		t.Errorf("Text = %q, want the alt text %q", box.Text, "a cat")
	}
	if pending := box.PendingImages(); len(pending) != 0 {
		t.Errorf("PendingImages() = %v, want none", pending)
	}
}

// TestImageGetInlineLayoutScalesBounds checks an image's layout bounds
// are its native pixel size times ctx.Scale, like every other sized
// quantity in the layout system (engine.Context.ScaledMargins and
// friends) - not the file's raw pixel size unconditionally, which would
// leave images pixel-locked against zoom/DPI scale.
func TestImageGetInlineLayoutScalesBounds(t *testing.T) {
	img := &engine.InlineImage{Src: "testdata/cat.jpeg"} // 400x600
	// FaceSelector/Styles: the first call sees the fetch still pending
	// and falls back to text ("(loading image…)"), which needs both.
	ctx := engine.Context{
		Scale:        2,
		ImageCache:   imagecache.NewCache(images.FileSource{}),
		FaceSelector: fonts.NewGoSelector(),
		Styles:       stylingtest.Basic(),
	}
	img.GetInlineLayout(ctx, engine.NaturalWidthMeasure)
	waitForSettled(t, ctx.ImageCache, img.Src)
	box, ok := img.GetInlineLayout(ctx, engine.NaturalWidthMeasure).(*engine.ImageBox)
	if !ok {
		t.Fatalf("GetInlineLayout returned %T, want *ImageBox", img.GetInlineLayout(ctx, engine.NaturalWidthMeasure))
	}
	want := image.Rect(0, 0, 800, 1200)
	if box.Rect != want {
		t.Errorf("bounds = %v, want %v", box.Rect, want)
	}
	if box.Img == nil {
		t.Error("img = nil, want the decoded image")
	}
}

// TestImageGetInlineLayoutFitsWidth checks an image whose ctx.Scale-d
// size would still exceed the width it's given is scaled down further,
// preserving aspect ratio (see fitWidth) - CSS's max-width: 100%,
// applied to an inline image, so a document author's own image at its
// native resolution never overflows the page.
func TestImageGetInlineLayoutFitsWidth(t *testing.T) {
	img := &engine.InlineImage{Src: "testdata/cat.jpeg"} // 400x600
	ctx := engine.Context{
		Scale:        1,
		ImageCache:   imagecache.NewCache(images.FileSource{}),
		FaceSelector: fonts.NewGoSelector(),
		Styles:       stylingtest.Basic(),
	}
	img.GetInlineLayout(ctx, 200)
	waitForSettled(t, ctx.ImageCache, img.Src)
	box, ok := img.GetInlineLayout(ctx, 200).(*engine.ImageBox)
	if !ok {
		t.Fatalf("GetInlineLayout returned %T, want *ImageBox", img.GetInlineLayout(ctx, 200))
	}
	want := image.Rect(0, 0, 200, 300)
	if box.Rect != want {
		t.Errorf("bounds = %v, want %v", box.Rect, want)
	}
}

func TestFitWidth(t *testing.T) {
	for _, tc := range []struct {
		name  string
		r     image.Rectangle
		width int
		want  image.Rectangle
	}{
		{"already fits", image.Rect(0, 0, 100, 50), 200, image.Rect(0, 0, 100, 50)},
		{"exactly fits", image.Rect(0, 0, 200, 50), 200, image.Rect(0, 0, 200, 50)},
		{"too wide, scaled down", image.Rect(0, 0, 400, 600), 200, image.Rect(0, 0, 200, 300)},
		{"unbounded width", image.Rect(0, 0, 400, 600), 0, image.Rect(0, 0, 400, 600)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := engine.FitWidth(tc.r, tc.width); got != tc.want {
				t.Errorf("fitWidth(%v, %d) = %v, want %v", tc.r, tc.width, got, tc.want)
			}
		})
	}
}

// TestImageGetInlineLayoutFallsBackWhenMissing checks a missing/unreadable
// image renders as flagged fallback text (see appendUnsupportedInline)
// instead of a silent zero-size gap, preferring alt text over title over
// a generic message naming the resolved (here: unresolved-any-further,
// since FileImageLoader doesn't resolve) source.
func TestImageGetInlineLayoutFallsBackWhenMissing(t *testing.T) {
	styleSheet := stylingtest.Basic()
	ctx := engine.Context{
		Scale:        1,
		FaceSelector: fonts.NewGoSelector(),
		Styles:       styleSheet,
		ImageCache:   imagecache.NewCache(images.FileSource{}),
	}
	fallbackNode := (*ast.Node)(nil).AddChild(ast.TagImage).AddChild(ast.TagUnsupported)

	// All three cases below share the src "nope.png" (only alt/title
	// differ), so waiting once here - before any of them look at the
	// result - is enough: the rest hit the already-settled cache entry
	// directly.
	ctx.ImageCache.Load("nope.png")
	waitForSettled(t, ctx.ImageCache, "nope.png")

	for _, tc := range []struct {
		name string
		img  *engine.InlineImage
		want string
	}{
		{"alt wins", &engine.InlineImage{Src: "nope.png", Alt: "a lovely cat", Title: "title", FallbackNode: fallbackNode}, "a lovely cat"},
		{"title when no alt", &engine.InlineImage{Src: "nope.png", Title: "a lovely cat", FallbackNode: fallbackNode}, "a lovely cat"},
		{"generic message when neither", &engine.InlineImage{Src: "nope.png", FallbackNode: fallbackNode}, "(image not found: nope.png)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			box, ok := tc.img.GetInlineLayout(ctx, engine.NaturalWidthMeasure).(*engine.TextBox)
			if !ok {
				t.Fatalf("GetInlineLayout returned %T, want *TextBox", tc.img.GetInlineLayout(ctx, engine.NaturalWidthMeasure))
			}
			if box.Text != tc.want {
				t.Errorf("Text = %q, want %q", box.Text, tc.want)
			}
			if box.Color != styleSheet.UnsupportedColor {
				t.Errorf("Color = %v, want UnsupportedColor %v", box.Color, styleSheet.UnsupportedColor)
			}
		})
	}
}

// TestImageGetInlineLayoutAnimated checks that an animated GIF src
// produces an ImageBox with anim set (not img), with bounds scaled
// from the animation's own (shared, per-frame) size.
func TestImageGetInlineLayoutAnimated(t *testing.T) {
	img := &engine.InlineImage{Src: "testdata/animated.gif"} // 64x64
	ctx := engine.Context{
		Scale:        2,
		ImageCache:   imagecache.NewCache(images.FileSource{}),
		FaceSelector: fonts.NewGoSelector(),
		Styles:       stylingtest.Basic(),
	}
	img.GetInlineLayout(ctx, engine.NaturalWidthMeasure)
	waitForSettled(t, ctx.ImageCache, img.Src)

	box, ok := img.GetInlineLayout(ctx, engine.NaturalWidthMeasure).(*engine.ImageBox)
	if !ok {
		t.Fatalf("GetInlineLayout returned %T, want *ImageBox", img.GetInlineLayout(ctx, engine.NaturalWidthMeasure))
	}
	if box.Anim == nil {
		t.Fatal("anim = nil, want the decoded imagecache.Animation")
	}
	if box.Img != nil {
		t.Errorf("img = %v, want nil for an animated GIF", box.Img)
	}
	want := image.Rect(0, 0, 128, 128) // 64x64 native * scale 2
	if box.Rect != want {
		t.Errorf("bounds = %v, want %v", box.Rect, want)
	}
}

// TestParseResolvesEntitiesAndEscapes checks the v2 migration's behavior
// change noted in the migration plan: text values are resolved (entity
// references and backslash escapes decoded), unlike v1's raw Text().
func TestParseResolvesEntitiesAndEscapes(t *testing.T) {
	doc := Parse([]byte(`Fish \& chips and &amp; and \*literal\*`))
	stack := doc.root
	para := unwrap(stack.Blocks[0]).(*engine.TextBlock)
	got := textOf(t, para.Parts)
	want := []string{"Fish", "&", "chips", "and", "&", "and", "*literal*"}
	if !stringsEqual(got, want) {
		t.Errorf("words = %v, want %v", got, want)
	}
}

// TestParseTypographerSmartQuotes checks that straight quotes become curly
// ones - goldmark's extension.TypographerParser substitutes its default
// HTML entities (e.g. "&rsquo;"), but Text.Value's decoder resolves those
// the same way it already resolves &amp;/&nbsp; in ordinary prose (see
// TestParseResolvesEntitiesAndEscapes), so the literal curly rune comes out
// with no extra config.
func TestParseTypographerSmartQuotes(t *testing.T) {
	doc := Parse([]byte(`"hello" and 'world'`))
	para := unwrap(doc.root.Blocks[0]).(*engine.TextBlock)
	got := textOf(t, para.Parts)
	want := []string{"“", "hello", "”", "and", "‘", "world", "’"}
	if !stringsEqual(got, want) {
		t.Fatalf("words = %v, want %v", got, want)
	}
}

// TestParseTypographerApostropheGluedToWord is the adjacency case the
// README's Typographer feature entry used to call out as the blocker:
// goldmark gives "Alice's" as three siblings with no whitespace between
// them - Text("Alice"), a synthetic Text("’") for the substituted
// apostrophe, Text("s book") - so without InlineLayout.Glued tracking
// (see project_inline_adjacency_spacing) this would render as "Alice ’ s
// book" and could wrap mid-word. Checks both the glued flags and that it
// can't be split across a line break even at a width that would force a
// wrap if the boundaries were (wrongly) breakable.
func TestParseTypographerApostropheGluedToWord(t *testing.T) {
	ctx := engine.Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: stylingtest.Basic()}
	doc := Parse([]byte("Alice's book"))
	para := unwrap(doc.root.Blocks[0]).(*engine.TextBlock)
	got := textOf(t, para.Parts)
	want := []string{"Alice", "’", "s", "book"}
	if !stringsEqual(got, want) {
		t.Fatalf("words = %v, want %v", got, want)
	}
	if para.Parts[0].(*engine.InlineText).Glued {
		t.Errorf("parts[0] (%q) glued = true, want false (nothing precedes it)", got[0])
	}
	for i := 1; i <= 2; i++ {
		if !para.Parts[i].(*engine.InlineText).Glued {
			t.Errorf("parts[%d] (%q) glued = false, want true (no source space around the substituted apostrophe)", i, got[i])
		}
	}
	if para.Parts[3].(*engine.InlineText).Glued {
		t.Errorf("parts[3] (%q) glued = true, want false (real source space before it)", got[3])
	}

	box := para.GetBlockLayout(ctx, 1).(*engine.StackBox)
	if len(box.Slots) != 2 {
		t.Errorf("built at width 1, got %d lines, want 2 (\"Alice’s\" stays together, \"book\" is the only breakable boundary)", len(box.Slots))
	}
}

// TestParseTypographerDashes checks that an unspaced "---"/"--" becomes a
// glued em/en dash (no source whitespace, so no rendered gap and no line
// break, same mechanism as the apostrophe case), while a spaced "--"
// becomes an ordinary breakable en dash - CommonMark/Markdown convention
// distinguishes the two by whether the author put spaces around it.
func TestParseTypographerDashes(t *testing.T) {
	cases := []struct {
		name        string
		source      string
		want        []string
		wantGlued   []bool
		description string
	}{
		{
			name:      "unspaced em dash",
			source:    "word---word",
			want:      []string{"word", "—", "word"},
			wantGlued: []bool{false, true, true},
		},
		{
			name:      "spaced en dash",
			source:    "word -- word",
			want:      []string{"word", "–", "word"},
			wantGlued: []bool{false, false, false},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := Parse([]byte(tc.source))
			para := unwrap(doc.root.Blocks[0]).(*engine.TextBlock)
			got := textOf(t, para.Parts)
			if !stringsEqual(got, tc.want) {
				t.Fatalf("words = %v, want %v", got, tc.want)
			}
			for i, want := range tc.wantGlued {
				if glued := para.Parts[i].(*engine.InlineText).Glued; glued != want {
					t.Errorf("parts[%d] (%q) glued = %v, want %v", i, got[i], glued, want)
				}
			}
		})
	}
}

// TestParseTypographerEllipsis checks the third substitution kind (besides
// quotes and dashes) - "..." becomes a single "…" glued to the word before
// it, same mechanism as the others.
func TestParseTypographerEllipsis(t *testing.T) {
	doc := Parse([]byte("wait..."))
	para := unwrap(doc.root.Blocks[0]).(*engine.TextBlock)
	got := textOf(t, para.Parts)
	want := []string{"wait", "…"}
	if !stringsEqual(got, want) {
		t.Fatalf("words = %v, want %v", got, want)
	}
	if !para.Parts[1].(*engine.InlineText).Glued {
		t.Errorf("parts[1] (%q) glued = false, want true (no source space before it)", got[1])
	}
}
