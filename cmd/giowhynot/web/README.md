# giowhynot in the browser

Runs the real `cmd/giowhynot` - toolbar (including the editable address bar), history, zoom,
theme - in a browser tab via Gio's own `js`/`wasm` backend. Local file opening and paste-to-open
both no-op with a console message instead of working, since neither maps onto a browser sandbox
the way it does on desktop - everything else (opening a Markdown document by `http(s)` URL,
following links between documents, scrolling/resizing/zooming, editing the address bar) works the
same as the desktop app.

## Build

From this directory:

```bash
GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o giowhynot.wasm ..
```

`wasm_exec.js` isn't committed in this directory - it's Go's own wasm runtime glue, not specific to
Gio or Ebitengine, so `cmd/whynot/web/wasm_exec.js` is the canonical copy (see that directory's own
README for how to refresh it against a new Go toolchain version). Copy it here too before serving
this directory standalone:

```bash
cp ../../whynot/web/wasm_exec.js .
```

(The live site's own build, via `.github/workflows/pages.yaml`, does this copy itself - only local
testing of this directory on its own needs the step above.)

## Run

Serve this directory over HTTP (opening `index.html` directly as a `file://` URL won't work -
`fetch` can't load `giowhynot.wasm` that way) and open it in a browser:

```bash
python3 -m http.server
```

Then visit <http://localhost:8000> for the built-in welcome page, or
<http://localhost:8000/?doc=https://raw.githubusercontent.com/arnodel/whynot/main/README.md> to
open a document by URL straight away - `?doc=` becomes `cmd/giowhynot`'s usual first command-line
argument (see `index.html`), the same convention `cmd/whynot/web` uses.
