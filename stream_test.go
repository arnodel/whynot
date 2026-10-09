package whynot

import (
	"fmt"
	"image"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/internal/ast"
	"github.com/arnodel/whynot/internal/canvastest"
	"github.com/arnodel/whynot/internal/engine"
	"github.com/arnodel/whynot/internal/styling/stylingtest"
)

// trickyMarkdown has constructs whose meaning later lines change: a
// paragraph that becomes a heading, a list that continues across blank
// lines and becomes loose, a fence that swallows what follows until it's
// closed, a table, nested quotes, and a reference definition before its
// use.
const trickyMarkdown = `Intro paragraph
spanning lines
===

[ref]: https://example.com "Title"

- one
- two

- three, now loose
  continued

> quote
> > nested
lazy continuation

` + "```go\nfunc main() {\n\n}\n```" + `

| A | B |
|---|---|
| *x* | [link][ref] |

Setext two
---

1. first
2. second
` + "    indented code\n" + `
## Intro paragraph spanning lines

A last paragraph with **bold** and ` + "`code`" + `.
`

// rendering draws blocks, laid out as a document at a fixed width, and
// returns what was drawn where.
func rendering(blocks []engine.Block) string {
	ctx := engine.Context{Scale: 1, FaceSelector: fonts.NewGoSelector(), Styles: stylingtest.Basic()}
	box := (&engine.StackBlock{Blocks: blocks}).StackLayout(&ctx, 400)
	rec := &canvastest.Recorder{Area: image.Rect(0, 0, 400, 1<<30)}
	box.DrawFrom(rec, engine.StackCursor{}, 0, 0, 0)
	var b strings.Builder
	for _, t := range rec.Texts {
		fmt.Fprintf(&b, "%d,%d %q\n", t.X, t.Y, t.S)
	}
	for _, r := range rec.Rects {
		fmt.Fprintf(&b, "rect %d,%d %dx%d %v\n", r.X, r.Y, r.W, r.H, r.Color)
	}
	return b.String()
}

// current returns doc's blocks as they stand, finished and unfinished.
func current(doc *Document) []engine.Block {
	doc.mu.Lock()
	defer doc.mu.Unlock()
	doc.update()
	return append(blocksOf(doc.finished), blocksOf(doc.tail)...)
}

// TestStreamMatchesParse checks the property progressive documents rest
// on: written a piece at a time, a Document always shows what Parse would
// make of the text so far, and its finished blocks never change.
func TestStreamMatchesParse(t *testing.T) {
	demo, err := os.ReadFile("testdata/demo.md")
	if err != nil {
		t.Fatal(err)
	}
	// Its reference definition comes after its use: see
	// TestStreamLateReferenceDefinition.
	demoText := strings.Replace(string(demo), "[ref]: https://github.com/arnodel/whynot", "", 1)
	contributing, err := os.ReadFile("CONTRIBUTING.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name   string
		text   string
		pieces []int // sizes of successive writes, cycled
	}{
		{"tricky, a byte at a time", trickyMarkdown, []int{1}},
		{"demo", demoText, []int{3, 50, 1, 200, 17, 90}},
		{"contributing", string(contributing), []int{5, 64, 1, 300}},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc, w := NewParser().Stream()
			var finished []engine.Block
			written := 0
			for i := 0; written < len(c.text); i++ {
				n := min(c.pieces[i%len(c.pieces)], len(c.text)-written)
				io.WriteString(w, c.text[written:written+n])
				written += n

				got := current(doc)
				if want := rendering(blocksOf(Parse([]byte(c.text[:written])).finished)); rendering(got) != want {
					t.Fatalf("after %d bytes, the document differs from Parse of the same text:\n%q", written, c.text[:written])
				}
				doc.mu.Lock()
				now := blocksOf(doc.finished)
				doc.mu.Unlock()
				for j, b := range finished {
					if now[j] != b {
						t.Fatalf("after %d bytes, finished block %d changed", written, j)
					}
				}
				finished = now
			}
			w.Close()
			if got, want := rendering(current(doc)), rendering(blocksOf(Parse([]byte(c.text)).finished)); got != want {
				t.Error("once complete, the document differs from Parse")
			}
			if !doc.Complete() {
				t.Error("Complete() = false after Close")
			}
		})
	}
}

// TestStreamLateReferenceDefinition pins down the one known difference
// from Parse: a reference definition that arrives after the link using it
// has been finished doesn't make it a link. Written in one go, the same
// text comes out as Parse makes it.
func TestStreamLateReferenceDefinition(t *testing.T) {
	const text = "See [the site][ref].\n\nMore.\n\n[ref]: https://example.com\n"
	links := func(blocks []engine.Block) int {
		n := 0
		for _, b := range blocks {
			if hasLink(b) {
				n++
			}
		}
		return n
	}
	if links(blocksOf(Parse([]byte(text)).finished)) != 1 {
		t.Fatal("test setup: Parse makes no link")
	}

	doc, w := NewParser().Stream()
	io.WriteString(w, text[:strings.Index(text, "[ref]:")])
	current(doc) // a View looks: the first paragraph is finished, without a link
	io.WriteString(w, text[strings.Index(text, "[ref]:"):])
	w.Close()
	if links(current(doc)) != 0 {
		t.Error("the late definition made a finished paragraph a link: the limitation is gone, update the docs")
	}

	doc, w = NewParser().Stream()
	io.WriteString(w, text)
	w.Close()
	if links(current(doc)) != 1 {
		t.Error("written in one go, the text isn't parsed as Parse does")
	}
}

// hasLink reports whether block, a paragraph, holds a link.
func hasLink(block engine.Block) bool {
	tb, ok := unwrap(block).(*engine.TextBlock)
	if !ok {
		return false
	}
	for _, part := range tb.Parts {
		if n := part.Node(); n != nil && n.AncestorTag(ast.TagLink) != nil {
			return true
		}
	}
	return false
}

// TestStreamUTF8 checks that a character split between writes comes out
// whole.
func TestStreamUTF8(t *testing.T) {
	doc, w := NewParser().Stream()
	text := []byte("Café ☕\n")
	for i := range text {
		w.Write(text[i : i+1])
		if s := rendering(current(doc)); strings.ContainsRune(s, '�') {
			t.Fatalf("after %d bytes, a broken character shows: %s", i+1, s)
		}
	}
	w.Close()
	if got, want := rendering(current(doc)), rendering(blocksOf(Parse(text).finished)); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if _, err := w.Write([]byte("x")); err == nil {
		t.Error("writing after Close succeeded")
	}
}

// TestStreamUpdates checks that every listener learns of a write, that one
// that doesn't read never blocks the writer, and that every channel closes
// at completion, including one asked for afterwards.
func TestStreamUpdates(t *testing.T) {
	doc, w := NewParser().Stream()
	a, b := doc.Updates(), doc.Updates()
	for range 100 {
		io.WriteString(w, "word ") // nobody reads b: this mustn't block
	}
	if _, ok := <-a; !ok {
		t.Fatal("a closed before completion")
	}
	select {
	case <-a:
		t.Error("a hundred writes left more than one update waiting")
	default:
	}
	w.Close()
	for _, ch := range []<-chan struct{}{a, b, doc.Updates()} {
		for range ch { // drain what's waiting, then the loop ends at close
		}
	}
	if !doc.Complete() {
		t.Error("Complete() = false after Close")
	}
}

// TestStreamConcurrency writes a Document on one goroutine while a View
// follows it on another: run with -race.
func TestStreamConcurrency(t *testing.T) {
	doc, w := NewParser().Stream()
	v := NewView(doc)
	v.SetBounds(image.Rect(0, 0, 300, 400))
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range 200 {
			fmt.Fprintf(w, "Line %d of a paragraph that grows.\n", i)
			if i%10 == 9 {
				io.WriteString(w, "\n")
			}
		}
		w.Close()
	}()
	for range doc.Updates() {
		v.Draw(&canvastest.Recorder{Area: image.Rect(0, 0, 300, 400)}, 0)
	}
	wg.Wait()
	v.Draw(&canvastest.Recorder{Area: image.Rect(0, 0, 300, 400)}, 0)
	if got, want := len(v.blocks), len(Parse([]byte(strings.Repeat("x\n\n", 20))).finished); got != want {
		t.Errorf("the View has %d blocks, want %d paragraphs", got, want)
	}
}

// TestViewFollowsKeepingLayout checks that a View following a growing
// Document keeps the layout of blocks it already laid out, and its scroll
// position.
func TestViewFollowsKeepingLayout(t *testing.T) {
	doc, w := NewParser().Stream()
	v := NewView(doc)
	v.SetBounds(image.Rect(0, 0, 300, 200))
	for i := range 30 {
		fmt.Fprintf(w, "Paragraph %d.\n\n", i)
	}
	draw := func() { v.Draw(&canvastest.Recorder{Area: image.Rect(0, 0, 300, 200)}, 0) }
	draw()
	v.ScrollBy(150)
	scroll := v.ScrollPosition()
	// The first slot that's a block's, not the top margin's.
	firstBlock := 0
	for v.stack.blockAt(firstBlock) == nil {
		firstBlock++
	}
	first := v.stack.box.BoxAt(firstBlock)

	io.WriteString(w, "One more.\n\n")
	draw()
	if v.stack.box.BoxAt(firstBlock) != first {
		t.Error("growing laid out the first block again")
	}
	if v.ScrollPosition() != scroll {
		t.Errorf("growing moved the scroll position from %v to %v", scroll, v.ScrollPosition())
	}
}
