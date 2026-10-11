package browser

import (
	"bytes"
	"sync"

	"github.com/arnodel/whynot"
	gmast "github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
)

// A whymark page's controls are what follows its last top-level thematic
// break ("---"): links that only make sense on the current page, such as
// "Play again" or a menu of actions. In journal mode, the earlier pages of
// the trail are shown without them.

// pageSource is a page's Markdown, kept as it arrives.
type pageSource struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *pageSource) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *pageSource) bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return bytes.Clone(s.buf.Bytes())
}

// controlsStart returns where a page's controls start in source: the start
// of its last top-level thematic break, or -1 if it has none.
func controlsStart(source []byte) int {
	doc := parser.New().Parse(source)
	start := -1
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if _, ok := n.(*gmast.ThematicBreak); ok {
			start = bytes.LastIndexByte(source[:n.Pos()], '\n') + 1
		}
	}
	return start
}

// trailView returns the View to show v in, as an earlier page of the
// journal's trail: for a complete whymark page with controls, a View of
// the page without them, made once; otherwise v itself.
func (a *App) trailView(v *whynot.View) *whynot.View {
	p := a.pageOf(v.Document())
	if p == nil || !v.Document().Complete() {
		return v
	}
	if p.full == v {
		return p.cut
	}
	source := p.source.bytes()
	start := controlsStart(source)
	if start < 0 {
		return v
	}
	doc := a.parser.Parse(source[:start], whynot.WithBaseURL(p.header.Location))
	// The same page: its links are followed from it.
	a.pages[doc] = p
	p.cut, p.full = a.NewView(doc), v
	return p.cut
}

// fullView returns the page v shows, whole: v itself, unless it's the View
// of a page without its controls (see trailView).
func (a *App) fullView(v *whynot.View) *whynot.View {
	if p := a.pageOf(v.Document()); p != nil && p.cut == v {
		return p.full
	}
	return v
}
