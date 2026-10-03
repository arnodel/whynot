package markdown

import (
	"strings"

	"github.com/arnodel/whynot/codeblocks"
	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/engine"
)

// codeBlockTabExpansion is what a literal tab in a code block's source is
// replaced with - see the KindCodeBlock case below.
const codeBlockTabExpansion = "    "

// pluginsFor returns the registered plugins that handle language, in
// registration order - resolved once per distinct language and cached
// from then on.
func (c *compiler) pluginsFor(language string) []codeblocks.Plugin {
	if ps, ok := c.pluginCache[language]; ok {
		return ps
	}
	if c.pluginCache == nil {
		c.pluginCache = make(map[string][]codeblocks.Plugin)
	}
	var ps []codeblocks.Plugin
	for _, p := range c.codeBlockPlugins {
		if p.Handles(language) {
			ps = append(ps, p)
		}
	}
	c.pluginCache[language] = ps
	return ps
}

// codeBlock builds a fenced or indented code block's content from its
// rawLines, through the first of plugins (those handling its language,
// in registration order) that makes something of it: Tokens become a
// colored code block, and an Image a diagram that falls back to what
// the remaining plugins make of the block. With no plugin left, it's
// plain text, one InlineText per line.
func (c *compiler) codeBlock(astNode *ast.Node, language string, rawLines []string, plugins []codeblocks.Plugin) engine.Block {
	for i, p := range plugins {
		// rawLines carry their own trailing newlines - joining with ""
		// avoids doubling them up.
		code := strings.Join(rawLines, "")
		switch content := p.Parse(language, code).(type) {
		case codeblocks.Tokens:
			if lines := tokenLines(content.Spans, astNode, len(rawLines)); lines != nil {
				return &engine.CodeBlock{Lines: lines, ASTNode: astNode}
			}
			// Tokens that don't reproduce the source: try the next plugin.
		case codeblocks.Image:
			fallback := c.codeBlock(astNode, language, rawLines, plugins[i+1:])
			return engine.NewDiagramBlock(astNode, content.AsyncImage, fallback)
		}
	}
	lines := make([][]engine.Inline, len(rawLines))
	for i, text := range rawLines {
		lines[i] = []engine.Inline{&engine.InlineText{Text: text, ASTNode: astNode}}
	}
	return &engine.CodeBlock{Lines: lines, ASTNode: astNode}
}

// tokenLines splits spans, which may each cover several lines, into one
// slice of Inlines per visual line. A span with a class gets its own
// TagCodeToken node under blockNode, carrying the class for styling; an
// unclassified one uses blockNode itself. Returns nil if the spans don't
// make exactly lineCount lines (a misbehaving plugin), so the caller can
// fall back.
func tokenLines(spans []codeblocks.Span, blockNode *ast.Node, lineCount int) [][]engine.Inline {
	if lineCount == 0 {
		return [][]engine.Inline{}
	}
	lines := make([][]engine.Inline, lineCount)
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
		if span.Class != "" {
			node = blockNode.AddChild(ast.TagCodeToken)
			node.Class = span.Class
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
	if lineIndex != lineCount {
		return nil
	}
	return lines
}
