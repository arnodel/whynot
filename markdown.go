package whynot

// MarkdownCompiler compiles Markdown source into a Block/ASTNode tree -
// structure only, no appearance. See Parse.
type MarkdownCompiler struct {
	source      []byte
	highlighter Highlighter

	// codeBlockPlugins is every CodeBlockPlugin registered via
	// WithCodeBlockPlugin, in registration order. pluginCache remembers
	// which one (if any) handles a given language, populated lazily by
	// pluginFor - so a document with many fences in the same language
	// only ever calls CanHandle once per plugin per language, not once
	// per fence. A cached nil value means "known: none handle it".
	codeBlockPlugins []CodeBlockPlugin
	pluginCache      map[string]CodeBlockPlugin

	// pendingSpace is true when a breakable space has been seen in the
	// source but not yet attached to the next appended Inline item - see
	// appendString. Reset to true at the start of each fresh run of
	// inline content (a paragraph, heading, list item head, or table
	// cell), since there's nothing for the first item there to glue to.
	pendingSpace bool

	// headings and soleImages accumulate the Document fields of the same
	// names as top-level blocks are compiled.
	headings   []TOCEntry
	soleImages map[Block]string
}

// pluginFor returns the first registered CodeBlockPlugin that handles
// language, or nil if none do - resolved once per distinct language and
// cached from then on.
func (c *MarkdownCompiler) pluginFor(language string) CodeBlockPlugin {
	if p, ok := c.pluginCache[language]; ok {
		return p
	}
	if c.pluginCache == nil {
		c.pluginCache = make(map[string]CodeBlockPlugin)
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
