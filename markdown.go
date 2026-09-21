package whynot

// MarkdownCompiler compiles Markdown source into a Block/ASTNode tree -
// structure only, no appearance. See Parse.
type MarkdownCompiler struct {
	source      []byte
	highlighter Highlighter

	// pendingSpace is true when a breakable space has been seen in the
	// source but not yet attached to the next appended Inline item - see
	// appendString. Reset to true at the start of each fresh run of
	// inline content (a paragraph, heading, list item head, or table
	// cell), since there's nothing for the first item there to glue to.
	pendingSpace bool
}
