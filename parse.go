package whynot

import (
	"net/url"

	"github.com/arnodel/whynot/codeblocks"
	"github.com/arnodel/whynot/fetch"
	"github.com/arnodel/whynot/internal/markdown"
)

// Parse compiles Markdown source into a Document ready to render with a
// View. Parse only builds structure: all appearance (fonts, colors,
// margins, ...) comes from the View's StyleSheet, so the same Document
// can be shown in different styles without parsing it again.
//
// Images are resolved here, once, through the registry given by
// [WithImageRegistry]: without one, a document shows no images, only
// their alternative text.
func Parse(source []byte, opts ...ParseOption) *Document {
	var o parseOptions
	for _, opt := range opts {
		opt(&o)
	}
	r := markdown.Compile(source, markdown.Options{
		Plugins:            o.plugins,
		InlineCodeLanguage: o.inlineCodeLanguage,
		ResolveImage: func(src string) (fetch.Source, error) {
			return o.images.Resolve(o.base, src)
		},
	})
	headings := make([]TOCEntry, len(r.Headings))
	for i, h := range r.Headings {
		headings[i] = TOCEntry(h)
	}
	return &Document{root: r.Root, headings: headings, soleImages: r.SoleImages}
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
