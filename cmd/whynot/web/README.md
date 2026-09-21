# whynot in the browser

Runs the real `cmd/whynot` - toolbar, history, zoom, theme, syntax highlighting - in a
browser tab via Ebitengine's own `js`/`wasm` backend, not just a demo (see
[`examples/wasm`](../../examples/wasm) for that). Local file opening, paste-to-open, and
"open a real webpage in the system browser" all no-op with a console message instead of
working, since none of those map onto a browser sandbox the way they do on desktop -
everything else (opening a Markdown document by `http(s)` URL, following links between
documents, scrolling/resizing/zooming) works the same as the desktop app.

## Build

From this directory:

```bash
GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o whynot.wasm ..
```

`wasm_exec.js` (Go's runtime glue for `js`/`wasm`, committed alongside `whynot.wasm` in
this directory) is coupled to the exact Go toolchain version it came from. If you rebuild
with a different Go version than this repo's `go.mod` currently pins, copy a fresh one
too, or the page may fail confusingly at load time rather than at build time:

```bash
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" .
```

(That path moved in Go 1.24 - older toolchains have it at
`$(go env GOROOT)/misc/wasm/wasm_exec.js` instead.)

## Run

Serve this directory over HTTP (opening `index.html` directly as a `file://` URL won't
work - `fetch` can't load `whynot.wasm` that way) and open it in a browser:

```bash
python3 -m http.server
```

Then visit <http://localhost:8000> for the built-in welcome page, or
<http://localhost:8000/?doc=https://raw.githubusercontent.com/arnodel/whynot/main/README.md>
to open a document by URL straight away - `?doc=` becomes `cmd/whynot`'s usual first
command-line argument (see `index.html`), so it takes the same "welcome" or `http(s)` URL
values a desktop invocation's argument would (minus local paths, which don't mean
anything here).
