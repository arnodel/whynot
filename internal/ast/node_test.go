package ast

import (
	"slices"
	"testing"
)

func TestNodePath(t *testing.T) {
	root := &Node{Tag: TagList}
	item := &Node{Tag: TagListItem, Parent: root}
	para := &Node{Tag: TagParagraph, Parent: item}

	got := para.Path()
	want := Path{TagList, TagListItem, TagParagraph}
	if !slices.Equal(got, want) {
		t.Errorf("Path() = %v, want %v", got, want)
	}
}

func TestNodePathRoot(t *testing.T) {
	root := &Node{Tag: TagParagraph}
	got := root.Path()
	want := Path{TagParagraph}
	if !slices.Equal(got, want) {
		t.Errorf("Path() = %v, want %v", got, want)
	}
}

func TestNodePathNil(t *testing.T) {
	var n *Node
	if got := n.Path(); got != nil {
		t.Errorf("Path() on nil = %v, want nil", got)
	}
}

// TestNodePathSiblingsDontAlias guards against a specific caching bug:
// if Path() cached its result via append(parentPath, tag) instead of
// make+copy, two children appending their own tag onto the same cached
// parent path - when that path has spare backing-array capacity, exactly
// what append's growth strategy can produce - would silently share a
// slot, and whichever child appended last would corrupt the other's
// already-returned slice. Constructed directly (with a manually-set
// parent.path carrying spare capacity) rather than relying on Go's actual
// growth behavior happening to produce spare capacity at some depth,
// which isn't guaranteed and didn't reliably reproduce it in practice.
func TestNodePathSiblingsDontAlias(t *testing.T) {
	parent := &Node{Tag: TagBlockquote}
	parent.path = make(Path, 1, 4) // len 1, cap 4: room for 3 more with no reallocation
	parent.path[0] = TagBlockquote

	child1 := &Node{Tag: TagParagraph, Parent: parent}
	child2 := &Node{Tag: TagList, Parent: parent}

	got1 := child1.Path()
	got2 := child2.Path()

	if want := (Path{TagBlockquote, TagParagraph}); !slices.Equal(got1, want) {
		t.Errorf("child1.Path() = %v, want %v", got1, want)
	}
	if want := (Path{TagBlockquote, TagList}); !slices.Equal(got2, want) {
		t.Errorf("child2.Path() = %v, want %v", got2, want)
	}
}
