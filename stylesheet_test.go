package whynot

import (
	"image/color"
	"testing"

	"golang.org/x/image/font"
)

// These pin DefaultStyleSheet's values against MarkdownCompiler's current
// hardcoded config in Parse - nothing consumes DefaultStyleSheet yet, so
// this is the only thing keeping the two in sync until a later migration
// step replaces Parse's literals with this type directly.

func TestDefaultStyleSheetMargins(t *testing.T) {
	s := NewDefaultStyleSheet()
	cases := []struct {
		tag  ASTTag
		want Margins
	}{
		{TagParagraph, Margins{Top: 10, Bottom: 10}},
		{TagHeading1, Margins{Top: 30, Bottom: 10}},
		{TagHeading2, Margins{Top: 26, Bottom: 10}},
		{TagHeading3, Margins{Top: 22, Bottom: 10}},
		{TagHeading4, Margins{Top: 18, Bottom: 10}},
		{TagHeading5, Margins{Top: 14, Bottom: 10}},
		{TagHeading6, Margins{Top: 10, Bottom: 10}},
		{TagList, Margins{Top: 10, Bottom: 10}},
		{TagListItem, Margins{Top: 5, Bottom: 5, Left: 40}},
		{TagCodeBlock, Margins{Top: 20, Bottom: 20, Left: 20}},
		{TagThematicBreak, Margins{Top: 20, Bottom: 20}},
		{TagBlockquote, Margins{Top: 10, Bottom: 10}},
		{TagTable, Margins{Top: 10, Bottom: 10}},
		{TagTableCell, Margins{}},
		{TagLink, Margins{}},
	}
	for _, tc := range cases {
		node := &ASTNode{Tag: tc.tag}
		if got := s.Margins(node); got != tc.want {
			t.Errorf("Margins(tag=%v) = %+v, want %+v", tc.tag, got, tc.want)
		}
	}
}

func TestDefaultStyleSheetMarginsNilNode(t *testing.T) {
	s := NewDefaultStyleSheet()
	if got := s.Margins(nil); got != (Margins{}) {
		t.Errorf("Margins(nil) = %+v, want zero value", got)
	}
}

func TestDefaultStyleSheetTextStyle(t *testing.T) {
	s := NewDefaultStyleSheet()
	cases := []struct {
		tag  ASTTag
		want TextStyle
	}{
		{TagParagraph, TextStyle{Size: 16}},
		{TagHeading1, TextStyle{Size: 40, Weight: font.WeightBold, Family: SmallCaps}},
		{TagHeading2, TextStyle{Size: 36, Weight: font.WeightBold, Family: Proportional}},
		{TagHeading6, TextStyle{Size: 20, Weight: font.WeightBold, Family: Proportional}},
		{TagListItem, TextStyle{Size: 16}},
		{TagCodeBlock, TextStyle{Size: 16, Family: Monospace}},
		{TagTableCell, TextStyle{Size: 16}},
		{TagCodeSpan, TextStyle{Family: Monospace}},
		{TagEmphasis, TextStyle{Style: font.StyleItalic}},
		{TagStrong, TextStyle{Weight: font.WeightBold}},
		{TagLink, TextStyle{}},
	}
	for _, tc := range cases {
		node := &ASTNode{Tag: tc.tag}
		if got := s.TextStyle(node); got != tc.want {
			t.Errorf("TextStyle(tag=%v) = %+v, want %+v", tc.tag, got, tc.want)
		}
	}
}

func TestDefaultStyleSheetColor(t *testing.T) {
	s := NewDefaultStyleSheet()
	cases := []struct {
		tag  ASTTag
		want color.Color
	}{
		{TagCodeBlock, color.RGBA{0xFF, 0xFF, 0x80, 0xFF}},
		{TagCodeSpan, color.RGBA{0xFF, 0xFF, 0x80, 0xFF}},
		{TagThematicBreak, color.RGBA{0x80, 0x80, 0x80, 0xFF}},
		{TagLink, color.RGBA{0x66, 0xB2, 0xFF, 0xFF}},
		{TagBlockquote, color.RGBA{0x80, 0x80, 0x80, 0xFF}},
		{TagTable, color.RGBA{0x80, 0x80, 0x80, 0xFF}},
		{TagParagraph, color.White},
		{TagHeading1, color.White},
	}
	for _, tc := range cases {
		node := &ASTNode{Tag: tc.tag}
		if got := s.Color(node); got != tc.want {
			t.Errorf("Color(tag=%v) = %v, want %v", tc.tag, got, tc.want)
		}
	}
}

func TestDefaultStyleSheetColorNilNode(t *testing.T) {
	s := NewDefaultStyleSheet()
	if got := s.Color(nil); got != color.White {
		t.Errorf("Color(nil) = %v, want %v", got, color.White)
	}
}

func TestDefaultStyleSheetDimensions(t *testing.T) {
	s := NewDefaultStyleSheet()

	if got := s.StrikeThickness(nil); got != 1 {
		t.Errorf("StrikeThickness() = %v, want 1", got)
	}
	if got := s.ThematicBreakThickness(nil); got != 2 {
		t.Errorf("ThematicBreakThickness() = %v, want 2", got)
	}

	wantBQ := BlockquoteGeometry{Indent: 16, BarWidth: 3}
	if got := s.BlockquoteGeometry(nil); got != wantBQ {
		t.Errorf("BlockquoteGeometry() = %+v, want %+v", got, wantBQ)
	}

	wantTable := TableGeometry{FrameThickness: 2, ColumnGap: 12, RowGap: 6, HeaderGap: 4, ColumnRuleThickness: 1}
	if got := s.TableGeometry(nil); got != wantTable {
		t.Errorf("TableGeometry() = %+v, want %+v", got, wantTable)
	}
}
