package whynot

import (
	"image/color"
)

type MarkdownCompiler struct {
	source         []byte
	headingStyles  [6]partStyle
	paragraphStyle partStyle
	listItemStyle  partStyle
	codeBlockStyle partStyle
	codeColor      color.Color
	linkColor      color.Color
	tableCellStyle partStyle
}

type partStyle struct {
	TextStyle
}
