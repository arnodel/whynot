# Welcome to Why Not? {{VERSION}}

**whynot** is a Markdown file renderer for [ebiten](https://ebitengine.org/) games. This app,
`cmd/whynot`, is a small standalone viewer built on top of the library - point it at a file or a
URL and it lays out and draws the document on screen. Only what's actually on screen is ever laid
out, so even a very large document scrolls and resizes just as smoothly as a short one.

## Open a document

Copy a file path or an `http(s)` URL to your clipboard, then paste it with **{{PASTE_SHORTCUT}}**.

## Getting around

- **Mouse wheel** scrolls; resize the window any time to see the document reflow.
- **Space** / **Shift+Space** page down / up.
- **+** / **-**, or the toolbar buttons, zoom in / out.
- **L** / **D**, or the toolbar button, switch to the light / dark theme.
- Click a link to follow it: a link to a webpage opens in your browser, while one to a Markdown
  document opens right here - like
  [whynot's own README](https://raw.githubusercontent.com/arnodel/whynot/main/README.md).
  **Backspace** (or the back button) goes back, **Shift+Backspace** (or the forward button)
  goes forward.
- The reload button re-fetches the current document.

Paste "welcome" at any time to come back to this page.

## What the library renders

- Headings, paragraphs, emphasis, strong emphasis, and strikethrough
- Inline code, and fenced or indented code blocks
- Ordered, unordered, and nested lists, including task lists
- Tables, with column alignment
- Images - PNG, JPEG, and GIF, including animated GIFs - loaded in the background so they never
  block scrolling
- Blockquotes, including nested ones
- Links, autolinks, and thematic breaks

A quick look at a few of those:

| Left | Center | Right |
|:-----|:------:|------:|
| *italic* | **bold** | ~~struck out~~ |
| `inline code` | a [link](https://github.com/arnodel/whynot) | plain text |

- A list item
  - that can nest
  - as deep as it needs to
- another top-level item

- [x] Renders animated GIFs without blocking the display
- [ ] Renders APNG (not yet)

> A blockquote, for good measure.

---

See [the project on GitHub](https://github.com/arnodel/whynot) for more.
