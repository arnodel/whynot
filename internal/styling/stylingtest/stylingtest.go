// Package stylingtest provides styles for tests of packages that can't
// import a theme package (whynot's own tests: styles/... import whynot).
package stylingtest

import (
	"image/color"

	"golang.org/x/image/font"

	"github.com/arnodel/whynot/internal/styling"
)

// Basic returns test styles. Their values match simpletheme's Dark preset
// when this was written, which the tests using them assert numbers from -
// they're fixed test data, not a theme.
func Basic() *styling.Basic {
	return &styling.Basic{
		ParagraphMargins:   styling.Margins{Top: 10, Bottom: 10},
		ParagraphTextStyle: styling.PartialTextStyle{TextStyle: styling.TextStyle{Size: 16}, Set: styling.FieldSize},

		HeadingMargins: [6]styling.Margins{
			{Top: 30, Bottom: 10},
			{Top: 26, Bottom: 10},
			{Top: 22, Bottom: 10},
			{Top: 18, Bottom: 10},
			{Top: 14, Bottom: 10},
			{Top: 10, Bottom: 10},
		},
		// Family is left at its zero value (styling.Proportional) for every
		// level: no bold small-caps font ships in
		// golang.org/x/image/font/gofont, and headings are bold, so
		// small caps isn't available as a default.
		HeadingTextStyles: [6]styling.PartialTextStyle{
			{TextStyle: styling.TextStyle{Size: 40, Weight: font.WeightBold}, Set: styling.FieldSize | styling.FieldWeight | styling.FieldFamily},
			{TextStyle: styling.TextStyle{Size: 36, Weight: font.WeightBold}, Set: styling.FieldSize | styling.FieldWeight | styling.FieldFamily},
			{TextStyle: styling.TextStyle{Size: 32, Weight: font.WeightBold}, Set: styling.FieldSize | styling.FieldWeight | styling.FieldFamily},
			{TextStyle: styling.TextStyle{Size: 28, Weight: font.WeightBold}, Set: styling.FieldSize | styling.FieldWeight | styling.FieldFamily},
			{TextStyle: styling.TextStyle{Size: 24, Weight: font.WeightBold}, Set: styling.FieldSize | styling.FieldWeight | styling.FieldFamily},
			{TextStyle: styling.TextStyle{Size: 20, Weight: font.WeightBold}, Set: styling.FieldSize | styling.FieldWeight | styling.FieldFamily},
		},

		ListMargins: styling.Margins{Top: 10, Bottom: 10},

		ListItemMargins:   styling.Margins{Top: 5, Bottom: 5, Left: 40},
		ListItemTextStyle: styling.PartialTextStyle{TextStyle: styling.TextStyle{Size: 16}, Set: styling.FieldSize},

		CodeBlockMargins:   styling.Margins{Top: 20, Bottom: 20, Left: 20},
		CodeBlockTextStyle: styling.PartialTextStyle{TextStyle: styling.TextStyle{Size: 16, Family: styling.Monospace}, Set: styling.FieldSize | styling.FieldFamily},
		CodeBlockColor:     color.RGBA{0xD4, 0xD4, 0xD4, 0xFF},

		// Like ThematicBreakColor/BlockquoteBarColor/TableFrameColor
		// below, a strong red reads as an error against either a light
		// or dark background, so Light leaves it as-is.
		UnsupportedColor: color.RGBA{0xFF, 0x33, 0x33, 0xFF},

		CodeSpanTextStyle: styling.PartialTextStyle{TextStyle: styling.TextStyle{Family: styling.Monospace}, Set: styling.FieldFamily},
		CodeSpanColor:     color.RGBA{0xFF, 0xFF, 0x80, 0xFF},
		EmphasisTextStyle: styling.PartialTextStyle{TextStyle: styling.TextStyle{Style: font.StyleItalic}, Set: styling.FieldStyle},
		StrongTextStyle:   styling.PartialTextStyle{TextStyle: styling.TextStyle{Weight: font.WeightBold}, Set: styling.FieldWeight},

		ThematicBreakMargins: styling.Margins{Top: 20, Bottom: 20},
		ThematicBreakColor:   color.RGBA{0x80, 0x80, 0x80, 0xFF},

		LinkColor: color.RGBA{0x66, 0xB2, 0xFF, 0xFF},

		// Like ThematicBreakColor/BlockquoteBarColor/TableFrameColor
		// below, an orange reads fine against either a light or dark
		// background, so Light leaves it as-is.
		Highlight: color.RGBA{0xFF, 0xA5, 0x00, 0xFF},

		// Light overrides this fully (see below) - unlike
		// Highlight above, these don't read well against both
		// backgrounds.
		Scrollbar: styling.ScrollbarColors{
			Idle:    color.RGBA{0x80, 0x80, 0x80, 0xA0},
			Hover:   color.RGBA{0xA0, 0xA0, 0xA0, 0xC0},
			Pressed: color.RGBA{0xC0, 0xC0, 0xC0, 0xE0},
		},

		// Light overrides this fully (see below) - a palette
		// tuned for a dark background won't read well on light and vice
		// versa, the same reason TextColor/LinkColor/CodeBlockColor/
		// CodeSpanColor differ between the two themes.
		Syntax: styling.SyntaxColors{
			Keyword:  color.RGBA{0xC5, 0x86, 0xF2, 0xFF}, // soft violet
			Type:     color.RGBA{0x4E, 0xC9, 0xB0, 0xFF}, // soft teal
			Function: color.RGBA{0xDC, 0xDC, 0xAA, 0xFF}, // soft yellow-tan
			String:   color.RGBA{0x9E, 0xD9, 0x7A, 0xFF}, // soft green
			Number:   color.RGBA{0xF2, 0xB0, 0x66, 0xFF}, // soft orange
			Comment:  color.RGBA{0x80, 0x80, 0x80, 0xFF}, // matches the existing mid-grey decoration color
		},

		BlockquoteMargins:  styling.Margins{Top: 10, Bottom: 10},
		BlockquoteBarColor: color.RGBA{0x80, 0x80, 0x80, 0xFF},

		TableCellTextStyle: styling.PartialTextStyle{TextStyle: styling.TextStyle{Size: 16}, Set: styling.FieldSize},
		TableMargins:       styling.Margins{Top: 10, Bottom: 10},
		TableFrameColor:    color.RGBA{0x80, 0x80, 0x80, 0xFF},

		ImagePlaceholderColor: color.RGBA{0x80, 0x80, 0x80, 0xFF},

		TextColor:     color.White,
		BaseTextStyle: styling.TextStyle{Size: 16},

		Background: color.Black,
		ViewMargin: styling.Margins{Top: 20, Bottom: 20, Left: 20, Right: 20},

		Dims: styling.Dimensions{
			Strike:         1,
			ThematicBreak:  2,
			LineHeight:     1.2,
			DiagramPadding: 12,
			Blockquote:     styling.BlockquoteGeometry{Indent: 16, BarWidth: 3},
			Table: styling.TableGeometry{
				FrameThickness:      2,
				ColumnGap:           12,
				RowGap:              6,
				HeaderGap:           4,
				ColumnRuleThickness: 1,
			},
		},
	}
}
