// Package markdown is whynot's Markdown compiler: it parses source with
// goldmark and builds the engine's block and inline tree, alongside the
// semantic ast.Node tree they refer to (Compile). Fenced code blocks go
// through the codeblocks.Plugin given for them.
//
// It only builds structure: appearance comes later, from a View's
// StyleSheet, and layout and drawing are the engine's job.
package markdown

import (
	"github.com/arnodel/whynot/codeblocks"
	"github.com/arnodel/whynot/internal/engine"
)

// compiler compiles Markdown source into a Block/ast.Node tree - structure
// only, no appearance. See Parse.
type compiler struct {
	source []byte

	// codeBlockPlugins is every codeblocks.Plugin registered via
	// WithCodeBlockPlugin, in registration order. pluginCache remembers
	// which of them handle a given language, populated lazily by
	// pluginsFor - so a document with many fences in the same language
	// only calls Handles once per plugin per language, not once per
	// fence.
	codeBlockPlugins []codeblocks.Plugin
	pluginCache      map[string][]codeblocks.Plugin

	// pendingSpace is true when a breakable space has been seen in the
	// source but not yet attached to the next appended Inline item - see
	// appendString. Reset to true at the start of each fresh run of
	// inline content (a paragraph, heading, list item head, or table
	// cell), since there's nothing for the first item there to glue to.
	pendingSpace bool

	// headings and soleImages accumulate the Result fields of the same
	// names as top-level blocks are compiled.
	headings   []Heading
	soleImages map[engine.Block]string
}

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
