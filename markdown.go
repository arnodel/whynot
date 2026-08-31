package whynot

import (
	"image/color"
)

type MarkdownCompiler struct {
	source         []byte
	headingStyles  [6]partStyle
	paragraphStyle partStyle
	listItemStyle  partStyle
	listStyle      partStyle
	codeBlockStyle partStyle
	codeColor      color.Color
}

type partStyle struct {
	TextStyle
	Margins
	LevelOffset int
}
