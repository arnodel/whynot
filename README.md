# Why Not?

A Markdown renderer for [ebiten](https://ebitengine.org/) games, written in
Go. Point it at a `[]byte` of Markdown and it lays out and draws the
document onto the screen - scrolling, resizing, and reflow handled for you.

```
go run ./cmd/whynot path/to/some.md
```

Use the mouse wheel to scroll; resize the window to see it reflow.

## Why does this exist?

Games sometimes need to show a chunk of formatted text - patch notes, an
in-game journal, help screens, a credits scroll - without pulling in a full
UI toolkit. Why Not renders straight onto an `ebiten.Image`, using
[goldmark](https://github.com/yuin/goldmark) to parse.

## Status

Early and incomplete - see [What's implemented](#whats-implemented) and the
[TODO](#todo) below. The API may still change. Architecture, internal
layout model, and the reasoning behind some of the trickier bits (lazy
layout, scroll anchoring) are documented in
[ARCHITECTURE.md](ARCHITECTURE.md).

## Using it as a library

The library (root package `whynot`) has no rendering backend dependency -
it only depends on `goldmark` for parsing. `ebitenrenderer` implements
`whynot.Canvas` on top of `ebiten`; a caller wires the two together:

```go
package main

import (
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/ebitenrenderer"
)

type game struct {
	view     *whynot.View
	renderer *ebitenrenderer.Renderer
}

func (g *game) Update() error {
	_, dy := ebiten.Wheel()
	g.view.Scroll(dy * 2)
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	g.view.Draw(g.renderer.NewCanvas(screen), 0, 0)
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	g.view.Layout(outsideWidth, 1)
	return outsideWidth, outsideHeight
}

func main() {
	source, err := os.ReadFile("doc.md")
	if err != nil {
		log.Fatal(err)
	}
	g := &game{
		view:     whynot.NewView(source, whynot.NewGoFontFaceSelector(72)),
		renderer: ebitenrenderer.New(),
	}
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
```

`cmd/whynot/main.go` is the fuller version of this, handling display scale
too.

## What's implemented

- Headings (all 6 levels), paragraphs
- Emphasis, strong, and both together (`*x*`, `**x**`, `***x***`)
- Inline code, and fenced and indented code blocks
- Ordered and unordered (tight) lists, including task lists (`- [ ]`)
- Images, including a title attribute
- Thematic breaks (`---`)
- Links and autolinks (rendered in a distinct color; not clickable yet)
- Blockquotes, including nested ones
- Strikethrough (`~~x~~`)
- Scrolling, window resizing with reflow and scroll-position anchoring,
  and viewport culling - all handled by `whynot.View`
- Large documents: layout and drawing are lazy, built outward from the
  current scroll position rather than the whole document, so cost tracks
  what's on screen, not the document's total size - a resize deep into a
  ~1000-line document costs microseconds, not tens of milliseconds. See
  [ARCHITECTURE.md](ARCHITECTURE.md#view-tying-the-layers-together-with-the-right-lifecycle)
  for how.

## TODO

Roughly in the order I'd tackle them - cheaper and more self-contained
first, with tables called out separately since it needs real design work
rather than being an incremental addition.

**Quick wins, no new layout concepts:**
- [x] Indented code blocks (fenced already works; this reuses the same path)
- [x] Thematic breaks (`---`)
- [ ] Typographer (smart quotes/dashes) - blocked on a real gap in the
      inline model: goldmark emits the substitution as a separate Text
      node with no whitespace from its neighbor (e.g. `Alice's` ->
      `"Alice"`, `"'"`, `"s "` as three siblings), but `appendString` word-
      splits each sibling independently, so adjacent no-space siblings
      would render as separately-spaced words. Needs word-adjacency
      tracking across sibling Inlines first, not scoped to this feature
- [x] Task lists (`- [ ]`)

**One new concept each:**
- [x] Links and autolinks (rendered distinctly; click-through needs input
      plumbing this library doesn't have yet)
- [x] Blockquotes
- [x] Strikethrough
- [ ] Nested lists - currently a *silent* gap: a list item's content past
      its first child is dropped rather than causing an error

**Needs its own design pass:**
- [ ] Tables - every layout primitive today (`StackBox`, `ContainerBox`)
      assumes a single 1D vertical stack; a table needs a genuine 2D
      layout primitive (column widths, row height = max of that row's
      cells), not an extension of the existing ones

**Lower priority:**
- [ ] Footnotes, definition lists, raw inline/block HTML

## Known issues

See [ARCHITECTURE.md](ARCHITECTURE.md#known-issues) - notably, an empty
document or list, or any unsupported Markdown construct, currently
`panic`s rather than degrading gracefully.
