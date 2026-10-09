package whynot

import (
	"io"
	"net/url"

	"github.com/arnodel/whynot/codeblocks"
	"github.com/arnodel/whynot/fetch"
	"github.com/arnodel/whynot/internal/markdown"
)

// Parse compiles Markdown source into a Document ready to render with a
// View: shorthand for NewParser(opts...).Parse(source). Parse only builds
// structure: all appearance (fonts, colors, margins, ...) comes from the
// View's StyleSheet, so the same Document can be shown in different
// styles without parsing it again.
//
// Images are resolved here, once, through the registry given by
// [WithImageRegistry]: without one, a document shows no images, only
// their alternative text.
func Parse(source []byte, opts ...ParseOption) *Document {
	return NewParser(opts...).Parse(source)
}

// A Parser makes Documents from Markdown, with the same options for each.
// It's immutable, and safe for concurrent use.
type Parser struct {
	opts []ParseOption
}

// NewParser returns a Parser with opts.
func NewParser(opts ...ParseOption) *Parser {
	return &Parser{opts: opts}
}

// Parse returns the Document of source, complete. opts apply to this
// Document only, after the Parser's: a [WithBaseURL] overrides the
// Parser's, and a [WithCodeBlockPlugin] adds to its plugins.
func (p *Parser) Parse(source []byte, opts ...ParseOption) *Document {
	s := markdown.NewStream(p.options(opts))
	s.Write(source)
	s.Close()
	finished, _ := s.Update()
	return &Document{finished: finished, version: 1, complete: true}
}

// Stream returns a Document that grows as Markdown is written to w, until
// w is closed (see "Documents that arrive a piece at a time" in the
// package documentation). opts apply to this Document only, as for Parse.
//
// Writing only stores the text, and returns at once: the Document
// compiles it when it's next looked at, as when a View showing it draws.
// Text can be split anywhere, even within a UTF-8 character. w can be
// used on any goroutine, while Views of the Document are used on another.
// Writing after Close is an error.
func (p *Parser) Stream(opts ...ParseOption) (doc *Document, w io.WriteCloser) {
	doc = &Document{stream: markdown.NewStream(p.options(opts))}
	return doc, streamWriter{doc}
}

// options returns the compiler's options: the Parser's, then extra.
func (p *Parser) options(extra []ParseOption) markdown.Options {
	var o parseOptions
	for _, opt := range p.opts {
		opt(&o)
	}
	for _, opt := range extra {
		opt(&o)
	}
	return markdown.Options{
		Plugins:            o.plugins,
		InlineCodeLanguage: o.inlineCodeLanguage,
		ResolveImage: func(src string) (fetch.Source, error) {
			return o.images.Resolve(o.base, src)
		},
	}
}

// parseOptions is what ParseOptions configure.
type parseOptions struct {
	plugins            []codeblocks.Plugin
	inlineCodeLanguage string
	base               *url.URL
	images             *fetch.Registry
}

// ParseOption customizes Parse.
type ParseOption func(*parseOptions)

// WithCodeBlockPlugin registers a plugin that parses fenced code blocks
// in the languages it handles, e.g. to color them or render them as
// diagrams. Call it once per plugin: plugins are tried in registration
// order (see [codeblocks.Plugin]).
func WithCodeBlockPlugin(p codeblocks.Plugin) ParseOption {
	return func(o *parseOptions) {
		o.plugins = append(o.plugins, p)
	}
}

// WithInlineCodeLanguage sets the language of the document's inline
// code, such as "lua", so that the code-block plugins (see
// [WithCodeBlockPlugin]) can color it: each code span is offered to them
// as a fenced block in that language would be. Only [codeblocks.Tokens]
// are used inline; a span no plugin makes tokens of stays plain.
//
// Without it, inline code isn't colored: Markdown has no way to say what
// language a code span is in.
func WithInlineCodeLanguage(language string) ParseOption {
	return func(o *parseOptions) {
		o.inlineCodeLanguage = language
	}
}

// WithBaseURL sets the URL the document comes from, which relative image
// srcs are resolved against: a src takes its scheme, host and directory,
// as in a web browser. Without one, a relative src is a file path,
// relative to the working directory.
func WithBaseURL(u *url.URL) ParseOption {
	return func(o *parseOptions) {
		o.base = u
	}
}

// WithImageRegistry sets what the document's images can be fetched from:
// only the schemes of r's Resolvers (see package [fetch]). Without it, no
// image is fetched.
func WithImageRegistry(r *fetch.Registry) ParseOption {
	return func(o *parseOptions) {
		o.images = r
	}
}
