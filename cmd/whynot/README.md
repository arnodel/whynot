# whynot, the viewer

A Markdown document viewer built on the [whynot](../../README.md) library and
[Ebitengine](https://ebitengine.org/): open a file or a URL, follow links between
documents, go back and forward, zoom, switch themes. Its Gio twin,
[`giowhynot`](../../backends/giobackend/cmd/giowhynot), behaves the same.

## Install

```bash
brew install arnodel/tap/whynot
```

or download a [release binary](https://github.com/arnodel/whynot/releases/latest), or

```bash
go install github.com/arnodel/whynot/cmd/whynot@latest
```

It also runs in a web browser: **[try it](https://arnodel.github.io/whynot/)**, and see
[`web`](web) for how that's built and what differs.

## Use

```bash
whynot path/to/document.md
whynot https://raw.githubusercontent.com/arnodel/whynot/refs/heads/main/README.md
```

With no argument it opens its welcome page, which explains all this too. To open another
document, paste its path or URL (**Cmd+V**, or **Ctrl+V**); paste "welcome" to come back.

| Action | How |
|---|---|
| Scroll | mouse wheel, touch, or drag the scrollbar |
| Scroll a step / a page | **↓** **↑** / **Space** **Shift+Space** |
| Scroll a wide code block or table sideways | **←** **→** (the one under the pointer, else the one last scrolled), or **Shift+wheel** |
| Follow a link | click it |
| Back / forward | **Backspace** / **Shift+Backspace**, or the toolbar |
| Zoom | **+** / **-**, or the toolbar |
| Table of contents | the toolbar; **Esc** closes it |
| Light / dark theme | the toolbar, or start with `-light` |
| Reload | the toolbar |
| Show FPS and frame timings | **F**, or start with `-debug-stats` |
| Outline what's under the pointer | start with `-debug-hit` |

## What it does with links and images

- A link to a Markdown document opens in the viewer. A link to a web page (one served as
  HTML) opens in your web browser. A `#heading` link scrolls to that heading.
- Links and images resolve against the current document's location, so relative ones
  work the same whether the document came from disk or the web.
- Images load in the background; one that can't load shows its alt text.
- Document text uses this platform's own fonts where it finds them (watch the log for
  what was picked), and the bundled Go fonts otherwise.

## How it's built

The viewer is a thin program around the library: a `whynot.Panel` for the document,
Ebitengine for the window and toolbar. Navigation, history, loading and theming live in
`internal/browser`, which `giowhynot` shares, so the two behave the same.
