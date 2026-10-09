package markdown

import (
	"bytes"

	gmast "github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"

	"github.com/arnodel/whynot/fetch"
	"github.com/arnodel/whynot/internal/engine"
)

// A Piece is a compiled top-level block, with what the compiler recorded
// about it.
type Piece struct {
	Block engine.Block
	// Heading is set if Block is a heading.
	Heading *Heading
	// SoleImage is set if Block is a text block consisting of a single
	// image, whose Source was resolved.
	SoleImage fetch.Source
}

// A Stream compiles a document that arrives a piece at a time, compiling
// each top-level block once it's finished, and only the unfinished end
// again as more arrives. It isn't safe for concurrent use.
//
// A top-level block is finished once the text holds, in complete lines,
// the start of another top-level block after it: CommonMark closes a
// block when a line arrives that can't continue it, and never reopens it.
// The unfinished last line never decides what's finished: a lone "-"
// parses as a list item, but completed as "---" it makes the paragraph
// above a heading.
//
// Each parse of the unfinished end is seeded with the finished part's
// heading ids and reference definitions, so the result is that of a
// parse of the whole text, but for one difference: a reference
// definition after the links that use it doesn't make them links.
type Stream struct {
	opts Options
	text []byte
	// start is where the unfinished end of text starts: the blocks before
	// it are finished.
	start int
	// parsed is len(text) as of the last Update, and done whether Update
	// has run since Close.
	parsed       int
	closed, done bool
	// ids and defs are the finished part's heading ids and reference
	// definitions, which each parse is seeded with.
	ids  []string
	defs []parser.LinkDefinition
}

// NewStream returns an empty Stream, which compiles with opts.
func NewStream(opts Options) *Stream {
	return &Stream{opts: opts}
}

// Write appends text.
func (s *Stream) Write(p []byte) {
	s.text = append(s.text, p...)
}

// Close marks the end of the text: the next Update finishes every block.
func (s *Stream) Close() {
	s.closed = true
}

// Changed reports whether Update has anything to do: text has been
// written, or the Stream closed, since it last ran.
func (s *Stream) Changed() bool {
	return s.parsed != len(s.text) || s.closed && !s.done
}

// Update compiles what has arrived since it last ran. It returns the
// blocks finished since then, and the unfinished ones at the end, which
// replace those it returned last time. After Close, every block is
// finished, and tail is empty.
func (s *Stream) Update() (finished, tail []Piece) {
	s.parsed = len(s.text)
	if s.closed {
		s.done = true
		src := s.text[s.start:]
		nodes, pc := s.parse(src)
		finished = s.compileNodes(src, nodes)
		s.finish(finished, pc)
		s.start = len(s.text)
		return finished, nil
	}
	// Only complete lines decide what's finished.
	if nl := bytes.LastIndexByte(s.text[s.start:], '\n'); nl >= 0 {
		src := s.text[s.start : s.start+nl+1]
		if nodes, pc := s.parse(src); len(nodes) >= 2 {
			last := nodes[len(nodes)-1]
			finished = s.compileNodes(src, nodes[:len(nodes)-1])
			s.finish(finished, pc)
			s.start += bytes.LastIndexByte(src[:last.Pos()], '\n') + 1
		}
	}
	src := s.text[s.start:]
	nodes, _ := s.parse(src)
	return finished, s.compileNodes(src, nodes)
}

// parse parses src, part of the text from its unfinished end, seeded with
// the finished part's heading ids and reference definitions, and returns
// its top-level block nodes.
func (s *Stream) parse(src []byte) ([]gmast.Node, parser.Context) {
	pc := parser.NewContext()
	for _, id := range s.ids {
		pc.IDs().Put([]byte(id))
	}
	for _, def := range s.defs {
		pc.AddLinkDefinition(def)
	}
	root := newParser().Parse(src, parser.WithContext(pc))
	var nodes []gmast.Node
	for node := root.FirstChild(); node != nil; node = node.NextSibling() {
		if _, ok := node.(gmast.BlockNode); ok {
			nodes = append(nodes, node)
		}
	}
	return nodes, pc
}

// compileNodes compiles nodes, top-level block nodes of a parse of src,
// dropping any that show nothing.
func (s *Stream) compileNodes(src []byte, nodes []gmast.Node) []Piece {
	c := newCompiler(src, s.opts)
	var pieces []Piece
	for _, node := range nodes {
		headings := len(c.headings)
		// A nil parent ast.Node is what marks a block as top-level.
		block := c.compileBlock(node, nil)
		if block == nil {
			continue
		}
		piece := Piece{Block: block, SoleImage: c.soleImages[block]}
		if len(c.headings) > headings {
			h := c.headings[len(c.headings)-1]
			piece.Heading = &h
		}
		pieces = append(pieces, piece)
	}
	return pieces
}

// finish keeps what later parses need from newly finished blocks: their
// heading ids, and the reference definitions of their parse, pc.
func (s *Stream) finish(finished []Piece, pc parser.Context) {
	for _, p := range finished {
		if p.Heading != nil {
			s.ids = append(s.ids, p.Heading.ID)
		}
	}
	s.defs = pc.LinkDefinitions()
}
