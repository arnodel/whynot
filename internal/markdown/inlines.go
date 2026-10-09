package markdown

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"unicode"

	gmast "github.com/yuin/goldmark/v2/ast"
	extast "github.com/yuin/goldmark/v2/extension/ast"

	"github.com/arnodel/whynot/fetch"
	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/engine"
)

// spanTags maps each inline construct that only wraps other inline
// content to the ast.Tag it applies to that content.
var spanTags = map[gmast.NodeKind]ast.Tag{
	gmast.KindEmphasis:       ast.TagEmphasis,
	gmast.KindStrong:         ast.TagStrong,
	extast.KindStrikethrough: ast.TagStrikethrough,
}

// appendChildren appends node's inline children, under astNode.
func (c *compiler) appendChildren(items []engine.Inline, node gmast.Node, astNode *ast.Node) []engine.Inline {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		items = c.appendInline(items, child, astNode)
	}
	return items
}

// appendInline appends node's inline content to items. astNode is node's
// parent in the ast.Node tree; only constructs with a tag of their own
// (emphasis, links, ...) add a child to it.
func (c *compiler) appendInline(items []engine.Inline, node gmast.Node, astNode *ast.Node) []engine.Inline {
	if tag, ok := spanTags[node.Kind()]; ok {
		return c.appendChildren(items, node, astNode.AddChild(tag))
	}
	switch node.Kind() {
	case gmast.KindText:
		t := node.(*gmast.Text)
		items = c.appendString(items, t.Value.Value(c.source), astNode)
		// goldmark marks a line break with these flags, not a newline in
		// Value: without them, "laid\nout" would become "laidout". A soft
		// break shows as a space.
		switch {
		case t.HardLineBreak():
			items = append(items, &engine.LineBreak{ASTNode: astNode})
			c.pendingSpace = true
		case t.SoftLineBreak():
			c.pendingSpace = true
		}
		return items
	case gmast.KindCodeSpan:
		cs := node.(*gmast.CodeSpan)
		childNode := astNode.AddChild(ast.TagCodeSpan)
		code := cs.Value.Value(c.source)
		spans := c.inlineTokens(code)
		if spans == nil {
			return c.appendString(items, code, childNode)
		}
		for _, span := range spans {
			// Each token is its own words, glued to its neighbors unless
			// there's a space between them, like text and emphasis.
			tokenNode := childNode
			if span.Class != "" {
				tokenNode = childNode.AddChild(ast.TagCodeToken)
				tokenNode.Class = span.Class
			}
			items = c.appendString(items, span.Text, tokenNode)
		}
		return items
	case gmast.KindImage:
		imgNode := node.(*gmast.Image)
		imageNode := astNode.AddChild(ast.TagImage)
		glued := !c.pendingSpace
		c.pendingSpace = false
		src := imgNode.Destination.Value(c.source)
		img, err := c.resolve(src)
		return append(items, &engine.InlineImage{
			Src:      src,
			Image:    img,
			ImageErr: err,
			Alt:      altText(imgNode, c.source),
			Title:    imgNode.Title.Value(c.source),
			ASTNode:  imageNode,
			// Made here, once: layout runs many times, and would add
			// a new child each time.
			FallbackNode: imageNode.AddChild(ast.TagUnsupported),
			Glued:        glued,
		})
	case gmast.KindLink:
		linkNode := astNode.AddChild(ast.TagLink)
		linkNode.Destination = node.(*gmast.Link).Destination.Value(c.source)
		return c.appendChildren(items, node, linkNode)
	case gmast.KindAutoLink:
		al := node.(*gmast.AutoLink)
		childNode := astNode.AddChild(ast.TagLink)
		childNode.Destination = al.Destination.Value(c.source)
		return c.appendString(items, al.Label.Value(c.source), childNode)
	default:
		return c.appendUnsupportedInline(items, node, astNode)
	}
}

// appendUnsupportedInline is compileUnsupportedBlock's inline
// counterpart: the construct's raw source (where available) becomes
// ordinary words in the paragraph, styled as unsupported.
func (c *compiler) appendUnsupportedInline(items []engine.Inline, node gmast.Node, astNode *ast.Node) []engine.Inline {
	text := fmt.Sprintf("(unsupported: %s)", node.Kind())
	if raw, ok := node.(*gmast.RawHTML); ok {
		text = raw.Value.Value(c.source)
		if strings.HasPrefix(text, "<!--") {
			// A <!-- comment -->: never shown. Unlike blocks, inline
			// RawHTML has no kind of its own to check.
			return items
		}
	}
	log.Printf("whynot: unsupported %s inline content, showing its source instead", node.Kind())
	return c.appendString(items, text, astNode.AddChild(ast.TagUnsupported))
}

// altText flattens an image's description, which may contain any inline
// markup (`![a *b*](x.png)`), into plain text, as HTML does for an alt
// attribute.
func altText(node gmast.Node, source []byte) string {
	var b strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() == gmast.KindText {
			b.WriteString(child.(*gmast.Text).Value.Value(source))
		} else {
			b.WriteString(altText(child, source))
		}
	}
	return b.String()
}

// nbsp is a non-breaking space (U+00A0): what both a literal one and an
// `&nbsp;` entity become in a Text node's Value.
const nbsp = ' '

// appendString splits s into words, as a browser does with plain text:
// each run of breakable whitespace is a gap between words. A
// non-breaking space is a word of its own instead, glued on both sides,
// so it shows as a space but never breaks a line.
//
// Whitespace carries over between calls through c.pendingSpace: a
// whitespace-only Text node between two other nodes ("**a** *b*") adds
// no words, but still separates what comes next.
func (c *compiler) appendString(items []engine.Inline, s string, node *ast.Node) []engine.Inline {
	runes := []rune(s)
	for i := 0; i < len(runes); {
		switch r := runes[i]; {
		case r == nbsp:
			items = append(items, &engine.InlineText{Text: string(nbsp), ASTNode: node, Glued: !c.pendingSpace})
			c.pendingSpace = false
			i++
		case unicode.IsSpace(r):
			c.pendingSpace = true
			i++
		default:
			start := i
			for i < len(runes) && runes[i] != nbsp && !unicode.IsSpace(runes[i]) {
				i++
			}
			items = append(items, &engine.InlineText{Text: string(runes[start:i]), ASTNode: node, Glued: !c.pendingSpace})
			c.pendingSpace = false
		}
	}
	return items
}

// resolve returns the Source of an image's src.
func (c *compiler) resolve(src string) (fetch.Source, error) {
	if c.resolveImage == nil {
		return nil, errors.New("no image resolver")
	}
	return c.resolveImage(src)
}
