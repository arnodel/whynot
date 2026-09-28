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
	TokenPlain TokenClass = iota // no special color - inherits the block's own CodeBlockColor
	TokenKeyword
	// TokenType is a type name - a builtin primitive type and a declared
	// custom type/class name share this one class, so e.g. int and a
	// user-defined struct read as the same kind of thing. A Highlighter
	// isn't expected to recognize every usage of a type, only what it
	// can tell from syntax (typically its declaration).
	TokenType
	// TokenFunction is a function/method name - same expectation as
	// TokenType: recognize what's clear from syntax (a declaration, and
	// often a call), not necessarily every usage.
	TokenFunction
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

// CodeBlockPlugin lets a caller replace how a fenced code block in a
// recognized language renders - e.g. a ```mermaid fence as a diagram
// (see the kroki package) instead of its raw/highlighted
// diagram-definition text. Checked before Highlighter, and only for a
// language CanHandle recognizes - it knows nothing about Block/ASTNode,
// compile.go owns all of that uniformly for every plugin.
type CodeBlockPlugin interface {
	// CanHandle reports whether this plugin handles fenced code blocks
	// written in language.
	CanHandle(language string) bool
	// Image starts rendering a fenced code block CanHandle has already
	// approved, returning an AsyncImage that resolves once it's ready.
	Image(language, code string) AsyncImage
}

// WithCodeBlockPlugin registers one CodeBlockPlugin - callable more
// than once to register several; a fenced code block's language is
// matched against them in registration order (see
// MarkdownCompiler.pluginFor). See WithCodeBlockPlugins for the
// equivalent NewView option.
func WithCodeBlockPlugin(p CodeBlockPlugin) ParseOption {
	return func(c *MarkdownCompiler) {
		c.codeBlockPlugins = append(c.codeBlockPlugins, p)
	}
}

// tokenClassTags maps a Highlighter's TokenClass to the ASTTag whose
// StyleSheet.Color contribution renders it - TokenPlain deliberately has
// no entry: a plain span reuses its enclosing code block's own ASTNode
// directly rather than getting a child node of its own, inheriting
// CodeBlockColor the same way untouched code text always has.
var tokenClassTags = map[TokenClass]ASTTag{
	TokenKeyword:  TagCodeKeyword,
	TokenType:     TagCodeType,
	TokenFunction: TagCodeFunction,
	TokenString:   TagCodeString,
	TokenNumber:   TagCodeNumber,
	TokenComment:  TagCodeComment,
}

// codeBlockLines returns the per-visual-line Inline spans for a fenced
// or indented code block's rawLines - highlighted via c.highlighter if
// one's configured and it behaves (see highlightLines), else one plain
// InlineText per line. Used both for an ordinary CodeBlock and as a
// CodeBlockPlugin's fallback content (see compile.go's KindCodeBlock
// case) - identical either way, since a plugin's fallback is exactly
// what today's non-plugin rendering already is.
func (c *MarkdownCompiler) codeBlockLines(astNode *ASTNode, language string, rawLines []string) [][]Inline {
	var lines [][]Inline
	if c.highlighter != nil {
		lines = highlightLines(c.highlighter, astNode, language, rawLines)
	}
	if lines == nil {
		// No highlighter configured, or highlightLines bailed out on a
		// mismatched line count (a misbehaving Highlighter) - both cases
		// fall back to the same plain, one-InlineText-per-line
		// rendering.
		lines = make([][]Inline, len(rawLines))
		for i, text := range rawLines {
			lines[i] = []Inline{&InlineText{text: text, node: astNode}}
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
		if len(current) == 0 {
			// A blank source line (or a span boundary that happens to
			// land exactly on one) leaves current empty - LineBox
			// requires at least one part (it indexes parts[0]
			// unconditionally), so give it an empty-text placeholder
			// rather than an empty slice.
			current = []Inline{&InlineText{text: "", node: blockNode}}
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
