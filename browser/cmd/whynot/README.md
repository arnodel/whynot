# whynot, the viewer

A Markdown document viewer built on the [whynot](../../../README.md) library and
[Ebitengine](https://ebitengine.org/): open a file or a URL, follow links between
documents, go back and forward, zoom, switch themes. Its Gio twin,
[`giowhynot`](../giowhynot), behaves the same.

## Install

```bash
brew install arnodel/tap/whynot
```

or download a [release binary](https://github.com/arnodel/whynot/releases/latest), or

```bash
go install github.com/arnodel/whynot/browser/cmd/whynot@latest
```

It also runs in a web browser: **[try it](https://arnodel.github.io/whynot/)**, and see
[`web`](web) for how that's built and what differs.

## Use

```bash
whynot path/to/document.md
whynot https://raw.githubusercontent.com/arnodel/whynot/refs/heads/main/README.md
```

With no argument it opens its welcome page, which explains all this too. To open another
document, paste its path or URL (**Cmd+V**); paste "welcome" to come back. On Windows and
Linux, **Ctrl** takes the place of **Cmd**.

| Action | How |
|---|---|
| Scroll | mouse wheel, touch, or drag the scrollbar |
| Scroll a step / a page | **↓** **↑** / **Space** **Shift+Space** |
| Scroll a wide code block or table sideways | **←** **→** (the one under the pointer, else the one last scrolled), or **Shift+wheel** |
| Follow a link | click it |
| Everything below, as links, with its keys | **Cmd+/** or **?**, or the toolbar's **⋮** |
| Back / forward | **Cmd+[** / **Cmd+]** or **Cmd+←** / **Cmd+→** (**Alt+←** / **Alt+→** on Windows and Linux), **Backspace** / **Shift+Backspace**, or the toolbar |
| Zoom | **Cmd+=** / **Cmd+-** / **Cmd+0**, or the actions page |
| Table of contents | **Cmd+T**, or the toolbar; **Esc** closes it |
| Journal mode: the pages visited, above the current one | **Cmd+J** |
| Light / dark theme | **Cmd+Shift+L**, the actions page, or start with `-light` |
| Only open local files beneath a directory | start with `-root dir` |
| Let documents from the web use the AI | start with `-remote-ai` |
| Reload | **Cmd+R**, or the toolbar |
| Show FPS and frame timings | **F**, or start with `-debug-stats` |
| Outline what's under the pointer | start with `-debug-hit` |

## Documents that run

A **whymark** document is Markdown whose front matter says so: passages, with Lua in them,
that make a page at a time as you follow links. The viewer shows its pages, and journal mode
shows how you got to one. A page can ask for text: a strip below it opens a field (Enter
sends). Try [the cellar](../../examples/cellar.md),
[the clockmaker's shop](../../examples/clockmaker.md),
[the notebook](../../examples/notebook.md) or [a chat with an AI](../../examples/chat.md); [whymark.md](../../whymark.md) is the format's
specification.

A document can also ask an AI, if it declares so, when `ANTHROPIC_API_KEY` is set: it asks
Claude, at your expense. Local documents can; ones from the web only if you start with
`-remote-ai`. Without the AI, a document shows what it wrote for that case instead.

## What it does with links and images

- A link to a Markdown document opens in the viewer. A link to a web page (one served as
  HTML, or a local `.html` file) opens in your web browser. Anything else that isn't text,
  such as an image or a PDF, isn't opened. A `#heading` link scrolls to that heading.
- Links and images resolve against the current document's location, so relative ones
  work the same whether the document came from disk or the web.
- Images load in the background; one that can't load shows its alt text.
- Local documents and images can come from anywhere on the document's drive. To be
  stricter, start with `-root dir`: then only files beneath `dir` are opened, even
  through `..` or a symbolic link.
- Document text uses this platform's own fonts where it finds them (watch the log for
  what was picked), and the bundled Go fonts otherwise.

## How it's built

The viewer is a thin program around the library: a `whynot.Panel` for the document,
Ebitengine for the window and toolbar. Navigation, history, loading and theming live in
`browser/internal/browser`, which `giowhynot` shares, so the two behave the same.
