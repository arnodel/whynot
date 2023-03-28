package main

import (
	"fmt"
	"image/color"
	"log"
	"os"
	"strings"

	"github.com/yuin/goldmark"
	gmast "github.com/yuin/goldmark/ast"
	gmtext "github.com/yuin/goldmark/text"
	"golang.org/x/image/font"
)

func parseMarkdown(f string) Block {
	source, err := os.ReadFile(f)
	if err != nil {
		panic(err)
	}
	parser := goldmark.DefaultParser()
	reader := gmtext.NewReader(source)
	node := parser.Parse(reader)
	node.Dump(source, 2)
	compiler := MarkdownCompiler{
		source: source,
		paragraphStyle: partStyle{
			TextStyle: TextStyle{Size: 16},
			Margins:   Margins{Top: 10, Bottom: 10},
		},
		listItemStyle: partStyle{
			TextStyle: TextStyle{Size: 16},
			Margins:   Margins{Top: 5, Bottom: 5, Left: 40},
		},
		listStyle: partStyle{
			Margins: Margins{Top: 10, Bottom: 10},
		},
		headingStyles: [6]partStyle{
			{
				TextStyle:   TextStyle{Size: 40, Weight: font.WeightBold, Family: SmallCaps},
				Margins:     Margins{Top: 30, Bottom: 10},
				LevelOffset: 2,
			},
			{
				TextStyle:   TextStyle{Size: 36, Weight: font.WeightBold},
				Margins:     Margins{Top: 26, Bottom: 10},
				LevelOffset: 2,
			},
			{
				TextStyle:   TextStyle{Size: 32, Weight: font.WeightBold},
				Margins:     Margins{Top: 22, Bottom: 10},
				LevelOffset: 2,
			},
			{
				TextStyle:   TextStyle{Size: 28, Weight: font.WeightBold},
				Margins:     Margins{Top: 18, Bottom: 10},
				LevelOffset: 2,
			},
			{
				TextStyle:   TextStyle{Size: 24, Weight: font.WeightBold},
				Margins:     Margins{Top: 14, Bottom: 10},
				LevelOffset: 2,
			},
			{
				TextStyle:   TextStyle{Size: 20, Weight: font.WeightBold},
				Margins:     Margins{Top: 10, Bottom: 10},
				LevelOffset: 2,
			},
		},
		codeBlockStyle: partStyle{
			TextStyle: TextStyle{Size: 16, Family: Monospace},
			Margins:   Margins{Top: 20, Bottom: 20, Left: 20},
		},
	}
	return compiler.CompileDocument(node)
}

type MarkdownCompiler struct {
	source         []byte
	headingStyles  [6]partStyle
	paragraphStyle partStyle
	listItemStyle  partStyle
	listStyle      partStyle
	codeBlockStyle partStyle
}

type partStyle struct {
	TextStyle
	Margins
	LevelOffset int
}

func (c *MarkdownCompiler) CompileNode(node gmast.Node) Block {
	switch node.Type() {
	case gmast.TypeDocument:
		return c.CompileDocument(node)
	case gmast.TypeBlock:
		return c.CompileBlock(node)
	}
	return nil
}

func (c *MarkdownCompiler) CompileDocument(node gmast.Node) Block {
	var blocks []Block
	child := node.FirstChild()
	for child != nil {
		blocks = append(blocks, c.CompileNode(child))
		child = child.NextSibling()
	}
	return &StackBlock{blocks: blocks}
}

func (c *MarkdownCompiler) CompileBlock(node gmast.Node) Block {
	switch node.Kind() {
	case gmast.KindParagraph:
		var items []Inline
		child := node.FirstChild()
		for child != nil {
			items = c.AppendInlineNode(items, child, 0, c.paragraphStyle.Size)
			child = child.NextSibling()
		}
		return &TextBlock{parts: items, margins: c.paragraphStyle.Margins}
	case gmast.KindHeading:
		var items []Inline
		partStyle := c.headingStyles[node.(*gmast.Heading).Level-1]
		child := node.FirstChild()
		for child != nil {
			items = c.AppendInlineNode(items, child, 2, partStyle.Size)
			child = child.NextSibling()
		}
		return &TextBlock{parts: items, margins: partStyle.Margins}
	case gmast.KindList:
		list := node.(*gmast.List)
		var items []Block
		var index = 1
		child := node.FirstChild()
		for child != nil {
			items = append(items, c.CompileListItem(child, index, list.Marker))
			child = child.NextSibling()
			index++
		}
		return &StackBlock{blocks: items, margins: c.listStyle.Margins}
	case gmast.KindFencedCodeBlock:
		lineCount := node.Lines().Len()
		items := make([]Inline, lineCount)
		for i := 0; i < lineCount; i++ {
			line := node.Lines().At(i)
			items[i] = &InlineText{
				text:  string(line.Value(c.source)),
				style: c.codeBlockStyle.TextStyle,
				color: color.White,
			}
		}
		return &CodeBlock{
			margins: c.codeBlockStyle.Margins,
			lines:   items,
		}
	}
	panic("Unsupported block")
}

func (c *MarkdownCompiler) CompileListItem(node gmast.Node, index int, marker byte) Block {
	var items []Inline
	var markerString string
	switch marker {
	case '-', '+', '*':
		markerString = string(marker)
	case ')':
		markerString = fmt.Sprintf("%d)", index)
	case '.':
		markerString = fmt.Sprintf("%d.", index)
	default:
		panic("Unsupported marker")
	}

	contents := node.FirstChild()
	switch contents.Kind() {
	case gmast.KindTextBlock:
		child := contents.FirstChild()
		for child != nil {
			items = c.AppendInlineNode(items, child, 0, c.listItemStyle.Size)
			child = child.NextSibling()
		}
	default:
		log.Panicf("Unuspported node kind: %s", contents.Kind())
	}

	return &ListItemBlock{parts: items, margins: c.listItemStyle.Margins, marker: &InlineText{text: markerString, color: color.White, style: c.listItemStyle.TextStyle}}
}

func (c *MarkdownCompiler) AppendInlineNode(items []Inline, node gmast.Node, baseLevel int, size float64) []Inline {
	switch node.Kind() {
	case gmast.KindString:
		return appendString(items, string(node.(*gmast.String).Value), getStyle(baseLevel, size))
	case gmast.KindText:
		return appendString(items, string(node.Text(c.source)), getStyle(baseLevel, size))
	case gmast.KindEmphasis:
		child := node.FirstChild()
		baseLevel += node.(*gmast.Emphasis).Level
		for child != nil {
			items = c.AppendInlineNode(items, child, baseLevel, size)
			child = child.NextSibling()
		}
		return items
	case gmast.KindCodeSpan:
		style := getStyle(baseLevel, size)
		style.Family = Monospace
		return appendString(items, string(node.Text(c.source)), style)
	default:
		log.Panicf("Unsupported node kind %s", node.Kind())
	}
	return nil
}

var levelToStyles = [4]TextStyle{
	{0, font.StyleNormal, font.WeightNormal, Proportional},
	{0, font.StyleItalic, font.WeightNormal, Proportional},
	{0, font.StyleNormal, font.WeightBold, Proportional},
	{0, font.StyleItalic, font.WeightBold, Proportional},
}

func getStyle(level int, size float64) TextStyle {
	textStyle := levelToStyles[level%len(levelToStyles)]
	textStyle.Size = size
	return textStyle
}

func appendString(items []Inline, s string, style TextStyle) []Inline {
	textParts := strings.Fields(s)
	for _, part := range textParts {
		items = append(items, &InlineText{
			text:  part,
			color: color.White,
			style: style,
		})
	}
	return items
}
