# Backends

A backend adapts whynot to a graphics framework. whynot itself doesn't depend on any
framework: it draws through a small interface, and it receives input as plain values.
A backend provides those two things on top of its framework, and the program using it
drives the frame loop.

whynot comes with two backends:

| Backend | Framework | Notes |
|---|---|---|
| [`ebitenbackend`](ebitenbackend) | [Ebitengine](https://ebitengine.org/) | Reads input by polling it each tick |
| [`giobackend`](giobackend) | [Gio](https://gioui.org/) | Reads input from Gio's event stream; a separate Go module, on version 0 while Gio is |

Each one's package documentation shows a complete frame loop. The rest of this page is
for writing a backend for another framework. The two above are the best reference: they
take quite different approaches, and both are short.

## What a backend provides

### 1. Drawing: a `canvas.Canvas`

A View draws a document by calling the five methods of
[`canvas.Canvas`](https://pkg.go.dev/github.com/arnodel/whynot/canvas): it fills
rectangles, draws text and draws images, within a clipping rectangle. The
[package documentation](https://pkg.go.dev/github.com/arnodel/whynot/canvas) is the
specification to implement; these are the points that need the most care:

- **Text must be placed exactly as `font.Drawer` places it.** The View measures text with
  `golang.org/x/image/font`, so `DrawText` must advance each glyph by its advance width
  plus kerning, in the face's fixed-point units, without rounding between glyphs.
  Otherwise the drawn text drifts from where the layout expects it.
- **Images must be scaled, preferably smoothly.** `DrawImage` gives the size to draw at,
  which is usually not the image's own size.
- **`Clip` keeps the coordinates.** The Canvas it returns draws in the same coordinate
  space, and only shows what falls within the clipping rectangle.
- **The same values come back, so they can be cached.** The View passes the same
  `font.Face` for the same text style at the same scale, and the same `image.Image` for
  the same loaded image, so a backend can use them as keys, for example to keep an
  uploaded texture or a rasterized glyph.

Coordinates are integer pixels: the framework's device pixels, for both of whynot's
backends.

### 2. Input: `input.Event` values

The Controller in a View or Panel acts on the user's input in the form of
[`input`](https://pkg.go.dev/github.com/arnodel/whynot/input) events:

- `PointerMove`, `PointerLeave` and `PointerButton`, for a mouse;
- `Wheel`, for a mouse wheel or a trackpad;
- `TouchStart`, `TouchMove`, `TouchEnd` and `TouchCancel`, for touch screens.

A backend's input reader implements `input.Source`: each call to its `Events` method
returns what the user did since the previous call, in order. There are two ways to write
one, depending on the framework:

- If the framework lets you poll the input state, as Ebitengine does, compare it with
  the previous frame's and report the differences: the pointer moved, a button went
  down, and so on.
- If the framework delivers events, as Gio does, translate each one.

A few rules apply either way:

- Positions are in the same coordinate space as the Canvas. If the canvas covers a whole
  window, they're window coordinates, in the same pixels.
- A wheel's deltas are an amount to scroll, not a position: they're in logical pixels,
  so convert the framework's wheel units (or device pixels, divided by the screen's
  density) to those. The Controller scales them to the View's pixels itself. A
  framework's wheel units often differ between platforms: `ebitenbackend`'s
  `wheel_*.go` files show one way to handle that.
- Report all the input you see. The Controller ignores what falls outside its View's
  bounds, except that a press inside them captures the pointer until it's released.
- Keyboard input isn't part of it: the program binds keys to commands such as
  `Panel.PageDown`.

### 3. The frame loop

The program, not the backend, drives whynot, once per frame:

```go
// Each frame, with now the time on any steady clock:
events := panel.Frame(source.Events(), now) // apply the input; get back what happened
for _, e := range events {
	switch e.(type) {
	case whynot.LinkClick:
		// follow the link, for example
	}
}
panel.Draw(canvas, now) // draw onto the backend's Canvas for this frame
```

- **The scale:** `Panel.SetScale` takes the number of device pixels per logical pixel,
  so that text and margins are sized for the screen's density.
- **Frames on demand:** if the framework only draws a frame when something happens, as
  Gio does, it must ask for another frame while `panel.Animating()` is true. Otherwise a
  fling stops dead, and a fading scrollbar stays half visible.
- **The time:** whynot never reads the clock itself. `now` can be measured from any
  starting point, as long as it's the same clock every frame.

## Where to put a new backend

A backend doesn't have to be part of whynot: it can live in its own repository, because
the `canvas` and `input` packages it needs are public.
