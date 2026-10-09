package whynot

import (
	"errors"
	"sync"
	"unicode/utf8"

	"github.com/arnodel/whynot/internal/engine"
	"github.com/arnodel/whynot/internal/markdown"
)

// Document is a parsed Markdown document, made by Parse or a Parser, and
// shown by any number of Views.
//
// A Document only grows, and only while it's being written (see
// [Parser.Stream]): blocks are added at its end, and the last few may be
// replaced while the text that makes them is still arriving; nothing
// before them changes. A Document from Parse is complete from the start,
// and never changes.
//
// A Document's methods are safe to call while it's being written, from
// any goroutine.
type Document struct {
	mu sync.Mutex

	// stream compiles the text written so far, when someone looks: see
	// update. It's nil for a Document from Parse.
	stream *markdown.Stream
	// finished holds the blocks that will never change, and tail the
	// unfinished ones at the end, which the next update replaces. version
	// changes whenever either does.
	finished []markdown.Piece
	tail     []markdown.Piece
	version  int

	// frontMatter is the document's front matter, so far.
	frontMatter string

	complete  bool
	listeners []chan struct{}
	// partial is the start of a UTF-8 character at the end of what was
	// written, held back until the rest of it arrives.
	partial []byte
}

// TOCEntry is one heading of a Document - enough to build a table of
// contents entry linking to ID.
type TOCEntry struct {
	ID    string // the heading's anchor id - what View.ScrollToAnchor takes
	Level int    // 1-6
	Text  string
}

// Title returns the text of the document's first heading, at any level,
// or ok=false if it has none: so far, for a Document still being written.
//
// Only a top-level heading is found: one nested inside a blockquote or
// list isn't. The same holds for TOCEntries and [View.ScrollToAnchor].
func (d *Document) Title() (string, bool) {
	entries := d.TOCEntries()
	if len(entries) == 0 {
		return "", false
	}
	return entries[0].Text, true
}

// RawFrontMatter returns the YAML of the document's front matter, as
// written, without its fences, or "" if it has none (or none so far, for a
// Document still being written). Front matter starts a document as in
// Jekyll or Hugo:
//
//	---
//	title: Release notes
//	tags: [go, markdown]
//	---
//
// It isn't shown, and whynot doesn't read it: it's for the program to
// decode, with the YAML library of its choice, as in
//
//	var meta struct{ Title string }
//	err := yaml.Unmarshal([]byte(doc.RawFrontMatter()), &meta)
//
// It's recognized as other tools do: a first line of "---", then lines of YAML, the first not
// blank, closed by a line of "---" or "..." within the first 100 lines. A
// document starting with a thematic break, "---" followed by a blank
// line, has no front matter.
func (d *Document) RawFrontMatter() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.update()
	return d.frontMatter
}

// TOCEntries returns every top-level heading in the document, in document
// order: so far, for a Document still being written.
func (d *Document) TOCEntries() []TOCEntry {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.update()
	var entries []TOCEntry
	for _, pieces := range [][]markdown.Piece{d.finished, d.tail} {
		for _, p := range pieces {
			if p.Heading != nil {
				entries = append(entries, TOCEntry(*p.Heading))
			}
		}
	}
	return entries
}

// Complete reports whether the Document has stopped growing: it was made
// by Parse, or its stream's writer was closed.
func (d *Document) Complete() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.complete
}

// Updates returns a channel that receives a value when text is written to
// the Document, and is closed when it's complete: for a program that
// draws only when something happens, to know when to draw again, as in
//
//	go func() {
//		for range doc.Updates() {
//			window.Invalidate()
//		}
//	}()
//
// Each call returns a channel of its own. Updates never wait for their
// receiver: while a value is waiting to be received, further ones are
// dropped, so a receiver learns that the Document has changed since it
// last looked, not how often. A complete Document's channel is closed
// from the start.
func (d *Document) Updates() <-chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	l := make(chan struct{}, 1)
	if d.complete {
		close(l)
	} else {
		d.listeners = append(d.listeners, l)
	}
	return l
}

// update compiles what has been written since it last ran, if anything.
// It's called with mu held, by whoever looks at the Document: parsing
// happens when someone looks, not on each write.
func (d *Document) update() {
	if d.stream == nil || !d.stream.Changed() {
		return
	}
	finished, tail := d.stream.Update()
	d.finished = append(d.finished, finished...)
	d.tail = tail
	d.frontMatter = d.stream.FrontMatter()
	d.version++
}

// write is the stream writer's Write: it stores p, and wakes listeners.
func (d *Document) write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.complete {
		return 0, errWriteComplete
	}
	text := append(d.partial, p...)
	// Hold back a character that's only partly written.
	cut := len(text)
	for i := len(text) - 1; i >= 0 && i >= len(text)-utf8.UTFMax; i-- {
		if utf8.RuneStart(text[i]) {
			if !utf8.FullRune(text[i:]) {
				cut = i
			}
			break
		}
	}
	d.stream.Write(text[:cut])
	d.partial = append([]byte(nil), text[cut:]...)
	for _, l := range d.listeners {
		select {
		case l <- struct{}{}:
		default: // one is waiting already
		}
	}
	return len(p), nil
}

// finish is the stream writer's Close: the Document stops growing, and
// its listeners' channels close.
func (d *Document) finish() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.complete {
		return
	}
	d.stream.Write(d.partial)
	d.partial = nil
	d.stream.Close()
	d.complete = true
	for _, l := range d.listeners {
		close(l)
	}
	d.listeners = nil
}

var errWriteComplete = errors.New("whynot: write to a complete Document")

// streamWriter writes a Document (see Parser.Stream).
type streamWriter struct {
	doc *Document
}

func (w streamWriter) Write(p []byte) (int, error) { return w.doc.write(p) }

func (w streamWriter) Close() error {
	w.doc.finish()
	return nil
}

// A reader is a position in a Document, as a View follows it: the
// finished blocks it has handed out, and the version it last saw. A
// Document can have any number of readers, and keeps no track of them.
type reader struct {
	doc     *Document
	seen    int
	version int
}

// next advances the reader to the Document as it stands: it returns the
// blocks finished since it last advanced, and the current tail, compiling
// first what has been written since. ok is false if nothing changed.
func (r *reader) next() (added, tail []markdown.Piece, ok bool) {
	d := r.doc
	d.mu.Lock()
	defer d.mu.Unlock()
	d.update()
	if d.version == r.version {
		return nil, nil, false
	}
	n := len(d.finished)
	added = d.finished[r.seen:n:n]
	r.seen, r.version = n, d.version
	return added, d.tail, true
}

// blocksOf returns pieces' blocks.
func blocksOf(pieces []markdown.Piece) []engine.Block {
	blocks := make([]engine.Block, len(pieces))
	for i, p := range pieces {
		blocks[i] = p.Block
	}
	return blocks
}
