package whynot

import (
	"slices"
	"testing"
)

func TestASTNodePath(t *testing.T) {
	root := &ASTNode{Tag: TagList}
	item := &ASTNode{Tag: TagListItem, Parent: root}
	para := &ASTNode{Tag: TagParagraph, Parent: item}

	got := para.Path()
	want := ASTPath{TagList, TagListItem, TagParagraph}
	if !slices.Equal(got, want) {
		t.Errorf("Path() = %v, want %v", got, want)
	}
}

func TestASTNodePathRoot(t *testing.T) {
	root := &ASTNode{Tag: TagParagraph}
	got := root.Path()
	want := ASTPath{TagParagraph}
	if !slices.Equal(got, want) {
		t.Errorf("Path() = %v, want %v", got, want)
	}
}

func TestASTNodePathNil(t *testing.T) {
	var n *ASTNode
	if got := n.Path(); got != nil {
		t.Errorf("Path() on nil = %v, want nil", got)
	}
}

// TestASTNodePathSiblingsDontAlias guards against a specific caching bug:
// if Path() cached its result via append(parentPath, tag) instead of
// make+copy, two children appending their own tag onto the same cached
// parent path - when that path has spare backing-array capacity, exactly
// what append's growth strategy can produce - would silently share a
// slot, and whichever child appended last would corrupt the other's
// already-returned slice. Constructed directly (with a manually-set
// parent.path carrying spare capacity) rather than relying on Go's actual
// growth behavior happening to produce spare capacity at some depth,
// which isn't guaranteed and didn't reliably reproduce it in practice.
func TestASTNodePathSiblingsDontAlias(t *testing.T) {
	parent := &ASTNode{Tag: TagBlockquote}
	parent.path = make(ASTPath, 1, 4) // len 1, cap 4: room for 3 more with no reallocation
	parent.path[0] = TagBlockquote

	child1 := &ASTNode{Tag: TagParagraph, Parent: parent}
	child2 := &ASTNode{Tag: TagList, Parent: parent}

	got1 := child1.Path()
	got2 := child2.Path()

	if want := (ASTPath{TagBlockquote, TagParagraph}); !slices.Equal(got1, want) {
		t.Errorf("child1.Path() = %v, want %v", got1, want)
	}
	if want := (ASTPath{TagBlockquote, TagList}); !slices.Equal(got2, want) {
		t.Errorf("child2.Path() = %v, want %v", got2, want)
	}
}
