package whynot

import (
	"image/color"

	"golang.org/x/image/font"
)

// BlockquoteGeometry is the dimensional constants a blockquote's own bar
// and indent need - grouped since a blockquote's GetBox always reads both
// together.
type BlockquoteGeometry struct {
	Indent   float64
	BarWidth float64
}

// TableGeometry is the dimensional constants a table's layout needs -
// grouped since TableBlock.GetBox always reads all five together.
type TableGeometry struct {
	FrameThickness      float64
	ColumnGap           float64
	RowGap              float64
	HeaderGap           float64
	ColumnRuleThickness float64
}

// StyleSheet resolves an ASTNode's semantic role to concrete appearance -
// margins, text styling, color, and a few dimensional constants. Every
// method takes the node, uniformly, even where nothing in whynot itself
// varies a value by node today (there's only one table's worth of column
// gap, for instance) - an implementation is free to ignore it, but the
// shape leaves room for one that doesn't want to. Not yet consumed
// anywhere - see DefaultStyleSheet for the values MarkdownCompiler
// currently hardcodes, ported here as the migration's starting point.
type StyleSheet interface {
	// Margins returns the margins for node's own tag.
	Margins(node *ASTNode) Margins
	// TextStyle returns the text appearance for node's own tag. Doesn't
	// yet account for ancestry (e.g. Strong nested inside Emphasis) -
	// inline styling is its own, later migration step.
	TextStyle(node *ASTNode) TextStyle
	// Color returns the color for node's own tag.
	Color(node *ASTNode) color.Color

	StrikeThickness(node *ASTNode) float64
	ThematicBreakThickness(node *ASTNode) float64
	BlockquoteGeometry(node *ASTNode) BlockquoteGeometry
	TableGeometry(node *ASTNode) TableGeometry
}

// DefaultStyleSheet is whynot's built-in StyleSheet - the values
// MarkdownCompiler currently hardcodes in Parse, relocated here as the
// starting point for the migration. A caller who wants to change one
// thing can construct one with NewDefaultStyleSheet and mutate a field
// (ParagraphMargins, LinkColor, ...), or embed it in a custom StyleSheet
// and override individual methods for full control.
type DefaultStyleSheet struct {
	ParagraphMargins Margins
	ParagraphSize    float64

	HeadingMargins  [6]Margins
	HeadingSizes    [6]float64
	HeadingWeights  [6]font.Weight
	HeadingFamilies [6]FontFamily

	ListMargins Margins

	ListItemMargins Margins
	ListItemSize    float64

	CodeBlockMargins Margins
	CodeBlockSize    float64
	CodeColor        color.Color

	ThematicBreakMargins Margins
	ThematicBreakColor   color.Color

	LinkColor color.Color

	BlockquoteMargins  Margins
	BlockquoteBarColor color.Color

	TableCellSize   float64
	TableMargins    Margins
	TableFrameColor color.Color

	// TextColor is the default text color for tags with no color of
	// their own (paragraphs, headings, list items, table cells,
	// emphasis, strong, strikethrough).
	TextColor color.Color

	// Dimensional constants. Unexported: unlike the fields above, these
	// aren't the primary customization surface (a game reaches for
	// colors/margins/fonts, rarely a table's own column-gap width) - a
	// caller who does want to change one overrides the method instead,
	// via a custom StyleSheet embedding this one.
	strikeThickness        float64
	thematicBreakThickness float64
	blockquoteGeometry     BlockquoteGeometry
	tableGeometry          TableGeometry
}

var _ StyleSheet = (*DefaultStyleSheet)(nil)

// NewDefaultStyleSheet returns whynot's built-in default appearance - the
// exact values MarkdownCompiler.Parse hardcodes today.
func NewDefaultStyleSheet() *DefaultStyleSheet {
	return &DefaultStyleSheet{
		ParagraphMargins: Margins{Top: 10, Bottom: 10},
		ParagraphSize:    16,

		HeadingMargins: [6]Margins{
			{Top: 30, Bottom: 10},
			{Top: 26, Bottom: 10},
			{Top: 22, Bottom: 10},
			{Top: 18, Bottom: 10},
			{Top: 14, Bottom: 10},
			{Top: 10, Bottom: 10},
		},
		HeadingSizes:    [6]float64{40, 36, 32, 28, 24, 20},
		HeadingWeights:  [6]font.Weight{font.WeightBold, font.WeightBold, font.WeightBold, font.WeightBold, font.WeightBold, font.WeightBold},
		HeadingFamilies: [6]FontFamily{0: SmallCaps}, // levels 2-6 default to Proportional, the zero value

		ListMargins: Margins{Top: 10, Bottom: 10},

		ListItemMargins: Margins{Top: 5, Bottom: 5, Left: 40},
		ListItemSize:    16,

		CodeBlockMargins: Margins{Top: 20, Bottom: 20, Left: 20},
		CodeBlockSize:    16,
		CodeColor:        color.RGBA{0xFF, 0xFF, 0x80, 0xFF},

		ThematicBreakMargins: Margins{Top: 20, Bottom: 20},
		ThematicBreakColor:   color.RGBA{0x80, 0x80, 0x80, 0xFF},

		LinkColor: color.RGBA{0x66, 0xB2, 0xFF, 0xFF},

		BlockquoteMargins:  Margins{Top: 10, Bottom: 10},
		BlockquoteBarColor: color.RGBA{0x80, 0x80, 0x80, 0xFF},

		TableCellSize:   16,
		TableMargins:    Margins{Top: 10, Bottom: 10},
		TableFrameColor: color.RGBA{0x80, 0x80, 0x80, 0xFF},

		TextColor: color.White,

		strikeThickness:        1,
		thematicBreakThickness: 2,
		blockquoteGeometry:     BlockquoteGeometry{Indent: 16, BarWidth: 3},
		tableGeometry: TableGeometry{
			FrameThickness:      2,
			ColumnGap:           12,
			RowGap:              6,
			HeaderGap:           4,
			ColumnRuleThickness: 1,
		},
	}
}

func (s *DefaultStyleSheet) Margins(node *ASTNode) Margins {
	if node == nil {
		return Margins{}
	}
	switch node.Tag {
	case TagParagraph:
		return s.ParagraphMargins
	case TagHeading1, TagHeading2, TagHeading3, TagHeading4, TagHeading5, TagHeading6:
		return s.HeadingMargins[node.Tag-TagHeading1]
	case TagList:
		return s.ListMargins
	case TagListItem:
		return s.ListItemMargins
	case TagCodeBlock:
		return s.CodeBlockMargins
	case TagThematicBreak:
		return s.ThematicBreakMargins
	case TagBlockquote:
		return s.BlockquoteMargins
	case TagTable:
		return s.TableMargins
	default:
		return Margins{}
	}
}

// TextStyle's Emphasis/Strong/CodeSpan cases only reflect that single
// tag's own contribution (italic, bold, monospace) - they don't yet
// compose with ancestry the way e.g. Strong-inside-Emphasis needs to.
// Nothing consumes this yet; see the interface doc.
func (s *DefaultStyleSheet) TextStyle(node *ASTNode) TextStyle {
	if node == nil {
		return TextStyle{}
	}
	switch node.Tag {
	case TagParagraph:
		return TextStyle{Size: s.ParagraphSize}
	case TagHeading1, TagHeading2, TagHeading3, TagHeading4, TagHeading5, TagHeading6:
		i := node.Tag - TagHeading1
		return TextStyle{Size: s.HeadingSizes[i], Weight: s.HeadingWeights[i], Family: s.HeadingFamilies[i]}
	case TagListItem:
		return TextStyle{Size: s.ListItemSize}
	case TagCodeBlock:
		return TextStyle{Size: s.CodeBlockSize, Family: Monospace}
	case TagTableCell:
		return TextStyle{Size: s.TableCellSize}
	case TagCodeSpan:
		return TextStyle{Family: Monospace}
	case TagEmphasis:
		return TextStyle{Style: font.StyleItalic}
	case TagStrong:
		return TextStyle{Weight: font.WeightBold}
	default:
		return TextStyle{}
	}
}

func (s *DefaultStyleSheet) Color(node *ASTNode) color.Color {
	if node == nil {
		return s.TextColor
	}
	switch node.Tag {
	case TagCodeBlock, TagCodeSpan:
		return s.CodeColor
	case TagThematicBreak:
		return s.ThematicBreakColor
	case TagLink:
		return s.LinkColor
	case TagBlockquote:
		return s.BlockquoteBarColor
	case TagTable:
		return s.TableFrameColor
	default:
		return s.TextColor
	}
}

func (s *DefaultStyleSheet) StrikeThickness(node *ASTNode) float64 {
	return s.strikeThickness
}

func (s *DefaultStyleSheet) ThematicBreakThickness(node *ASTNode) float64 {
	return s.thematicBreakThickness
}

func (s *DefaultStyleSheet) BlockquoteGeometry(node *ASTNode) BlockquoteGeometry {
	return s.blockquoteGeometry
}

func (s *DefaultStyleSheet) TableGeometry(node *ASTNode) TableGeometry {
	return s.tableGeometry
}
