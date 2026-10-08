package codeblocks

import "github.com/arnodel/whynot/fetch"

// Plugin parses fenced code blocks in the languages it handles.
//
// Several plugins can handle the same language: the first one
// registered parses the block. If its Content is an Image, what the next
// one would make of the block is shown while the image loads, or if it
// fails; plain text if there's no next one.
type Plugin interface {
	// Handles reports whether the plugin parses code blocks in language
	// (the fence's info string, e.g. "go"). It's asked once per language
	// per document.
	Handles(language string) bool

	// Parse makes Content of a code block's source, in a language Handles
	// approved. A nil Content means the plugin declines this block, and
	// the next plugin handling the language is asked instead. code can
	// also be a code span's, a single line, when the document's inline
	// code is in language (see the package's Inline code section).
	Parse(language, code string) Content
}

// Content is what a Plugin makes of a code block: Tokens or an Image.
// Only this package can define kinds of Content, so new kinds can be
// added without breaking plugins.
type Content interface{ isContent() }

// Tokens is a code block's source split into consecutive spans, each
// classified for coloring. The spans' Text must concatenate back to the
// source exactly; if they don't, the tokens are ignored.
type Tokens struct {
	Spans []Span
}

// Span is one classified run of source text. Class is open-ended: a
// stylesheet colors the classes it knows (see the Class constants) and
// shows any other class, or none (""), in the code block's own color.
type Span struct {
	Text  string
	Class string
}

// Image is a code block shown as an image, e.g. a rendered diagram,
// fetched from Source in the background. Source's key only needs to be
// unique among the plugin's own keys: whynot namespaces it.
type Image struct {
	Source fetch.Source
}

func (Tokens) isContent() {}
func (Image) isContent()  {}

// Conventional Span classes. These are the ones whynot's own
// stylesheets color; a plugin may use others, which a stylesheet shows
// in the code block's own color unless it knows them.
const (
	ClassKeyword = "keyword"
	// ClassType is a type name: a builtin type and a declared type or
	// class name share it.
	ClassType = "type"
	// ClassFunction is a function or method name, at least where the
	// syntax makes it clear (a declaration, often a call).
	ClassFunction = "function"
	ClassString   = "string"
	ClassNumber   = "number"
	ClassComment  = "comment"
)
