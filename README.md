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
	"image/color"
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
	screen.Fill(color.Black) // whynot never fills its own background
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

## `cmd/whynot`: a standalone viewer

```
go run ./cmd/whynot path/to/some.md
```

Run with no argument and it opens a built-in welcome page (embedded in the
binary, `cmd/whynot/welcome.md`) explaining how to use it - including
pasting a file path or `http(s)` URL (Cmd/Ctrl+V) to open it, and pasting
the word "welcome" to come back. Beyond scrolling and resizing, it
demonstrates what a caller can build on top of the library:

- **`-light`** switches to the light theme at startup; **L**/**D** toggle
  between light and dark while running.
- **Hovering a link** highlights it (`View.Hover`).
- **Clicking a link follows it** - a relative path loads another local
  file, an `http(s)` URL fetches it (opening it in the system's default
  browser instead if its `Content-Type` turns out to be HTML rather than
  Markdown/plain text - a real webpage, not a `.md` file), and a URL
  fragment (`#some-heading`) scrolls to that heading
  (`View.ScrollToAnchor`), even on a document just navigated to. Either
  way it's resolved against the current document's own location
  (`net/url.URL.ResolveReference`), so a relative link works the same
  whether that document came from disk or from a fetch.
- **Backspace goes back** to wherever a link was followed from, exact
  scroll position included - whether that was a different document or
  just an in-page anchor jump.
- **Images resolve and load the same way** - a relative `src` is
  resolved against the document's own location, an `http(s)` one is
  fetched, regardless of whether the document itself came from disk or
  a fetch. Fetching and decoding happen in the background, never
  blocking rendering; a missing or undecodable image falls back to its
  alt text (or title, or a generic message) instead of a silent gap.
- **`-debug-hit`** outlines whatever `View.HitTest` resolves under the
  cursor, for debugging.
- A Markdown construct whynot doesn't understand (e.g. raw HTML) shows in
  a distinct color with a warning logged, instead of crashing the whole
  document - see [What's implemented](#whats-implemented).

None of the link-following/history logic lives in the library itself -
`whynot` only exposes the primitives (`View.Hover`, `LinkAt`,
`ScrollToAnchor`, `ScrollPosition`/`RestoreScrollPosition`); loading
files, fetching URLs, and keeping a history stack are all `cmd/whynot`'s
own, in [cmd/whynot/main.go](cmd/whynot/main.go). Image loading follows
the same split, one layer further in: the library defines `ImageSource`
(defaulting to a plain local file open) and owns caching the result
(`ImageCache` - an image is resolved, fetched, and decoded at most once,
however many times it's asked for, however many rendering backends ask
for it); `cmd/whynot` supplies the file-or-`http(s)`,
resolved-against-the-document's-location `ImageSource`, via
`WithImageSource`.

## What's implemented

- Headings (all 6 levels), paragraphs
- Emphasis, strong, and both together (`*x*`, `**x**`, `***x***`)
- Inline code, and fenced and indented code blocks
- Ordered and unordered lists, tight or loose, including task lists (`- [ ]`)
- Images - PNG/JPEG/GIF (including animated GIFs, disposal-correct and
  always looping), resolved and loaded via a pluggable `ImageSource`,
  fetched and decoded at most once per image regardless of how many
  times it's asked for (relative-to-document and `http(s)` paths both
  work, see `cmd/whynot` below), scaled with zoom/DPI like everything
  else; loading never blocks rendering, so a still-pending image shows
  a placeholder at its final size (or, until its size is even known, a
  "loading" fallback); a missing or undecodable image falls back to
  its alt text, then its title, then a generic message, instead of a
  silent gap - alt text itself is captured from arbitrary inline
  content, per CommonMark
- Thematic breaks (`---`)
- Links and autolinks, including reference-style (`[text][ref]`) -
  highlighted on hover (`View.Hover`), destination resolvable at a point
  (`View.LinkAt`); following one is up to the caller, see `cmd/whynot`
  below
- Heading anchors: goldmark's auto-generated heading ids, scrollable to
  via `View.ScrollToAnchor` - what a link's `#fragment` targets
- Blockquotes, including nested ones
- Strikethrough (`~~x~~`)
- Nested lists, to any depth
- Tables (GFM), including column alignment and negotiated column widths
- Scrolling, window resizing with reflow and scroll-position anchoring,
  and viewport culling - all handled by `whynot.View`
- Graceful degradation: a Markdown construct whynot doesn't recognize
  (e.g. raw HTML) logs a warning and renders as flagged, distinctly
  colored text/code showing its source, rather than crashing - a
  reference-style link's own `[ref]: url` definition line and an HTML
  comment are recognized as intentionally invisible rather than
  unsupported, since no Markdown renderer ever shows them either
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
- [x] Links and autolinks (highlighted on hover; following one - loading
      a new document, keeping history - is `cmd/whynot`'s job, not the
      library's, see above)
- [x] Blockquotes
- [x] Strikethrough
- [x] Nested lists

**Needs its own design pass:**
- [x] Tables - needed a genuine 2D layout primitive (`TableBox`, with
      negotiated column widths and row height = max of that row's cells),
      not an extension of the existing 1D `StackBox`/`ContainerBox`

**Lower priority:**
- [ ] Footnotes, definition lists - goldmark extensions for both exist
      but aren't enabled, so the syntax (`[^1]`, term/`: definition`)
      isn't recognized at all yet, rendering as plain literal text
- [ ] Raw inline/block HTML rendered as HTML - currently shown as
      flagged unsupported text instead (see
      [What's implemented](#whats-implemented)), not a crash, but not
      styled/laid out as real HTML would be either

## Known issues

See [ARCHITECTURE.md](ARCHITECTURE.md#known-issues).
