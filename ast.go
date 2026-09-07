package whynot

import "slices"

// ASTTag identifies the semantic role of an ASTNode - which part of a
// StyleSheet a value tagged with it should resolve its appearance
// against. Exported because StyleSheet implementations switch on it.
type ASTTag int

const (
	TagParagraph ASTTag = iota
	TagHeading1
	TagHeading2
	TagHeading3
	TagHeading4
	TagHeading5
	TagHeading6
	TagList
	TagListItem
	TagCodeBlock
	TagThematicBreak
	TagBlockquote
	TagTable
	TagTableCell
	TagLink
	TagCodeSpan
	TagEmphasis
	TagStrong
	TagStrikethrough
)

// ASTPath is a tag ancestry, root-to-leaf.
type ASTPath []ASTTag

// ASTNode is a node in the semantic tree built alongside the Block/Inline
// tree during compilation - structural only (what kind of thing this is,
// and where it sits), with no rendering or style baked in. Public because
// it's the input a StyleSheet queries, and potentially useful beyond
// styling later (document navigation, folding).
type ASTNode struct {
	Tag    ASTTag
	Parent *ASTNode

	path ASTPath // memoized on first Path() call
}

// AddChild creates a new ASTNode tagged with tag, parented to n, and
// returns it - the usual way to extend the tree during compilation.
// Works with a nil n (a fresh root node): the result only stores n as its
// Parent rather than dereferencing it.
func (n *ASTNode) AddChild(tag ASTTag) *ASTNode {
	return &ASTNode{Tag: tag, Parent: n}
}

// Path returns the tag ancestry from the root down to and including n - a
// convenience for StyleSheet implementations that want ancestry without
// walking Parent themselves. Computed once and cached.
//
// slices.Concat, not append(parentPath, n.Tag) - append can leave two
// siblings aliasing the same backing-array slot.
func (n *ASTNode) Path() ASTPath {
	if n == nil {
		return nil
	}
	if n.path == nil {
		n.path = slices.Concat(n.Parent.Path(), ASTPath{n.Tag})
	}
	return n.path
}
