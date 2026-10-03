package markdown

import (
	"fmt"
	"log"
	"strings"
	"unicode"

	gmast "github.com/yuin/goldmark/v2/ast"
	extast "github.com/yuin/goldmark/v2/extension/ast"

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

// appendInline walks an inline subtree, appending each leaf as an
// Inline. astNode is node's parent in the ast.Node tree - extended only by
// the constructs that get their own ast.Tag, and threaded straight through
// everywhere else. Appearance (font, color, strike) is never resolved
// here - each produced Inline just carries the ast.Node it was created
// under, resolved later by engine.Context against a StyleSheet.
func (c *compiler) appendInline(items []engine.Inline, node gmast.Node, astNode *ast.Node) []engine.Inline {
	if tag, ok := spanTags[node.Kind()]; ok {
		return c.appendChildren(items, node, astNode.AddChild(tag))
	}
	switch node.Kind() {
	case gmast.KindText:
		t := node.(*gmast.Text)
		items = c.appendString(items, t.Value.Value(c.source), astNode)
		// The newline itself isn't in either Text node's Value - goldmark
		// represents a line break purely via this flag - so without this,
		// the next word would glue on with no space (e.g. "laid\nout" ->
		// "laidout"). No forced-break rendering exists yet, so both
		// kinds just become a space.
		if t.SoftLineBreak() || t.HardLineBreak() {
			c.pendingSpace = true
		}
		return items
	case gmast.KindCodeSpan:
		cs := node.(*gmast.CodeSpan)
		childNode := astNode.AddChild(ast.TagCodeSpan)
		return c.appendString(items, cs.Value.Value(c.source), childNode)
	case gmast.KindImage:
		imgNode := node.(*gmast.Image)
		imageNode := astNode.AddChild(ast.TagImage)
		glued := !c.pendingSpace
		c.pendingSpace = false
		return append(items, &engine.InlineImage{
			Src:     imgNode.Destination.Value(c.source),
			Alt:     altText(imgNode, c.source),
			Title:   imgNode.Title.Value(c.source),
			ASTNode: imageNode,
			// fallbackNode is precomputed once, here, rather than
			// on demand inside GetInlineLayout - a layout-time
			// "does the image load" check can run many times
			// (every resize/zoom/reload), and ast.Node.AddChild
			// isn't idempotent, so mutating the tree there would
			// grow a new child every time instead of reusing one.
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

// appendUnsupportedInline is appendInline's fallback for any inline
// Markdown construct whynot doesn't have a case for - the inline
// counterpart to compileUnsupportedBlock. Inline content can't hold a
// block-level box, so the raw source (where available) is spliced into
// the surrounding paragraph as ordinary words, styled in
// StyleSheet.UnsupportedColor via ast.TagUnsupported.
func (c *compiler) appendUnsupportedInline(items []engine.Inline, node gmast.Node, astNode *ast.Node) []engine.Inline {
	text := fmt.Sprintf("(unsupported: %s)", node.Kind())
	if raw, ok := node.(*gmast.RawHTML); ok {
		text = raw.Value.Value(c.source)
		if strings.HasPrefix(text, "<!--") {
			// A <!-- comment -->, invisible in any Markdown renderer -
			// not "unsupported", never meant to be shown at all. Unlike
			// HTMLBlockKind2, goldmark gives inline RawHTML no kind of
			// its own to check instead.
			return items
		}
	}
	log.Printf("whynot: unsupported %s inline content, showing its source instead", node.Kind())
	return c.appendString(items, text, astNode.AddChild(ast.TagUnsupported))
}

// altText flattens an image's child nodes - CommonMark allows arbitrary
// inline content in an image's alt-text description (`![a *b*](x.png)`
// is valid) - into a plain string, the same way HTML rendering flattens
// it into an <img alt="..."> attribute. whynot has nowhere to show
// formatted alt text either, so this recurses into any node kind
// generically rather than special-casing Emphasis/Strong/etc.
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

// nbsp is a non-breaking space (U+00A0) - what a literal NBSP character or
// an `&nbsp;` entity in the source both normalize to by the time goldmark
// hands us a Text node's Value (there's no AST-level distinction between
// them, or from an ordinary space, confirmed against goldmark v2 directly).
const nbsp = ' '

// appendString splits s into Inline items along the same lines a browser
// would collapse/wrap plain text: each maximal run of ordinary breakable
// whitespace becomes a gap between words (as strings.Fields did before),
// but unlike strings.Fields, a literal non-breaking space is never treated
// as that kind of gap - it becomes its own atomic word instead, so it
// can carry Glued (see InlineLayout.Glued) on both sides and end up
// visually spaced but never a line-break point.
//
// Each produced item's Glued reflects c.pendingSpace, the whitespace
// carried over from wherever the previous item (in this call or an
// earlier one, however many sibling nodes back) left off - see
// compiler.pendingSpace's own doc comment for why that carry is
// needed at all (a whitespace-only Text node between two non-text
// siblings, e.g. "**a** *b*", produces zero items of its own here but
// still needs to un-glue whatever comes next).
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
