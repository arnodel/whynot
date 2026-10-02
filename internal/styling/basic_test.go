package styling_test

import (
	"image/color"
	"testing"

	"golang.org/x/image/font"

	"github.com/arnodel/whynot/codeblocks"
	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/styling"
	"github.com/arnodel/whynot/internal/styling/stylingtest"
)

// These pin styling.Basic's values against whynot's previous
// hardcoded config in the compiler's old Parse implementation.

func TestBasicMargins(t *testing.T) {
	s := stylingtest.Basic()
	cases := []struct {
		tag  ast.Tag
		want styling.Margins
	}{
		{ast.TagParagraph, styling.Margins{Top: 10, Bottom: 10}},
		{ast.TagHeading1, styling.Margins{Top: 30, Bottom: 10}},
		{ast.TagHeading2, styling.Margins{Top: 26, Bottom: 10}},
		{ast.TagHeading3, styling.Margins{Top: 22, Bottom: 10}},
		{ast.TagHeading4, styling.Margins{Top: 18, Bottom: 10}},
		{ast.TagHeading5, styling.Margins{Top: 14, Bottom: 10}},
		{ast.TagHeading6, styling.Margins{Top: 10, Bottom: 10}},
		{ast.TagList, styling.Margins{Top: 10, Bottom: 10}},
		{ast.TagListItem, styling.Margins{Top: 5, Bottom: 5, Left: 40}},
		{ast.TagCodeBlock, styling.Margins{Top: 20, Bottom: 20, Left: 20}},
		{ast.TagThematicBreak, styling.Margins{Top: 20, Bottom: 20}},
		{ast.TagBlockquote, styling.Margins{Top: 10, Bottom: 10}},
		{ast.TagTable, styling.Margins{Top: 10, Bottom: 10}},
		{ast.TagTableCell, styling.Margins{}},
		{ast.TagLink, styling.Margins{}},
	}
	for _, tc := range cases {
		node := &ast.Node{Tag: tc.tag}
		if got := s.Margins(node); got != tc.want {
			t.Errorf("Margins(tag=%v) = %+v, want %+v", tc.tag, got, tc.want)
		}
	}
}

func TestBasicMarginsNilNode(t *testing.T) {
	s := stylingtest.Basic()
	if got := s.Margins(nil); got != (styling.Margins{}) {
		t.Errorf("Margins(nil) = %+v, want zero value", got)
	}
}

// TestBasicTextStyle pins each tag's own contribution - the
// Set mask matters as much as the values, since an unset field is meant
// to be inherited from an ancestor (see TestResolvedTextStyle) rather
// than read as its zero value.
func TestBasicTextStyle(t *testing.T) {
	s := stylingtest.Basic()
	cases := []struct {
		tag  ast.Tag
		want styling.PartialTextStyle
	}{
		{ast.TagParagraph, styling.PartialTextStyle{fonts.TextStyle{Size: 16}, styling.FieldSize}},
		{ast.TagHeading1, styling.PartialTextStyle{fonts.TextStyle{Size: 40, Weight: font.WeightBold, Family: fonts.Proportional}, styling.FieldSize | styling.FieldWeight | styling.FieldFamily}},
		{ast.TagHeading2, styling.PartialTextStyle{fonts.TextStyle{Size: 36, Weight: font.WeightBold, Family: fonts.Proportional}, styling.FieldSize | styling.FieldWeight | styling.FieldFamily}},
		{ast.TagHeading6, styling.PartialTextStyle{fonts.TextStyle{Size: 20, Weight: font.WeightBold, Family: fonts.Proportional}, styling.FieldSize | styling.FieldWeight | styling.FieldFamily}},
		{ast.TagListItem, styling.PartialTextStyle{fonts.TextStyle{Size: 16}, styling.FieldSize}},
		{ast.TagCodeBlock, styling.PartialTextStyle{fonts.TextStyle{Size: 16, Family: fonts.Monospace}, styling.FieldSize | styling.FieldFamily}},
		{ast.TagTableCell, styling.PartialTextStyle{fonts.TextStyle{Size: 16}, styling.FieldSize}},
		{ast.TagCodeSpan, styling.PartialTextStyle{fonts.TextStyle{Family: fonts.Monospace}, styling.FieldFamily}},
		{ast.TagEmphasis, styling.PartialTextStyle{fonts.TextStyle{Style: font.StyleItalic}, styling.FieldStyle}},
		{ast.TagStrong, styling.PartialTextStyle{fonts.TextStyle{Weight: font.WeightBold}, styling.FieldWeight}},
		{ast.TagLink, styling.PartialTextStyle{}},
	}
	for _, tc := range cases {
		node := &ast.Node{Tag: tc.tag}
		if got := s.TextStyle(node); got != tc.want {
			t.Errorf("TextStyle(tag=%v) = %+v, want %+v", tc.tag, got, tc.want)
		}
	}
}

// TestBasicTextStyleNilNode checks that the root claims every
// field via BaseTextStyle - the fallback ResolvedTextStyle reaches if no
// real ancestor ever set some field.
func TestBasicTextStyleNilNode(t *testing.T) {
	s := stylingtest.Basic()
	want := styling.PartialTextStyle{fonts.TextStyle{Size: 16}, styling.AllTextStyleFields}
	if got := s.TextStyle(nil); got != want {
		t.Errorf("TextStyle(nil) = %+v, want %+v", got, want)
	}
}

// TestBasicColor checks the cascading text color: only tags
// with a real opinion (code, link) return non-nil - everything else,
// including the block types with their own BorderColor, returns nil so
// ResolvedColor's ancestry walk passes through them.
func TestBasicColor(t *testing.T) {
	s := stylingtest.Basic()
	cases := []struct {
		tag   ast.Tag
		class string
		want  color.Color
	}{
		{ast.TagCodeBlock, "", color.RGBA{0xD4, 0xD4, 0xD4, 0xFF}},
		{ast.TagCodeSpan, "", color.RGBA{0xFF, 0xFF, 0x80, 0xFF}},
		{ast.TagLink, "", color.RGBA{0x66, 0xB2, 0xFF, 0xFF}},
		{ast.TagCodeToken, codeblocks.ClassKeyword, color.RGBA{0xC5, 0x86, 0xF2, 0xFF}},
		{ast.TagCodeToken, codeblocks.ClassType, color.RGBA{0x4E, 0xC9, 0xB0, 0xFF}},
		{ast.TagCodeToken, codeblocks.ClassFunction, color.RGBA{0xDC, 0xDC, 0xAA, 0xFF}},
		{ast.TagCodeToken, codeblocks.ClassString, color.RGBA{0x9E, 0xD9, 0x7A, 0xFF}},
		{ast.TagCodeToken, codeblocks.ClassNumber, color.RGBA{0xF2, 0xB0, 0x66, 0xFF}},
		{ast.TagCodeToken, codeblocks.ClassComment, color.RGBA{0x80, 0x80, 0x80, 0xFF}},
		// An unknown class has no opinion: it shows in its block's color.
		{ast.TagCodeToken, "decorator", nil},
		{ast.TagParagraph, "", nil},
		{ast.TagHeading1, "", nil},
		{ast.TagBlockquote, "", nil},
		{ast.TagTable, "", nil},
		{ast.TagThematicBreak, "", nil},
	}
	for _, tc := range cases {
		node := &ast.Node{Tag: tc.tag, Class: tc.class}
		if got := s.Color(node); got != tc.want {
			t.Errorf("Color(tag=%v, class=%q) = %v, want %v", tc.tag, tc.class, got, tc.want)
		}
	}
}

func TestBasicColorNilNode(t *testing.T) {
	s := stylingtest.Basic()
	if got := s.Color(nil); got != color.White {
		t.Errorf("Color(nil) = %v, want %v", got, color.White)
	}
}

// TestBasicBorderColor checks the non-cascading decoration
// color - a thematic break's rule, a blockquote's bar, a table's frame -
// resolved directly against exactly the tag it decorates, unlike Color.
func TestBasicBorderColor(t *testing.T) {
	s := stylingtest.Basic()
	cases := []struct {
		tag  ast.Tag
		want color.Color
	}{
		{ast.TagThematicBreak, color.RGBA{0x80, 0x80, 0x80, 0xFF}},
		{ast.TagBlockquote, color.RGBA{0x80, 0x80, 0x80, 0xFF}},
		{ast.TagTable, color.RGBA{0x80, 0x80, 0x80, 0xFF}},
		{ast.TagParagraph, nil},
		{ast.TagLink, nil},
	}
	for _, tc := range cases {
		node := &ast.Node{Tag: tc.tag}
		if got := s.BorderColor(node); got != tc.want {
			t.Errorf("BorderColor(tag=%v) = %v, want %v", tc.tag, got, tc.want)
		}
	}
}

func TestBasicBorderColorNilNode(t *testing.T) {
	s := stylingtest.Basic()
	if got := s.BorderColor(nil); got != nil {
		t.Errorf("BorderColor(nil) = %v, want nil", got)
	}
}

// TestBasicStrikeThickness checks that the thickness doubles
// as the "is this struck at all" signal: 0 unless node or an ancestor
// carries ast.TagStrikethrough.
func TestBasicStrikeThickness(t *testing.T) {
	s := stylingtest.Basic()

	if got := s.StrikeThickness(nil); got != 0 {
		t.Errorf("StrikeThickness(nil) = %v, want 0", got)
	}

	notStruck := &ast.Node{Tag: ast.TagEmphasis}
	if got := s.StrikeThickness(notStruck); got != 0 {
		t.Errorf("StrikeThickness(Emphasis) = %v, want 0", got)
	}

	struck := &ast.Node{Tag: ast.TagStrikethrough}
	if got := s.StrikeThickness(struck); got != 1 {
		t.Errorf("StrikeThickness(Strikethrough) = %v, want 1", got)
	}

	nestedInStruck := struck.AddChild(ast.TagEmphasis)
	if got := s.StrikeThickness(nestedInStruck); got != 1 {
		t.Errorf("StrikeThickness(Emphasis nested in Strikethrough) = %v, want 1", got)
	}
}

func TestBasicDimensions(t *testing.T) {
	s := stylingtest.Basic()

	if got := s.ThematicBreakThickness(nil); got != 2 {
		t.Errorf("ThematicBreakThickness() = %v, want 2", got)
	}

	wantBQ := styling.BlockquoteGeometry{Indent: 16, BarWidth: 3}
	if got := s.BlockquoteGeometry(nil); got != wantBQ {
		t.Errorf("BlockquoteGeometry() = %+v, want %+v", got, wantBQ)
	}

	wantTable := styling.TableGeometry{FrameThickness: 2, ColumnGap: 12, RowGap: 6, HeaderGap: 4, ColumnRuleThickness: 1}
	if got := s.TableGeometry(nil); got != wantTable {
		t.Errorf("TableGeometry() = %+v, want %+v", got, wantTable)
	}

	if got := s.LineHeight(nil); got != 1.2 {
		t.Errorf("LineHeight() = %v, want 1.2", got)
	}
}

// TestBasicScrollbarColor checks that ScrollbarColor picks the right one of the three Scrollbar
// colors for each combination of hover/pressed.
func TestBasicScrollbarColor(t *testing.T) {
	cases := []struct {
		name           string
		sheet          *styling.Basic
		hover, pressed bool
		want           color.Color
	}{
		{"idle", stylingtest.Basic(), false, false, color.RGBA{0x80, 0x80, 0x80, 0xA0}},
		{"hover", stylingtest.Basic(), true, false, color.RGBA{0xA0, 0xA0, 0xA0, 0xC0}},
		{"pressed", stylingtest.Basic(), true, true, color.RGBA{0xC0, 0xC0, 0xC0, 0xE0}},
		{"pressed without hover", stylingtest.Basic(), false, true, color.RGBA{0xC0, 0xC0, 0xC0, 0xE0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.sheet.ScrollbarColor(c.hover, c.pressed); got != c.want {
				t.Errorf("ScrollbarColor(%v, %v) = %v, want %v", c.hover, c.pressed, got, c.want)
			}
		})
	}
}
