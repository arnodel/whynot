package styling

import (
	"image/color"
	"testing"

	"golang.org/x/image/font"
)

// These pin Basic's values against whynot's previous
// hardcoded config in the compiler's old Parse implementation.

func TestBasicMargins(t *testing.T) {
	s := Dark()
	cases := []struct {
		tag  Tag
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
		node := &Node{Tag: tc.tag}
		if got := s.Margins(node); got != tc.want {
			t.Errorf("Margins(tag=%v) = %+v, want %+v", tc.tag, got, tc.want)
		}
	}
}

func TestBasicMarginsNilNode(t *testing.T) {
	s := Dark()
	if got := s.Margins(nil); got != (Margins{}) {
		t.Errorf("Margins(nil) = %+v, want zero value", got)
	}
}

// TestBasicTextStyle pins each tag's own contribution - the
// Set mask matters as much as the values, since an unset field is meant
// to be inherited from an ancestor (see TestResolvedTextStyle) rather
// than read as its zero value.
func TestBasicTextStyle(t *testing.T) {
	s := Dark()
	cases := []struct {
		tag  Tag
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
		node := &Node{Tag: tc.tag}
		if got := s.TextStyle(node); got != tc.want {
			t.Errorf("TextStyle(tag=%v) = %+v, want %+v", tc.tag, got, tc.want)
		}
	}
}

// TestBasicTextStyleNilNode checks that the root claims every
// field via BaseTextStyle - the fallback ResolvedTextStyle reaches if no
// real ancestor ever set some field.
func TestBasicTextStyleNilNode(t *testing.T) {
	s := Dark()
	want := PartialTextStyle{TextStyle{Size: 16}, AllTextStyleFields}
	if got := s.TextStyle(nil); got != want {
		t.Errorf("TextStyle(nil) = %+v, want %+v", got, want)
	}
}

// TestBasicColor checks the cascading text color: only tags
// with a real opinion (code, link) return non-nil - everything else,
// including the block types with their own BorderColor, returns nil so
// ResolvedColor's ancestry walk passes through them.
func TestBasicColor(t *testing.T) {
	s := Dark()
	cases := []struct {
		tag  Tag
		want color.Color
	}{
		{TagCodeBlock, color.RGBA{0xD4, 0xD4, 0xD4, 0xFF}},
		{TagCodeSpan, color.RGBA{0xFF, 0xFF, 0x80, 0xFF}},
		{TagLink, color.RGBA{0x66, 0xB2, 0xFF, 0xFF}},
		{TagCodeKeyword, color.RGBA{0xC5, 0x86, 0xF2, 0xFF}},
		{TagCodeType, color.RGBA{0x4E, 0xC9, 0xB0, 0xFF}},
		{TagCodeFunction, color.RGBA{0xDC, 0xDC, 0xAA, 0xFF}},
		{TagCodeString, color.RGBA{0x9E, 0xD9, 0x7A, 0xFF}},
		{TagCodeNumber, color.RGBA{0xF2, 0xB0, 0x66, 0xFF}},
		{TagCodeComment, color.RGBA{0x80, 0x80, 0x80, 0xFF}},
		{TagParagraph, nil},
		{TagHeading1, nil},
		{TagBlockquote, nil},
		{TagTable, nil},
		{TagThematicBreak, nil},
	}
	for _, tc := range cases {
		node := &Node{Tag: tc.tag}
		if got := s.Color(node); got != tc.want {
			t.Errorf("Color(tag=%v) = %v, want %v", tc.tag, got, tc.want)
		}
	}
}

func TestBasicColorNilNode(t *testing.T) {
	s := Dark()
	if got := s.Color(nil); got != color.White {
		t.Errorf("Color(nil) = %v, want %v", got, color.White)
	}
}

// TestBasicBorderColor checks the non-cascading decoration
// color - a thematic break's rule, a blockquote's bar, a table's frame -
// resolved directly against exactly the tag it decorates, unlike Color.
func TestBasicBorderColor(t *testing.T) {
	s := Dark()
	cases := []struct {
		tag  Tag
		want color.Color
	}{
		{TagThematicBreak, color.RGBA{0x80, 0x80, 0x80, 0xFF}},
		{TagBlockquote, color.RGBA{0x80, 0x80, 0x80, 0xFF}},
		{TagTable, color.RGBA{0x80, 0x80, 0x80, 0xFF}},
		{TagParagraph, nil},
		{TagLink, nil},
	}
	for _, tc := range cases {
		node := &Node{Tag: tc.tag}
		if got := s.BorderColor(node); got != tc.want {
			t.Errorf("BorderColor(tag=%v) = %v, want %v", tc.tag, got, tc.want)
		}
	}
}

func TestBasicBorderColorNilNode(t *testing.T) {
	s := Dark()
	if got := s.BorderColor(nil); got != nil {
		t.Errorf("BorderColor(nil) = %v, want nil", got)
	}
}

// TestBasicStrikeThickness checks that the thickness doubles
// as the "is this struck at all" signal: 0 unless node or an ancestor
// carries TagStrikethrough.
func TestBasicStrikeThickness(t *testing.T) {
	s := Dark()

	if got := s.StrikeThickness(nil); got != 0 {
		t.Errorf("StrikeThickness(nil) = %v, want 0", got)
	}

	notStruck := &Node{Tag: TagEmphasis}
	if got := s.StrikeThickness(notStruck); got != 0 {
		t.Errorf("StrikeThickness(Emphasis) = %v, want 0", got)
	}

	struck := &Node{Tag: TagStrikethrough}
	if got := s.StrikeThickness(struck); got != 1 {
		t.Errorf("StrikeThickness(Strikethrough) = %v, want 1", got)
	}

	nestedInStruck := struck.AddChild(TagEmphasis)
	if got := s.StrikeThickness(nestedInStruck); got != 1 {
		t.Errorf("StrikeThickness(Emphasis nested in Strikethrough) = %v, want 1", got)
	}
}

func TestBasicDimensions(t *testing.T) {
	s := Dark()

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

	if got := s.LineHeight(nil); got != 1.2 {
		t.Errorf("LineHeight() = %v, want 1.2", got)
	}
}

// TestLightColors checks that the light theme actually
// inverts the parts that need to be readable against a light background
// (background itself, default text, links, code) rather than just being
// Dark under a different name.
func TestLightColors(t *testing.T) {
	dark := Dark()
	light := Light()

	if light.Background == dark.Background {
		t.Errorf("light Background = dark's (%v), want a light background", light.Background)
	}
	if light.TextColor == dark.TextColor {
		t.Errorf("light TextColor = dark's (%v), want a dark foreground", light.TextColor)
	}
	if light.LinkColor == dark.LinkColor {
		t.Errorf("light LinkColor = dark's (%v), want a color readable on a light background", light.LinkColor)
	}
	if light.CodeBlockColor == dark.CodeBlockColor {
		t.Errorf("light CodeBlockColor = dark's (%v), want a color readable on a light background", light.CodeBlockColor)
	}
	if light.CodeSpanColor == dark.CodeSpanColor {
		t.Errorf("light CodeSpanColor = dark's (%v), want a color readable on a light background", light.CodeSpanColor)
	}
	if light.Scrollbar == dark.Scrollbar {
		t.Errorf("light Scrollbar = dark's (%+v), want colors readable on a light background", light.Scrollbar)
	}
	if light.Syntax == dark.Syntax {
		t.Errorf("light Syntax = dark's (%+v), want a palette readable on a light background", light.Syntax)
	}
}

// TestBasicScrollbarColor checks that ScrollbarColor picks the right one of the three Scrollbar
// colors for each combination of hover/pressed.
func TestBasicScrollbarColor(t *testing.T) {
	cases := []struct {
		name           string
		sheet          *Basic
		hover, pressed bool
		want           color.Color
	}{
		{"dark idle", Dark(), false, false, color.RGBA{0x80, 0x80, 0x80, 0xA0}},
		{"dark hover", Dark(), true, false, color.RGBA{0xA0, 0xA0, 0xA0, 0xC0}},
		{"dark pressed", Dark(), true, true, color.RGBA{0xC0, 0xC0, 0xC0, 0xE0}},
		{"dark pressed without hover", Dark(), false, true, color.RGBA{0xC0, 0xC0, 0xC0, 0xE0}},
		{"light idle", Light(), false, false, color.RGBA{0x60, 0x60, 0x60, 0xA0}},
		{"light hover", Light(), true, false, color.RGBA{0x40, 0x40, 0x40, 0xC0}},
		{"light pressed", Light(), true, true, color.RGBA{0x20, 0x20, 0x20, 0xE0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.sheet.ScrollbarColor(c.hover, c.pressed); got != c.want {
				t.Errorf("ScrollbarColor(%v, %v) = %v, want %v", c.hover, c.pressed, got, c.want)
			}
		})
	}
}

// TestLightSharesNonColorValues checks that margins, sizes,
// weights, and dimensional constants aren't duplicated/drifted between
// the two themes - only color should differ.
func TestLightSharesNonColorValues(t *testing.T) {
	dark := Dark()
	light := Light()

	if light.ParagraphMargins != dark.ParagraphMargins {
		t.Errorf("ParagraphMargins = %+v, want dark's %+v", light.ParagraphMargins, dark.ParagraphMargins)
	}
	if light.HeadingTextStyles != dark.HeadingTextStyles {
		t.Errorf("HeadingTextStyles = %+v, want dark's %+v", light.HeadingTextStyles, dark.HeadingTextStyles)
	}
	struckNode := &Node{Tag: TagStrikethrough}
	if light.StrikeThickness(struckNode) != dark.StrikeThickness(struckNode) {
		t.Errorf("StrikeThickness value differs from dark's")
	}
	if light.TableGeometry(nil) != dark.TableGeometry(nil) {
		t.Errorf("TableGeometry = %+v, want dark's %+v", light.TableGeometry(nil), dark.TableGeometry(nil))
	}
}
