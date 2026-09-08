package whynot

import (
	"image/color"
	"testing"

	"golang.org/x/image/font"
)

// These pin DefaultStyleSheet's values against whynot's previous
// hardcoded config in MarkdownCompiler's old Parse implementation.

func TestDefaultStyleSheetMargins(t *testing.T) {
	s := NewDarkStyleSheet()
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
	s := NewDarkStyleSheet()
	if got := s.Margins(nil); got != (Margins{}) {
		t.Errorf("Margins(nil) = %+v, want zero value", got)
	}
}

// TestDefaultStyleSheetTextStyle pins each tag's own contribution - the
// Set mask matters as much as the values, since an unset field is meant
// to be inherited from an ancestor (see TestResolvedTextStyle) rather
// than read as its zero value.
func TestDefaultStyleSheetTextStyle(t *testing.T) {
	s := NewDarkStyleSheet()
	cases := []struct {
		tag  ASTTag
		want PartialTextStyle
	}{
		{TagParagraph, PartialTextStyle{TextStyle{Size: 16}, FieldSize}},
		{TagHeading1, PartialTextStyle{TextStyle{Size: 40, Weight: font.WeightBold, Family: Proportional}, FieldSize | FieldWeight | FieldFamily}},
		{TagHeading2, PartialTextStyle{TextStyle{Size: 36, Weight: font.WeightBold, Family: Proportional}, FieldSize | FieldWeight | FieldFamily}},
		{TagHeading6, PartialTextStyle{TextStyle{Size: 20, Weight: font.WeightBold, Family: Proportional}, FieldSize | FieldWeight | FieldFamily}},
		{TagListItem, PartialTextStyle{TextStyle{Size: 16}, FieldSize}},
		{TagCodeBlock, PartialTextStyle{TextStyle{Size: 16, Family: Monospace}, FieldSize | FieldFamily}},
		{TagTableCell, PartialTextStyle{TextStyle{Size: 16}, FieldSize}},
		{TagCodeSpan, PartialTextStyle{TextStyle{Family: Monospace}, FieldFamily}},
		{TagEmphasis, PartialTextStyle{TextStyle{Style: font.StyleItalic}, FieldStyle}},
		{TagStrong, PartialTextStyle{TextStyle{Weight: font.WeightBold}, FieldWeight}},
		{TagLink, PartialTextStyle{}},
	}
	for _, tc := range cases {
		node := &ASTNode{Tag: tc.tag}
		if got := s.TextStyle(node); got != tc.want {
			t.Errorf("TextStyle(tag=%v) = %+v, want %+v", tc.tag, got, tc.want)
		}
	}
}

// TestDefaultStyleSheetTextStyleNilNode checks that the root claims every
// field via BaseTextStyle - the fallback ResolvedTextStyle reaches if no
// real ancestor ever set some field.
func TestDefaultStyleSheetTextStyleNilNode(t *testing.T) {
	s := NewDarkStyleSheet()
	want := PartialTextStyle{TextStyle{Size: 16}, allTextStyleFields}
	if got := s.TextStyle(nil); got != want {
		t.Errorf("TextStyle(nil) = %+v, want %+v", got, want)
	}
}

// TestDefaultStyleSheetColor checks the cascading text color: only tags
// with a real opinion (code, link) return non-nil - everything else,
// including the block types with their own BorderColor, returns nil so
// ResolvedColor's ancestry walk passes through them.
func TestDefaultStyleSheetColor(t *testing.T) {
	s := NewDarkStyleSheet()
	cases := []struct {
		tag  ASTTag
		want color.Color
	}{
		{TagCodeBlock, color.RGBA{0xFF, 0xFF, 0x80, 0xFF}},
		{TagCodeSpan, color.RGBA{0xFF, 0xFF, 0x80, 0xFF}},
		{TagLink, color.RGBA{0x66, 0xB2, 0xFF, 0xFF}},
		{TagParagraph, nil},
		{TagHeading1, nil},
		{TagBlockquote, nil},
		{TagTable, nil},
		{TagThematicBreak, nil},
	}
	for _, tc := range cases {
		node := &ASTNode{Tag: tc.tag}
		if got := s.Color(node); got != tc.want {
			t.Errorf("Color(tag=%v) = %v, want %v", tc.tag, got, tc.want)
		}
	}
}

func TestDefaultStyleSheetColorNilNode(t *testing.T) {
	s := NewDarkStyleSheet()
	if got := s.Color(nil); got != color.White {
		t.Errorf("Color(nil) = %v, want %v", got, color.White)
	}
}

// TestDefaultStyleSheetBorderColor checks the non-cascading decoration
// color - a thematic break's rule, a blockquote's bar, a table's frame -
// resolved directly against exactly the tag it decorates, unlike Color.
func TestDefaultStyleSheetBorderColor(t *testing.T) {
	s := NewDarkStyleSheet()
	cases := []struct {
		tag  ASTTag
		want color.Color
	}{
		{TagThematicBreak, color.RGBA{0x80, 0x80, 0x80, 0xFF}},
		{TagBlockquote, color.RGBA{0x80, 0x80, 0x80, 0xFF}},
		{TagTable, color.RGBA{0x80, 0x80, 0x80, 0xFF}},
		{TagParagraph, nil},
		{TagLink, nil},
	}
	for _, tc := range cases {
		node := &ASTNode{Tag: tc.tag}
		if got := s.BorderColor(node); got != tc.want {
			t.Errorf("BorderColor(tag=%v) = %v, want %v", tc.tag, got, tc.want)
		}
	}
}

func TestDefaultStyleSheetBorderColorNilNode(t *testing.T) {
	s := NewDarkStyleSheet()
	if got := s.BorderColor(nil); got != nil {
		t.Errorf("BorderColor(nil) = %v, want nil", got)
	}
}

// TestDefaultStyleSheetStrikeThickness checks that the thickness doubles
// as the "is this struck at all" signal: 0 unless node or an ancestor
// carries TagStrikethrough.
func TestDefaultStyleSheetStrikeThickness(t *testing.T) {
	s := NewDarkStyleSheet()

	if got := s.StrikeThickness(nil); got != 0 {
		t.Errorf("StrikeThickness(nil) = %v, want 0", got)
	}

	notStruck := &ASTNode{Tag: TagEmphasis}
	if got := s.StrikeThickness(notStruck); got != 0 {
		t.Errorf("StrikeThickness(Emphasis) = %v, want 0", got)
	}

	struck := &ASTNode{Tag: TagStrikethrough}
	if got := s.StrikeThickness(struck); got != 1 {
		t.Errorf("StrikeThickness(Strikethrough) = %v, want 1", got)
	}

	nestedInStruck := struck.AddChild(TagEmphasis)
	if got := s.StrikeThickness(nestedInStruck); got != 1 {
		t.Errorf("StrikeThickness(Emphasis nested in Strikethrough) = %v, want 1", got)
	}
}

func TestDefaultStyleSheetDimensions(t *testing.T) {
	s := NewDarkStyleSheet()

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

// TestNewLightStyleSheetColors checks that the light theme actually
// inverts the parts that need to be readable against a light background
// (background itself, default text, links, code) rather than just being
// NewDarkStyleSheet under a different name.
func TestNewLightStyleSheetColors(t *testing.T) {
	dark := NewDarkStyleSheet()
	light := NewLightStyleSheet()

	if light.BackgroundColor == dark.BackgroundColor {
		t.Errorf("light BackgroundColor = dark's (%v), want a light background", light.BackgroundColor)
	}
	if light.TextColor == dark.TextColor {
		t.Errorf("light TextColor = dark's (%v), want a dark foreground", light.TextColor)
	}
	if light.LinkColor == dark.LinkColor {
		t.Errorf("light LinkColor = dark's (%v), want a color readable on a light background", light.LinkColor)
	}
	if light.CodeColor == dark.CodeColor {
		t.Errorf("light CodeColor = dark's (%v), want a color readable on a light background", light.CodeColor)
	}
}

// TestNewLightStyleSheetSharesNonColorValues checks that margins, sizes,
// weights, and dimensional constants aren't duplicated/drifted between
// the two themes - only color should differ.
func TestNewLightStyleSheetSharesNonColorValues(t *testing.T) {
	dark := NewDarkStyleSheet()
	light := NewLightStyleSheet()

	if light.ParagraphMargins != dark.ParagraphMargins {
		t.Errorf("ParagraphMargins = %+v, want dark's %+v", light.ParagraphMargins, dark.ParagraphMargins)
	}
	if light.HeadingTextStyles != dark.HeadingTextStyles {
		t.Errorf("HeadingTextStyles = %+v, want dark's %+v", light.HeadingTextStyles, dark.HeadingTextStyles)
	}
	struckNode := &ASTNode{Tag: TagStrikethrough}
	if light.StrikeThickness(struckNode) != dark.StrikeThickness(struckNode) {
		t.Errorf("StrikeThickness value differs from dark's")
	}
	if light.TableGeometry(nil) != dark.TableGeometry(nil) {
		t.Errorf("TableGeometry = %+v, want dark's %+v", light.TableGeometry(nil), dark.TableGeometry(nil))
	}
}
