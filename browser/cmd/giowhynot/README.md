# giowhynot, the Gio viewer

The [whynot viewer](../whynot), built on [Gio](https://gioui.org/) instead of
Ebitengine: the same document browser (links, history, zoom, themes, table of contents)
with a Gio-native toolbar and Gio's own scrollbar. It's a full program built on whynot's
[Gio backend](../../../backends/giobackend).

## Install

```bash
go install github.com/arnodel/whynot/browser/cmd/giowhynot@latest
```

Gio needs a few system libraries on Linux: see
[Gio's installation guide](https://gioui.org/doc/install/linux). There's no release binary
yet.

On macOS, from a checkout, `make mac-app-gio` in [`browser`](../..) makes `build/giowhynot.app`,
an app bundle, in which macOS's dictation works in the prompt; `make run-mac-app-gio
ARGS=examples/todo.md` builds it and runs it from the terminal, with its log there.

It also runs in a web browser: **[try it](https://arnodel.github.io/whynot/giowhynot/)**,
and see [`web`](web) for how that's built and what differs.

## Use

The same as [`whynot`](../whynot#use): the same arguments, keys, and
`-light`, `-root` and `-remote-ai` options. The differences:

- **The address bar is editable:** click it, type a path, a URL or "welcome", and press
  **Enter** to go there.
- There's no **F** key or `-debug-stats`/`-debug-hit` option.
