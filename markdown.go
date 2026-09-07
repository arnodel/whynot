package whynot

// MarkdownCompiler compiles Markdown source into a Block/ASTNode tree -
// structure only, no appearance. See Parse.
type MarkdownCompiler struct {
	source []byte
}
