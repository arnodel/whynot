// Package codeblocks is how a document's code blocks can be rendered
// beyond plain text: a Highlighter colors their source token by token,
// and a Plugin replaces the rendering of a fenced block in a language it
// recognizes, with an image (e.g. a diagram). Both are given to
// whynot.Parse as options.
//
// Implementations live in subpackages: chromahighlight (a Highlighter)
// and kroki (a Plugin for diagrams).
package codeblocks

import "github.com/arnodel/whynot/images"

// Highlighter classifies a code block's source into consecutive typed
// spans, used to color it token-by-token instead of as one flat run.
// whynot.Parse has no highlighter by default - every code block then
// renders in its StyleSheet's single code color.
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

// Plugin lets a caller replace how a fenced code block in a
// recognized language renders - e.g. a ```mermaid fence as a diagram
// (see the kroki subpackage) instead of its raw/highlighted
// diagram-definition text. Checked before Highlighter, and only for a
// language CanHandle recognizes. A plugin only produces an image; laying
// it out is whynot's job, the same for every plugin.
type Plugin interface {
	// CanHandle reports whether this plugin handles fenced code blocks
	// written in language.
	CanHandle(language string) bool
	// Image starts rendering a fenced code block CanHandle has already
	// approved, returning an images.AsyncImage that resolves once it's ready.
	Image(language, code string) images.AsyncImage
}
