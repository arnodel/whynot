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
	TagImage
	TagUnsupported

	// TagCodeKeyword..TagCodeComment classify a syntax-highlighted code
	// token - see Highlighter. Appended after TagUnsupported deliberately:
	// headingTag() relies on TagHeading1..TagHeading6 staying consecutive,
	// so nothing may be inserted before them.
	TagCodeKeyword
	TagCodeString
	TagCodeNumber
	TagCodeComment
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

	// Destination is the URL a TagLink node points to. Empty for every
	// other tag.
	Destination string

	// ID is a heading's anchor id (goldmark's auto-generated slug, e.g.
	// "my-heading" for "## My Heading") - what a link's URL fragment
	// (the part after '#') targets. Empty for every non-heading tag.
	ID string

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

// HasAncestorTag reports whether n itself, or any of its ancestors, is
// tagged with tag - for style decisions that are a structural yes/no
// (e.g. "is this text struck through") rather than a value to resolve.
func (n *ASTNode) HasAncestorTag(tag ASTTag) bool {
	for ; n != nil; n = n.Parent {
		if n.Tag == tag {
			return true
		}
	}
	return false
}

// AncestorTag returns n itself, or the nearest ancestor, tagged with
// tag - or nil if neither n nor any ancestor is. The pointer-returning
// counterpart to HasAncestorTag, for callers that need the node itself
// (e.g. to compare identity against), not just whether one exists.
func (n *ASTNode) AncestorTag(tag ASTTag) *ASTNode {
	for ; n != nil; n = n.Parent {
		if n.Tag == tag {
			return n
		}
	}
	return nil
}

// HasAncestor reports whether n itself, or any of its ancestors, is
// target - matching a specific node's identity (e.g. "is this part of
// the currently hovered link") rather than a tag. A multi-word link's
// words don't all share one ASTNode when styled sub-spans are involved
// (e.g. a bold word inside a link gets its own child node), so matching
// by identity alone isn't enough - this walk is what makes matching the
// link's own node still catch every word inside it.
func (n *ASTNode) HasAncestor(target *ASTNode) bool {
	if target == nil {
		return false
	}
	for ; n != nil; n = n.Parent {
		if n == target {
			return true
		}
	}
	return false
}
