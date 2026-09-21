package whynot

import "strings"

// Highlighter classifies a code block's source into consecutive typed
// spans, used to color it token-by-token instead of as one flat run.
// Parse/NewView have no highlighter by default - every code block then
// renders in one flat color (StyleSheet's existing TagCodeBlock case),
// exactly as before this existed.
type Highlighter interface {
	// Highlight splits code (a whole fenced/indented block's source,
	// potentially multiple lines) into consecutive spans whose Text
	// concatenates back to code exactly. language is the fence's own
	// info string (e.g. "go"), "" if there was none (including every
	// indented code block, which has no fence to carry one).
	// Implementations are free to return a single TokenPlain span
	// covering all of code (e.g. an unrecognized language) - equivalent
	// to no highlighting for that block.
	Highlight(language, code string) []HighlightSpan
}

// HighlightSpan is one classified run of a Highlighter's output.
type HighlightSpan struct {
	Text  string
	Class TokenClass
}

// TokenClass is a small, whynot-level classification of a syntax token -
// deliberately much coarser than a real highlighting library's own token
// taxonomy (chroma alone has ~50 TokenTypes): StyleSheet only needs enough
// categories to assign a handful of distinct colors, not to reproduce a
// library's full type system.
type TokenClass int

const (
	TokenPlain TokenClass = iota // no special color - inherits the block's own CodeColor
	TokenKeyword
	TokenString
	TokenNumber
	TokenComment
)

// ParseOption customizes Parse - see WithSyntaxHighlighter.
type ParseOption func(*MarkdownCompiler)

// WithSyntaxHighlighter sets the Highlighter Parse uses to color code
// blocks token-by-token. Unset, code blocks render in one flat color,
// exactly as before this option existed. See WithHighlighter for the
// equivalent NewView option.
func WithSyntaxHighlighter(h Highlighter) ParseOption {
	return func(c *MarkdownCompiler) {
		c.highlighter = h
	}
}

// tokenClassTags maps a Highlighter's TokenClass to the ASTTag whose
// StyleSheet.Color contribution renders it - TokenPlain deliberately has
// no entry: a plain span reuses its enclosing code block's own ASTNode
// directly rather than getting a child node of its own, inheriting
// CodeColor the same way untouched code text always has.
var tokenClassTags = map[TokenClass]ASTTag{
	TokenKeyword: TagCodeKeyword,
	TokenString:  TagCodeString,
	TokenNumber:  TagCodeNumber,
	TokenComment: TagCodeComment,
}

// highlightLines runs h over rawLines (already tab-expanded, concatenated
// - each rawLine already carries its own trailing "\n", see compile.go's
// KindCodeBlock case - so a highlighter has real multi-line context for
// constructs like block comments), then splits its returned spans back
// into whynot's one-slice-per-visual-line shape. A single HighlightSpan
// can itself cover several lines (e.g. a block comment) - this is a real
// resplitting step, not a 1:1 relabeling. Returns nil if h's output
// doesn't reproduce exactly len(rawLines) lines (a misbehaving
// Highlighter), so the caller can fall back to plain, unhighlighted
// rendering rather than risk a corrupted line count.
func highlightLines(h Highlighter, blockNode *ASTNode, language string, rawLines []string) [][]Inline {
	if len(rawLines) == 0 {
		return [][]Inline{}
	}
	code := strings.Join(rawLines, "")
	spans := h.Highlight(language, code)

	lines := make([][]Inline, len(rawLines))
	lineIndex := 0
	var current []Inline
	flushLine := func() {
		if lineIndex >= len(lines) {
			return
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
					current = append(current, &InlineText{text: remaining, node: node})
				}
				break
			}
			if remaining[:nl] != "" {
				current = append(current, &InlineText{text: remaining[:nl], node: node})
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
