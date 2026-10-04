package whynot

import (
	"github.com/arnodel/whynot/codeblocks"
	"github.com/arnodel/whynot/internal/markdown"
)

// Parse compiles Markdown source into a Document ready to render with a
// View. Parse only builds structure: all appearance (fonts, colors,
// margins, ...) comes from the View's StyleSheet, so the same Document
// can be shown in different styles without parsing it again.
func Parse(source []byte, opts ...ParseOption) *Document {
	var o parseOptions
	for _, opt := range opts {
		opt(&o)
	}
	r := markdown.Compile(source, o.plugins)
	headings := make([]TOCEntry, len(r.Headings))
	for i, h := range r.Headings {
		headings[i] = TOCEntry(h)
	}
	return &Document{root: r.Root, headings: headings, soleImages: r.SoleImages}
}

// parseOptions is what ParseOptions configure.
type parseOptions struct {
	plugins []codeblocks.Plugin
}

// ParseOption customizes Parse - see WithCodeBlockPlugin.
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
