---
name: run-whynot
description: Build, run, and screenshot whynot (the Markdown-renderer-for-ebiten app in cmd/whynot). Use when asked to run whynot, launch cmd/whynot, take a screenshot of the rendered document, scroll to a section, or otherwise exercise the ebiten app end-to-end without a human at the keyboard.
---

whynot's only runnable binary is `cmd/whynot`, an ebiten.Game that renders a Markdown file and lets
a human scroll it with the mouse wheel. Drive it headlessly with
`.claude/skills/run-whynot/driver.go`, which launches `cmd/whynot` as an `exp/vmhost` guest (no
visible window, no changes to `cmd/whynot`'s source), optionally scrolls it by injecting wheel
input, and dumps the final frame as a PNG. All paths below are relative to the repo root.

## Prerequisites

None beyond what building the module already needs (Go, and whatever ebiten's desktop backend
needs on your OS - already satisfied if `go build ./...` works in this repo). Verified on macOS; no
extra system packages were required there. On Linux, the host window is hidden but still needs a
real display/GPU context (`exp/vmhost` is *windowless*, not *displayless*), so headless Linux CI
would need Xvfb or an EGL setup - untested in this session.

## Build

No separate build step - the driver builds the guest itself (see Run). Just confirm the module
builds first:

```bash
go build ./... && go vet ./...
```

## Run (agent path)

```bash
go run ./.claude/skills/run-whynot -out /tmp/frame.png
```

That renders `cmd/whynot/test.md` at the top of the document into `/tmp/frame.png`, 1024x768
logical pixels (physical size is that times your display's `DeviceScaleFactor`, e.g. 2048x1536 on a
2x/Retina display). Flags:

| flag | default | what it does |
|---|---|---|
| `-pkg` | `./cmd/whynot` | guest package to build and run |
| `-source` | `cmd/whynot/test.md` | markdown file rendered; resolved to an absolute path and passed to the guest as its file argument, so it works regardless of the guest's cwd |
| `-out` | `frame.png` | PNG output path |
| `-w`, `-h` | `1024`, `768` | logical window size |
| `-wheel-dy` | `-50` | wheel dy re-injected every tick of the scroll phase; negative scrolls **down** the document |
| `-scroll-ticks` | `0` | how many ticks to hold `-wheel-dy` before releasing it; `0` skips scrolling entirely |
| `-ticks` | `5` | settle ticks run after scrolling, immediately before the frame is captured |
| `-cursor-x`, `-cursor-y` | `-1`, `-1` | cursor position (device-independent pixels, same space as `-w`/`-h`) to move to before the settle ticks; negative (either one) skips moving the cursor |
| `-click` | `false` | press and release the left mouse button (at `-cursor-x`/`-cursor-y`) after scrolling, before the settle ticks - e.g. to follow a link under the cursor |
| `-key` | `""` | name of an `ebiten.Key` (e.g. `Backspace`) to press and release after scrolling, before the settle ticks - e.g. to trigger cmd/whynot's back action |
| `-modifier` | `""` | name of an `ebiten.Key` (e.g. `Meta`) to hold down for the duration of `-key`'s press+release - e.g. `-modifier Meta -key V` for Cmd+V |
| `-hold-ticks` | `1` | number of ticks to hold `-key` down before releasing it - `>1` to test key-repeat behavior (`inpututil.KeyPressDuration`), e.g. cmd/whynot's zoom +/- |
| `-debug-hit` | `false` | pass `-debug-hit` through to the guest, so the captured frame shows the red `HitTest` outline at the cursor - useful for confirming exact click coordinates before using `-click` |
| `-settle-delay` | `0` | real wall-clock `time.Sleep` before the settle ticks - see the Gotchas entry on background goroutines below before reaching for this |

To scroll to a specific section, increase `-scroll-ticks` (or `-wheel-dy`'s magnitude) and check the
result - there's no direct "scroll to heading" API, only wheel-notch simulation, so getting to a
specific spot is trial and error. As a data point verified in this repo: against `cmd/whynot/test.md`
at `-h 900`, `-wheel-dy -50 -scroll-ticks 24` lands with the "## Tables" section's first few tables
in frame; `-scroll-ticks 30` overshoots past the first two tables.

```bash
# Example: scroll ~24 wheel-ticks down, then capture
go run ./.claude/skills/run-whynot -h 900 -wheel-dy -50 -scroll-ticks 24 -out /tmp/frame.png
```

To screenshot a different markdown file (e.g. one you're editing to test a rendering change),
point `-source` at it - no need to overwrite `cmd/whynot/test.md`:

```bash
go run ./.claude/skills/run-whynot -source /tmp/my-test.md -out /tmp/frame.png
```

Then view `/tmp/frame.png` with your image-viewing tool.

## Run (human path)

```bash
cd cmd/whynot && go run .
```

Opens a real, visible window titled "Why Not?" showing `test.md`; scroll with the mouse wheel,
resize to see it reflow, Cmd/Alt-F4 or close the window to quit. Useless in a headless environment -
use the driver instead.

## Test

```bash
go test ./...
```

## Gotchas

- **whynot never fills a full-canvas background** - only glyphs, rules, and cell borders draw
  anything, so any pixel the document doesn't touch comes back from `ReadPixels` as transparent
  black rather than the opaque black a real window shows there (a real window ignores alpha and
  just displays raw RGB, which happens to be black for an untouched pixel). The driver's `dump()`
  forces alpha to 255 after reading pixels, without touching RGB, which - because `ReadPixels`
  returns premultiplied-alpha RGBA - exactly reproduces "composited over opaque black" including
  correctly darkened antialiased glyph edges. Skipping this step produces a PNG that different
  viewers render inconsistently (some flatten transparent-black to white, some to black), instead
  of consistently matching what the real windowed app shows. Hit while testing a short scratch
  document that left most of the frame untouched - a long document like `test.md` can visually hide
  this because it happens to fill most of the viewport with drawn content anyway.
- **Wheel input is a one-tick event, not a held state.** Unlike `PressKey`/`ReleaseKey`, a single
  `GuestSession.ScrollWheel` call only affects the very next tick that reads it - the guest resets
  its wheel delta to zero every tick (matching real wheel-event semantics, where `ebiten.Wheel()`
  reports only that tick's delta). To simulate holding the wheel down for `N` ticks, the driver
  re-injects `ScrollWheel` before *each* of the `N` ticks in a loop rather than injecting once before
  a single `AdvanceTicks(N)` - the naive version silently scrolls nothing.
- **The guest is version-locked to the host's ebiten build.** The driver imports
  `github.com/hajimehoshi/ebiten/v2/exp/vmhost` from this module's own `go.mod`
  (`v2.10.1` as of this writing), and builds the guest from source with `go build`, so they always
  match automatically. If `go.mod`'s ebiten version changes and this driver stops working, diff it
  against the freshly-vendored copy at
  `$(go env GOMODCACHE)/github.com/hajimehoshi/ebiten/v2@<new-version>/skills/run-ebitengine-app-headless/_driver/main.go`
  - `exp/vmhost`'s API has already moved twice: method signatures shifted between alpha versions,
    and going from the last alpha (`v2.10.0-alpha.12`) to the stable `v2.10.1` renamed the guest
    build tag from `ebitenginevm` to `ebitenginevmguest` and moved the vendored skill from
    `.agents/skills/run-ebitengine-app-headless/driver/main.go` to
    `skills/run-ebitengine-app-headless/_driver/main.go` - a stale tag silently builds a guest
    that tries to open a real window instead of connecting to the host, which just hangs until
    the host's own accept timeout fires.
- **Don't pass a relative `-source` and rely on the guest's cwd.** The driver resolves `-source` to
  an absolute path *before* passing it to the guest as an argument, specifically so the guest's
  actual working directory (some arbitrary temp dir, since the driver execs a freshly-built binary)
  never matters.
- **`-settle-delay` (a real `time.Sleep` before the settle ticks, added to check whynot's
  async image-loading fix) did not reliably let a guest-side background goroutine settle**, even at
  2+ real seconds with visibly continuous rendering activity throughout (confirmed via the
  `CAMetalLayer` spam below, at its normal ~16ms cadence for the whole sleep) - including a *local*
  file case that should fail via an instant `os.Open` syscall with no network dependency at all.
  `AdvanceTicks` itself doesn't pace to real time either (confirmed separately: 300 ticks executed in
  well under a second). Root cause not confirmed - plausibly something about how `exp/vmhost`'s guest
  process schedules independently-spawned goroutines between host-driven ticks, but that's a guess,
  not a diagnosis. Net effect: don't trust this driver to verify that an async background operation
  (a goroutine started independently of the guest's Update/Draw calls) eventually completes and
  repaints - it may never appear to, regardless of real elapsed time or tick count. Verify that kind
  of behavior with normal Go tests (real goroutines, no vmhost) instead, and treat this driver as
  good for input-driven, synchronous-per-tick behavior only.

## Troubleshooting

- **`[CAMetalLayer nextDrawable] returning nil because allocation failed` spamming stderr (macOS)**:
  harmless - the host window is hidden (`SetWindowVisible(false)`), so its Metal layer never
  actually presents a drawable. The run still completes and the frame is still captured correctly;
  ignore these lines.
