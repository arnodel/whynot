# whynot in WebAssembly

A minimal proof of concept: `whynot` rendering `demo.md` (a copy of `../../testdata/demo.md`
- see `main.go`'s doc comment for why it's a copy, not an import), scrollable and reflowing
on resize, running entirely in the browser via Ebitengine's own `js`/`wasm` backend. See
`main.go`'s doc comment for exactly what this does and doesn't do yet.

## Build

```bash
GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o whynot.wasm .
```

`wasm_exec.js` (Go's runtime glue for `js`/`wasm`, committed alongside `whynot.wasm` in
this directory) is coupled to the exact Go toolchain version it came from. If you rebuild
with a different Go version than this repo's `go.mod` currently pins, copy a fresh one
too, or `go run .` may fail confusingly at load time rather than at build time:

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

Then visit <http://localhost:8000>.
