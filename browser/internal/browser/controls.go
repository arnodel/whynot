package browser

import (
	"bytes"
	"regexp"
	"sync"

	"github.com/arnodel/whynot"
	gmast "github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
)

// A whymark page's controls are what follows its controls heading, a
// top-level "## (Controls)", the last one if there are several: what only
// makes sense on the current page, such as "Play again" or a menu of
// actions. The heading is quiet: it's never shown. In journal mode, the
// earlier pages of the trail are shown without their controls.

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

// controlsHeading matches a controls heading's line.
var controlsHeading = regexp.MustCompile(`(?i)^ {0,3}##[ \t]+\([ \t]*controls[ \t]*\)[ \t]*#*[ \t]*$`)

// controlsStart returns where a page's controls start in source, its
// Markdown: the start of its last controls heading, or -1 if it has none.
func controlsStart(source []byte) int {
	doc := parser.New().Parse(source)
	start := -1
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if h, ok := n.(*gmast.Heading); ok && h.Level == 2 {
			lineStart := bytes.LastIndexByte(source[:n.Pos()], '\n') + 1
			line, _, _ := bytes.Cut(source[lineStart:], []byte("\n"))
			if controlsHeading.Match(bytes.TrimRight(line, "\r")) {
				start = lineStart
			}
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
