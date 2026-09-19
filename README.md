# Why Not?

A Markdown renderer for [ebiten](https://ebitengine.org/) games, written in
Go. Point it at a `[]byte` of Markdown and it lays out and draws the
document onto the screen - scrolling, resizing, and reflow handled for you.

```
go run ./cmd/whynot path/to/some.md
```

Use the mouse wheel to scroll; resize the window to see it reflow.

## Try the standalone viewer

On macOS or Linux:

```bash
brew install arnodel/tap/whynot
```

Otherwise, grab a binary for your platform from the
[latest release](https://github.com/arnodel/whynot/releases/latest), or build
it yourself:

```bash
go install github.com/arnodel/whynot/cmd/whynot@latest
```

Run it with no argument and it opens a built-in welcome page explaining how
to use it.

## Why does this exist?

Games sometimes need to show a chunk of formatted text - patch notes, an
in-game journal, help screens, a credits scroll - without pulling in a full
UI toolkit. Why Not renders straight onto an `ebiten.Image`, using
[goldmark](https://github.com/yuin/goldmark) to parse.

## Status

Early and incomplete - see [Features](#features) below for what's done and what
isn't. The API may still change. Architecture, internal
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
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/ebitenrenderer"
)

type game struct {
	view     *whynot.View
	renderer *ebitenrenderer.Renderer
	start    time.Time
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
	// The third argument is elapsed time since rendering started - only
	// animated images actually need it (see whynot.RenderingContext.Time).
	g.view.Layout(outsideWidth, 1, time.Since(g.start))
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
		start:    time.Now(),
	}
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
```

[`cmd/whynot`](cmd/whynot) is the fuller version of this, handling display
scale too.

## Styling

Appearance - colors, margins, text sizes/weights, and a handful of dimensional
constants (table/blockquote geometry, rule and strikethrough thickness) - is entirely
driven by the `StyleSheet` interface ([stylesheet.go](stylesheet.go)), resolved per node
when the layout tree is built rather than hardcoded anywhere. `DefaultStyleSheet` is the
configurable built-in implementation; `NewDarkStyleSheet()`/`NewLightStyleSheet()` (what
`cmd/whynot`'s theme toggle switches between) are both just different field values on the
same struct - tweak one (`s := whynot.NewDarkStyleSheet(); s.LinkColor = myColor`) and
pass it via `whynot.WithStyleSheet(s)`, or embed `DefaultStyleSheet` in your own type and
override individual methods for full control over one aspect (e.g. per-node margins)
without reimplementing the rest. Swapping a `View`'s `StyleSheet` at runtime
(`View.SetStyleSheet`) re-lays-out the document immediately, which is all `cmd/whynot`'s
theme toggle button does.

## `cmd/whynot`: a standalone viewer

```
go run ./cmd/whynot path/to/some.md
```

Run with no argument and it opens a built-in welcome page (embedded in the
binary, `cmd/whynot/assets/welcome.md`) explaining how to use it - including
pasting a file path or `http(s)` URL (Cmd/Ctrl+V) to open it, and pasting
the word "welcome" to come back. Beyond scrolling and resizing, it
demonstrates what a caller can build on top of the library:

- **`-light`** switches to the light theme at startup; the toolbar button
  toggles between light and dark while running.
- **Dragging the scrollbar** jumps to a position (`View.ScrollToRatio`) -
  it self-corrects toward wherever the mouse currently is as
  not-yet-resolved parts of the document get resolved during the drag,
  rather than drifting away from the cursor. **↓**/**↑** nudge the scroll
  position a bit at a time, repeating while held.
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
- **`-debug-stats`** (togglable at runtime with **F**) shows FPS/TPS and
  per-frame Update/Draw timing.
- A Markdown construct whynot doesn't understand (e.g. raw HTML) shows in
  a distinct color with a warning logged, instead of crashing the whole
  document - see [Features](#features).

None of the link-following/history logic lives in the library itself -
`whynot` only exposes the primitives (`View.Hover`, `LinkAt`,
`ScrollToAnchor`, `ScrollPosition`/`RestoreScrollPosition`); loading
files, fetching URLs, and keeping a history stack are all `cmd/whynot`'s
own (`load.go`, `navigate.go`). Image loading follows
the same split, one layer further in: the library defines `ImageSource`
(defaulting to a plain local file open) and owns caching the result
(`ImageCache` - an image is resolved, fetched, and decoded at most once,
however many times it's asked for, however many rendering backends ask
for it); `cmd/whynot` supplies the file-or-`http(s)`,
resolved-against-the-document's-location `ImageSource`, via
`WithImageSource`.

## Features

Checked items are implemented; unchecked ones aren't yet. Grouped by theme rather than
by implementation order now that most of the list is done.

**Text and inline formatting**
- [x] Headings (all 6 levels), paragraphs
- [x] Emphasis, strong, and both together (`*x*`, `**x**`, `***x***`)
- [x] Strikethrough (`~~x~~`)
- [x] Inline code
- [ ] Typographer (smart quotes/dashes) - blocked on a real gap in the inline model:
      goldmark emits the substitution as a separate Text node with no whitespace from
      its neighbor (e.g. `Alice's` -> `"Alice"`, `"'"`, `"s "` as three siblings), but
      `appendString` word-splits each sibling independently, so adjacent no-space
      siblings would render as separately-spaced words - needs word-adjacency tracking
      across sibling Inlines first

**Block structures**
- [x] Fenced and indented code blocks
- [x] Blockquotes, including nested ones
- [x] Thematic breaks (`---`)
- [x] Ordered and unordered lists, tight or loose, nested to any depth, including task
      lists (`- [ ]`)
- [x] Tables (GFM), including column alignment and negotiated column widths - needed a
      genuine 2D layout primitive (`TableBox`, row height = max of that row's cells),
      not an extension of the existing 1D `StackBox`/`ContainerBox`
- [ ] Footnotes, definition lists - goldmark extensions for both exist but aren't
      enabled, so the syntax (`[^1]`, term/`: definition`) isn't recognized at all yet,
      rendering as plain literal text

**Images**
- [x] PNG/JPEG/GIF, including animated GIFs (disposal-correct compositing, always
      looping) - resolved and loaded via a pluggable `ImageSource`, fetched and decoded
      at most once per image regardless of how many times it's asked for; alt text is
      captured from arbitrary inline content, per CommonMark
- [x] Loading never blocks rendering - a still-pending image shows a placeholder at its
      final size once known (or a "loading" fallback before that); a missing or
      undecodable image falls back to alt text, then title, then a generic message
- [ ] Prefetch images ahead of the scroll position - today an image only starts loading
      once its containing slot is actually resolved (in practice, scrolled near), not
      when the document is first opened

**Links and navigation**
- [x] Links and autolinks, including reference-style (`[text][ref]`) - highlighted on
      hover (`View.Hover`), destination resolvable at a point (`View.LinkAt`)
- [x] Heading anchors: goldmark's auto-generated heading ids, scrollable to via
      `View.ScrollToAnchor` - what a link's `#fragment` targets
- [ ] Raw inline/block HTML rendered as HTML - currently shown as flagged unsupported
      text instead, not a crash, but not styled/laid out as real HTML would be either

**Rendering and performance**
- [x] Scrolling, window resizing with reflow and scroll-position anchoring, and
      viewport culling - all handled by `whynot.View`
- [x] `View.DocumentBounds`/`VisibleViewBounds` expose real per-slot pixel-height
      geometry (each top-level slot's height, once resolved; extrapolated from the
      average of what's known for the rest, refined as more of the document is visited)
      for a caller to build its own scrollbar, or feed to any other UI it wants to drive
      from scroll position. `View.ScrollToRatio` is the other direction - a caller
      driving a scrollbar thumb drag recomputes the ratio from the mouse's current
      position every frame rather than a target captured once, so a jump into
      not-yet-resolved territory only ever corrects toward the cursor, never drifts
- [x] Large documents: layout and drawing are lazy, built outward from the current
      scroll position rather than the whole document, so cost tracks what's on screen,
      not the document's total size - a resize deep into a ~1000-line document costs
      microseconds, not tens of milliseconds. See
      [ARCHITECTURE.md](ARCHITECTURE.md#view-tying-the-layers-together-with-the-right-lifecycle)
      for how
- [x] Graceful degradation: a Markdown construct whynot doesn't recognize (e.g. raw
      HTML) logs a warning and renders as flagged, distinctly colored text/code showing
      its source, rather than crashing - a reference-style link's own `[ref]: url`
      definition line and an HTML comment are recognized as intentionally invisible
      rather than unsupported, since no Markdown renderer ever shows them either

**Styling**
- [x] Fully customizable via the `StyleSheet` interface (see [above](#styling)) -
      colors, margins, text styles/sizes, and dimensional constants all resolve through
      it; override one field on `DefaultStyleSheet`, or embed it in a custom
      `StyleSheet` for full control. Swappable at runtime (`View.SetStyleSheet`) -
      `cmd/whynot`'s light/dark toggle is just two `DefaultStyleSheet` instances

**`cmd/whynot`, the standalone viewer**
- [x] Built-in welcome page, shown by default, explaining how to use the app
- [x] Paste a file path or `http(s)` URL to open it; paste "welcome" to return here
- [x] A link to a webpage opens in the system browser instead of failing; a link to
      Markdown opens in whynot itself (see [above](#cmdwhynot-a-standalone-viewer))
- [x] Back/forward history, light/dark theme, zoom
- [x] Scrollbar - drawn by `cmd/whynot` itself (`drawScrollbar`, `draw.go`), not the
      library: `View.DocumentBounds`/`VisibleViewBounds` expose the geometry instead, so
      an embedder using its own UI framework (or wanting a native scrollbar widget) can
      build whatever it wants rather than being stuck with the library's own opinion.
      Draggable (`View.ScrollToRatio`), with hover/drag color feedback and a
      theme-aware color (a light thumb on the dark theme's near-black background would
      be invisible against the light theme's white one, and vice versa)
- [ ] A real app icon instead of the generic terminal one when launched as a bundled
      macOS/Windows/Linux app

## Known issues

See [ARCHITECTURE.md](ARCHITECTURE.md#known-issues).
