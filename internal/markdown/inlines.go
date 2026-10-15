package markdown

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"unicode"
	"unicode/utf8"

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
	// add adds the node of an inline element, with its source.
	add := func(tag ast.Tag) *ast.Node {
		n := astNode.AddChild(tag)
		n.Source = c.rangeOf(c.inlineStart(node), c.inlineEnd(node))
		return n
	}
	if tag, ok := spanTags[node.Kind()]; ok {
		return c.appendChildren(items, node, add(tag))
	}
	switch node.Kind() {
	case gmast.KindText:
		t := node.(*gmast.Text)
		start, end, decoded := c.textRange(t)
		text := t.Value.Value(c.source)
		items = c.appendString(items, text, astNode, c.textSource(start, end, decoded, text))
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
		childNode := add(ast.TagCodeSpan)
		code := cs.Value.Value(c.source)
		src := textSource{Range: childNode.Source}
		if is := cs.Value.Indices(); !cs.Value.IsOwned() && len(is) == 1 {
			// The code without the space the backquotes may be padded
			// with.
			start, end := is[0].Start, is[0].Stop
			if raw := string(c.source[start:end]); len(raw) >= 2 && raw != code && raw[0] == ' ' && raw[len(raw)-1] == ' ' {
				start, end = start+1, end-1
			}
			src = c.textSource(start, end, true, code)
		}
		spans := c.inlineTokens(code)
		if spans == nil {
			return c.appendString(items, code, childNode, src)
		}
		for _, span := range spans {
			// Each token is its own words, glued to its neighbors unless
			// there's a space between them, like text and emphasis.
			tokenNode := childNode
			if span.Class != "" {
				tokenNode = childNode.AddChild(ast.TagCodeToken)
				tokenNode.Class = span.Class
			}
			items = c.appendString(items, span.Text, tokenNode, src)
			src = src.advance(len(span.Text))
		}
		return items
	case gmast.KindImage:
		imgNode := node.(*gmast.Image)
		imageNode := add(ast.TagImage)
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
		linkNode := add(ast.TagLink)
		linkNode.Destination = node.(*gmast.Link).Destination.Value(c.source)
		return c.appendChildren(items, node, linkNode)
	case gmast.KindAutoLink:
		al := node.(*gmast.AutoLink)
		childNode := add(ast.TagLink)
		childNode.Destination = al.Destination.Value(c.source)
		label := al.Label.Value(c.source)
		src := textSource{Range: childNode.Source}
		if i := al.Label.Index(); !al.Label.IsOwned() {
			src = c.textSource(i.Start, i.Stop, true, label)
		}
		return c.appendString(items, label, childNode, src)
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
	unsupported := astNode.AddChild(ast.TagUnsupported)
	unsupported.Source = c.rangeOf(c.inlineStart(node), c.inlineEnd(node))
	return c.appendString(items, text, unsupported, textSource{Range: unsupported.Source})
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

// textSource is where text appended by appendString is in the document's
// source: as a whole, and, if offsets is set, each of its bytes (see
// align), so that each word's source can be found.
type textSource struct {
	ast.Range
	offsets []int
	raw     string
}

// textSource returns the textSource of text, decoded from [start, end) in
// c.source if decoded, or else made from it.
func (c *compiler) textSource(start, end int, decoded bool, text string) textSource {
	s := textSource{Range: c.rangeOf(start, end)}
	if decoded && s.Range != (ast.Range{}) {
		s.raw = string(c.source[start:end])
		s.offsets = align(s.raw, text)
	}
	return s
}

// word returns the source of the part of the text at [from, to), in
// bytes, and whether the part is that source exactly.
func (s textSource) word(from, to int, text string) (ast.Range, bool) {
	if s.offsets == nil {
		return s.Range, false
	}
	start, end := s.offsets[from], s.offsets[to]
	return ast.Range{Start: s.Start + start, End: s.Start + end}, s.raw[start:end] == text
}

// advance returns the source of what's after the text's first n bytes.
func (s textSource) advance(n int) textSource {
	if s.offsets != nil {
		base := s.offsets[n]
		s.Start += base
		s.raw = s.raw[base:]
		offsets := make([]int, len(s.offsets)-n)
		for i := range offsets {
			offsets[i] = s.offsets[n+i] - base
		}
		s.offsets = offsets
	}
	return s
}

// appendString splits s into words, as a browser does with plain text:
// each run of breakable whitespace is a gap between words. A
// non-breaking space is a word of its own instead, glued on both sides,
// so it shows as a space but never breaks a line.
//
// Whitespace carries over between calls through c.pendingSpace: a
// whitespace-only Text node between two other nodes ("**a** *b*") adds
// no words, but still separates what comes next.
func (c *compiler) appendString(items []engine.Inline, s string, node *ast.Node, src textSource) []engine.Inline {
	word := func(from, to int) *engine.InlineText {
		w := &engine.InlineText{Text: s[from:to], ASTNode: node, Glued: !c.pendingSpace}
		w.Source, w.SourceExact = src.word(from, to, w.Text)
		c.pendingSpace = false
		return w
	}
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == nbsp:
			items = append(items, word(i, i+size))
			i += size
		case unicode.IsSpace(r):
			c.pendingSpace = true
			i += size
		default:
			start := i
			for i < len(s) {
				r, size := utf8.DecodeRuneInString(s[i:])
				if r == nbsp || unicode.IsSpace(r) {
					break
				}
				i += size
			}
			items = append(items, word(start, i))
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
