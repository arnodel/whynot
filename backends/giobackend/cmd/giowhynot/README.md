# giowhynot, the Gio viewer

The [whynot viewer](../../../../cmd/whynot), built on [Gio](https://gioui.org/) instead of
Ebitengine: the same document browser (links, history, zoom, themes, table of contents)
with a Gio-native toolbar and Gio's own scrollbar. It's a full program built on whynot's
[Gio backend](../..).

## Install

```bash
go install github.com/arnodel/whynot/backends/giobackend/cmd/giowhynot@latest
```

Gio needs a few system libraries on Linux: see
[Gio's installation guide](https://gioui.org/doc/install/linux). There's no release binary
yet.

It also runs in a web browser: **[try it](https://arnodel.github.io/whynot/giowhynot/)**,
and see [`web`](web) for how that's built and what differs.

## Use

The same as [`whynot`](../../../../cmd/whynot#use): the same arguments, keys, and
`-light` and `-root` options. The differences:

- **The address bar is editable:** click it, type a path, a URL or "welcome", and press
  **Enter** to go there.
- There's no **F** key or `-debug-stats`/`-debug-hit` option.
