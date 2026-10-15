package whynot

import "github.com/arnodel/whynot/internal/engine"

// A selection is the View's state, like its scroll position: what the
// reader selected, between two places in the text. Within a paragraph,
// heading, table cell or code block, it's a range of characters; across
// them, it's whole blocks, as the reader would expect to copy: the list
// items from the one holding one end to the one holding the other, say,
// or whole table rows.

// selection is a View's selection, between anchor, where it started, and
// focus, where it ends, in tree, the selection tree of the View's blocks
// as of when it was made.
type selection struct {
	tree          *engine.SelTree
	treeVersion   int // the View's blocksVersion tree is of
	anchor, focus engine.Position
	active        bool
}

// selTree returns the selection tree of the View's blocks, carrying the
// selection over to it if the blocks have changed since, or clearing it
// if its ends are in blocks that were replaced.
func (v *View) selTree() *engine.SelTree {
	v.follow()
	if v.sel.tree != nil && v.sel.treeVersion == v.blocksVersion {
		return v.sel.tree
	}
	tree := engine.NewSelTree(v.blocks)
	if v.sel.active {
		a, ok1 := tree.Carry(v.sel.anchor)
		f, ok2 := tree.Carry(v.sel.focus)
		v.sel.anchor, v.sel.focus, v.sel.active = a, f, ok1 && ok2
	}
	v.sel.tree, v.sel.treeVersion = tree, v.blocksVersion
	return tree
}

// span returns the selection's normalized Span, if it selects anything.
func (v *View) span() (engine.Span, bool) {
	v.selTree()
	if !v.sel.active {
		return engine.Span{}, false
	}
	return engine.Normalize(v.sel.anchor, v.sel.focus)
}

// HasSelection reports whether anything is selected.
func (v *View) HasSelection() bool {
	_, ok := v.span()
	return ok
}

// SelectAll selects the whole document.
func (v *View) SelectAll() {
	tree := v.selTree()
	start, ok1 := tree.Start()
	end, ok2 := tree.End()
	v.setSelection(start, end, ok1 && ok2)
}

// ClearSelection selects nothing.
func (v *View) ClearSelection() {
	v.setSelection(engine.Position{}, engine.Position{}, false)
}

// setSelection sets the selection, between anchor and focus if active.
func (v *View) setSelection(anchor, focus engine.Position, active bool) {
	v.sel.anchor, v.sel.focus, v.sel.active = anchor, focus, active
}

// SelectedText returns the selection's plain text: the exact range within
// a block, or whole blocks separated by blank lines, list items by line
// breaks and keeping their markers, and table cells by tabs. It's empty if
// nothing is selected.
func (v *View) SelectedText() string {
	s, ok := v.span()
	if !ok {
		return ""
	}
	return s.Text()
}

// SelectedMarkdown returns the selection's Markdown: whole blocks as
// written; table rows, or cells of a row, as a table with its header; and
// within a block, the range widened to whole inline elements, such as
// emphasis or a link, so that their markup stays balanced. It's empty if
// nothing is selected.
func (v *View) SelectedMarkdown() string {
	s, ok := v.span()
	if !ok {
		return ""
	}
	return s.Markdown(v.doc.source())
}
