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
| Zoom | **Cmd+=** / **Cmd+-** / **Cmd+0** (on the web, with Shift too, leaving Cmd alone to the web browser), or the actions page |
| Table of contents | **Cmd+T**, or the toolbar; **Esc** closes it |
| Journal mode: the pages visited, above the current one | **Cmd+J** |
| Light / dark theme | **Cmd+Shift+L**, the actions page, or start with `-light` |
| Only open local files beneath a directory | start with `-root dir` |
| Let documents from the web use the AI | start with `-remote-ai` |
| Reload | **Cmd+R**, or the toolbar |
| Select text | drag (in journal mode, across pages too); double-click for a word, triple-click for a paragraph; Shift-click extends |
| Copy the selection as Markdown | **Cmd+C**, or the actions page |
| Select all of the page (in journal mode, again for the whole journal) | **Cmd+A**, or the actions page |
| Copy the page as Markdown (in journal mode, the whole journal) | **Cmd+Shift+C**, or the actions page |
| Show FPS and frame timings | **F**, or start with `-debug-stats` |
| Outline what's under the pointer | start with `-debug-hit` |

## Documents that run

A **whymark** document is Markdown whose front matter says so: passages, with Lua in them,
that make a page at a time as you follow links. The viewer shows its pages, and journal mode
shows how you got to one. A page can ask for text: a strip below it opens a field (Enter
sends). Try [the cellar](../../examples/cellar.md),
[the clockmaker's shop](../../examples/clockmaker.md),
[the notebook](../../examples/notebook.md), [a chat with an AI](../../examples/chat.md) or
[a to-do list you talk to](../../examples/todo.md);
[whymark.md](../../whymark.md) is the format's specification. The clockmaker, the chat and
the to-do list use modules from [examples/lib](../../examples/lib): dice, AI answers that
remember earlier pages, and dates.

whymark pages are kept on disk, in your user configuration directory (on macOS,
`~/Library/Application Support/whynot/whymark`), so a document that asks to be resumed,
like the to-do list, carries on where you left it the next time you open it; a link to it
from its own page, like "Start over", starts it afresh. Each page has a URL of its own,
ending in `@` and its page id (copy it from the actions page): opening it, in any whynot on
the machine, carries on from that page.

A document can also ask an AI, if it declares so, when whynot has an API key: it asks
Claude, at your expense. Local documents can; ones from the web only if you start with
`-remote-ai`. Without the AI, a document shows what it wrote for that case instead.

The key comes from `ANTHROPIC_API_KEY`, if it's set, or else from a config file, which the
app bundle reads too: `~/.config/whynot/config.yaml` (in `$XDG_CONFIG_HOME` if that's set),
or else `whynot/config.yaml` in the system's config directory, such as
`~/Library/Application Support` on macOS or `%AppData%` on Windows:

```yaml
anthropic-api-key: sk-ant-...
```

or, to keep the key in a password store, a command that prints it:

```yaml
anthropic-api-key-command: security find-generic-password -s whynot-claude-api-key -w
```

The file is only trusted with either if no one else can read or write it: `chmod 600` it.
The log says where the key came from, or why it wasn't used, and names a config file that's
ignored because another comes first.

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
