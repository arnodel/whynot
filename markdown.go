package whynot

import (
	"github.com/arnodel/whynot/codeblocks"
	"github.com/arnodel/whynot/internal/engine"
)

// compiler compiles Markdown source into a Block/ast.Node tree - structure
// only, no appearance. See Parse.
type compiler struct {
	source      []byte
	highlighter codeblocks.Highlighter

	// codeBlockPlugins is every codeblocks.Plugin registered via
	// WithCodeBlockPlugin, in registration order. pluginCache remembers
	// which one (if any) handles a given language, populated lazily by
	// pluginFor - so a document with many fences in the same language
	// only ever calls CanHandle once per plugin per language, not once
	// per fence. A cached nil value means "known: none handle it".
	codeBlockPlugins []codeblocks.Plugin
	pluginCache      map[string]codeblocks.Plugin

	// pendingSpace is true when a breakable space has been seen in the
	// source but not yet attached to the next appended Inline item - see
	// appendString. Reset to true at the start of each fresh run of
	// inline content (a paragraph, heading, list item head, or table
	// cell), since there's nothing for the first item there to glue to.
	pendingSpace bool

	// headings and soleImages accumulate the Document fields of the same
	// names as top-level blocks are compiled.
	headings   []TOCEntry
	soleImages map[engine.Block]string
}

// pluginFor returns the first registered codeblocks.Plugin that handles
// language, or nil if none do - resolved once per distinct language and
// cached from then on.
func (c *compiler) pluginFor(language string) codeblocks.Plugin {
	if p, ok := c.pluginCache[language]; ok {
		return p
	}
	if c.pluginCache == nil {
		c.pluginCache = make(map[string]codeblocks.Plugin)
	}
	for _, p := range c.codeBlockPlugins {
		if p.CanHandle(language) {
			c.pluginCache[language] = p
			return p
		}
	}
	c.pluginCache[language] = nil
	return nil
}

// ParseOption customizes Parse - see WithSyntaxHighlighter and
// WithCodeBlockPlugin.
type ParseOption func(*compiler)

// WithSyntaxHighlighter sets the Highlighter Parse uses to color code
// blocks token-by-token. Unset, code blocks render in one flat color.
func WithSyntaxHighlighter(h codeblocks.Highlighter) ParseOption {
	return func(c *compiler) {
		c.highlighter = h
	}
}

// WithCodeBlockPlugin registers one CodeBlockPlugin - callable more
// than once to register several; a fenced code block's language is
// matched against them in registration order (see
// compiler.pluginFor).
func WithCodeBlockPlugin(p codeblocks.Plugin) ParseOption {
	return func(c *compiler) {
		c.codeBlockPlugins = append(c.codeBlockPlugins, p)
	}
}
