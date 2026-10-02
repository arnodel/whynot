package whynot

import (
	"strings"

	"github.com/arnodel/whynot/codeblocks"
	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/engine"
)

// tokenClassTags maps a Highlighter's TokenClass to the ast.Tag whose
// Styles.Color contribution renders it - TokenPlain deliberately has
// no entry: a plain span reuses its enclosing code block's own ast.Node
// directly rather than getting a child node of its own, inheriting
// CodeBlockColor the same way untouched code text always has.
var tokenClassTags = map[codeblocks.TokenClass]ast.Tag{
	codeblocks.TokenKeyword:  ast.TagCodeKeyword,
	codeblocks.TokenType:     ast.TagCodeType,
	codeblocks.TokenFunction: ast.TagCodeFunction,
	codeblocks.TokenString:   ast.TagCodeString,
	codeblocks.TokenNumber:   ast.TagCodeNumber,
	codeblocks.TokenComment:  ast.TagCodeComment,
}

// codeBlockLines returns the per-visual-line Inline spans for a fenced
// or indented code block's rawLines - highlighted via c.highlighter if
// one's configured and it behaves (see highlightLines), else one plain
// InlineText per line. Used both for an ordinary CodeBlock and as a
// codeblocks.Plugin's fallback content (see compile.go's KindCodeBlock
// case) - identical either way, since a plugin's fallback is exactly
// what today's non-plugin rendering already is.
func (c *compiler) codeBlockLines(astNode *ast.Node, language string, rawLines []string) [][]engine.Inline {
	var lines [][]engine.Inline
	if c.highlighter != nil {
		lines = highlightLines(c.highlighter, astNode, language, rawLines)
	}
	if lines == nil {
		// No highlighter configured, or highlightLines bailed out on a
		// mismatched line count (a misbehaving Highlighter) - both cases
		// fall back to the same plain, one-InlineText-per-line
		// rendering.
		lines = make([][]engine.Inline, len(rawLines))
		for i, text := range rawLines {
			lines[i] = []engine.Inline{&engine.InlineText{Text: text, ASTNode: astNode}}
		}
	}
	return lines
}

// highlightLines runs h over rawLines joined into one string (giving a
// Highlighter real multi-line context, e.g. for block comments), then
// splits its returned spans back into whynot's one-slice-per-visual-line
// shape - a single HighlightSpan can itself cover several lines. Returns
// nil if h's output doesn't reproduce exactly len(rawLines) lines (a
// misbehaving Highlighter), so the caller can fall back to plain,
// unhighlighted rendering instead.
func highlightLines(h codeblocks.Highlighter, blockNode *ast.Node, language string, rawLines []string) [][]engine.Inline {
	if len(rawLines) == 0 {
		return [][]engine.Inline{}
	}
	code := strings.Join(rawLines, "")
	spans := h.Highlight(language, code)

	lines := make([][]engine.Inline, len(rawLines))
	lineIndex := 0
	var current []engine.Inline
	flushLine := func() {
		if lineIndex >= len(lines) {
			return
		}
		if len(current) == 0 {
			// A blank source line (or a span boundary that happens to
			// land exactly on one) leaves current empty - LineBox
			// requires at least one part (it indexes parts[0]
			// unconditionally), so give it an empty-text placeholder
			// rather than an empty slice.
			current = []engine.Inline{&engine.InlineText{Text: "", ASTNode: blockNode}}
		}
		lines[lineIndex] = current
		current = nil
		lineIndex++
	}
	for _, span := range spans {
		node := blockNode
		if tag, ok := tokenClassTags[span.Class]; ok {
			node = blockNode.AddChild(tag)
		}
		remaining := span.Text
		for {
			nl := strings.IndexByte(remaining, '\n')
			if nl == -1 {
				if remaining != "" {
					current = append(current, &engine.InlineText{Text: remaining, ASTNode: node})
				}
				break
			}
			if remaining[:nl] != "" {
				current = append(current, &engine.InlineText{Text: remaining[:nl], ASTNode: node})
			}
			flushLine()
			remaining = remaining[nl+1:]
		}
	}
	flushLine()
	if lineIndex != len(rawLines) {
		return nil
	}
	return lines
}
