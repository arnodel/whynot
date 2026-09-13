package whynot

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"os"
	"testing"
	"time"
)

// pixelAt reads img's color at (x, y) as color.RGBA, for exact
// comparison against one of the test palette's own colors.
func pixelAt(t *testing.T, img image.Image, x, y int) color.RGBA {
	t.Helper()
	r, g, b, a := img.At(x, y).RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

// TestDecodeAnimatedGIFCompositesDisposal builds a small synthetic
// multi-frame GIF exercising all three disposal methods on a 2x2
// canvas, and checks each composited frame's exact pixels - this is
// the one genuinely fiddly piece of new logic (many real-world GIFs
// only encode each frame's *changed region*, not the whole canvas, so
// getting disposal wrong shows up as ghosting or stale pixels), worth
// pinning down precisely rather than smoke-tested.
//
// Palette: 0=white(background), 1=red, 2=blue, 3=green.
//
//   - Frame 0 (DisposalNone): full 2x2 canvas, red. -> [[R,R],[R,R]]
//   - Frame 1 (DisposalBackground): 1x1 at (0,0), blue, drawn over the
//     still-red canvas (DisposalNone left frame 0 as-is) -> [[B,R],[R,R]].
//     Disposal then clears (0,0)-(1,1) to background before frame 2.
//   - Frame 2 (DisposalNone): 1x1 at (1,1), green, drawn on
//     [[white,R],[R,R]] (frame 1's rect now cleared) -> [[W,R],[R,G]].
//   - Frame 3 (DisposalPrevious): 1x1 at (0,1), blue, drawn on frame 2's
//     canvas -> [[W,R],[B,G]]. Disposal then reverts the canvas to its
//     state from just before frame 3 (i.e. back to [[W,R],[R,G]]).
//   - Frame 4 (DisposalNone): 1x1 at (0,0), green, drawn on the
//     *reverted* canvas -> [[G,R],[R,G]] if DisposalPrevious worked;
//     [[G,R],[B,G]] (blue leaking through) if it didn't.
func TestDecodeAnimatedGIFCompositesDisposal(t *testing.T) {
	palette := color.Palette{
		color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}, // 0: white (background)
		color.RGBA{0xFF, 0x00, 0x00, 0xFF}, // 1: red
		color.RGBA{0x00, 0x00, 0xFF, 0xFF}, // 2: blue
		color.RGBA{0x00, 0xFF, 0x00, 0xFF}, // 3: green
	}
	white, red, blue, green := palette[0].(color.RGBA), palette[1].(color.RGBA), palette[2].(color.RGBA), palette[3].(color.RGBA)

	solid := func(rect image.Rectangle, colorIndex uint8) *image.Paletted {
		p := image.NewPaletted(rect, palette)
		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			for x := rect.Min.X; x < rect.Max.X; x++ {
				p.SetColorIndex(x, y, colorIndex)
			}
		}
		return p
	}

	g := &gif.GIF{
		Image: []*image.Paletted{
			solid(image.Rect(0, 0, 2, 2), 1), // frame 0: full canvas, red
			solid(image.Rect(0, 0, 1, 1), 2), // frame 1: (0,0), blue
			solid(image.Rect(1, 1, 2, 2), 3), // frame 2: (1,1), green
			solid(image.Rect(0, 1, 1, 2), 2), // frame 3: (0,1), blue
			solid(image.Rect(0, 0, 1, 1), 3), // frame 4: (0,0), green
		},
		Delay:           []int{10, 10, 10, 10, 10},
		Disposal:        []byte{gif.DisposalNone, gif.DisposalBackground, gif.DisposalNone, gif.DisposalPrevious, gif.DisposalNone},
		Config:          image.Config{ColorModel: palette, Width: 2, Height: 2},
		BackgroundIndex: 0,
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatalf("EncodeAll: %v", err)
	}

	anim, err := decodeAnimatedGIF(&buf)
	if err != nil {
		t.Fatalf("decodeAnimatedGIF: %v", err)
	}
	if len(anim.frames) != 5 {
		t.Fatalf("got %d frames, want 5", len(anim.frames))
	}

	want := [][4]color.RGBA{
		{red, red, red, red},      // frame 0: [[R,R],[R,R]] as (0,0),(1,0),(0,1),(1,1)
		{blue, red, red, red},     // frame 1
		{white, red, red, green},  // frame 2
		{white, red, blue, green}, // frame 3
		{green, red, red, green},  // frame 4 - proves DisposalPrevious reverted, not leaked
	}
	for i, frame := range anim.frames {
		got := [4]color.RGBA{
			pixelAt(t, frame, 0, 0), pixelAt(t, frame, 1, 0),
			pixelAt(t, frame, 0, 1), pixelAt(t, frame, 1, 1),
		}
		if got != want[i] {
			t.Errorf("frame %d pixels = %v, want %v", i, got, want[i])
		}
	}
}

// TestDecodeAnimatedGIFSingleFrame checks a non-animated (one-frame)
// GIF decodes fine too - no separate "is this actually animated"
// branch needed upstream.
func TestDecodeAnimatedGIFSingleFrame(t *testing.T) {
	palette := color.Palette{color.RGBA{0xFF, 0, 0, 0xFF}}
	frame := image.NewPaletted(image.Rect(0, 0, 3, 3), palette)
	g := &gif.GIF{
		Image:  []*image.Paletted{frame},
		Delay:  []int{0},
		Config: image.Config{ColorModel: palette, Width: 3, Height: 3},
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}
	anim, err := decodeAnimatedGIF(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(anim.frames) != 1 {
		t.Fatalf("got %d frames, want 1", len(anim.frames))
	}
	if anim.CurrentFrame(0) != anim.CurrentFrame(999*time.Hour) {
		t.Error("a single-frame animation should always return the same frame")
	}
}

// TestDecodeAnimatedGIFZeroDelayUsesMinimum checks that a GIF
// specifying delay 0 for every frame (not uncommon - some encoders
// leave this to "whatever the viewer defaults to") still gets a
// sensible frame rate rather than cycling frantically fast.
func TestDecodeAnimatedGIFZeroDelayUsesMinimum(t *testing.T) {
	palette := color.Palette{color.Black, color.White}
	frame := func(c uint8) *image.Paletted {
		p := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
		for i := range p.Pix {
			p.Pix[i] = c
		}
		return p
	}
	g := &gif.GIF{
		Image:  []*image.Paletted{frame(0), frame(1), frame(0), frame(1)},
		Delay:  []int{0, 0, 0, 0},
		Config: image.Config{ColorModel: palette, Width: 2, Height: 2},
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}
	anim, err := decodeAnimatedGIF(&buf)
	if err != nil {
		t.Fatal(err)
	}
	for i, d := range anim.delays {
		if d < 100*time.Millisecond {
			t.Errorf("delays[%d] = %v, want at least 100ms even though the GIF specified 0", i, d)
		}
	}
}

// TestDecodeAnimatedGIFRealFile decodes a real-world animated GIF
// (not a small synthetic one) - the actual bytes real users will have,
// likely with multiple frames, disposal methods, and a local rather
// than global palette, none of which the synthetic compositing test
// above exercises directly.
func TestDecodeAnimatedGIFRealFile(t *testing.T) {
	f, err := os.Open("testdata/homer_animated_gif.gif")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	anim, err := decodeAnimatedGIF(f)
	if err != nil {
		t.Fatalf("decodeAnimatedGIF: %v", err)
	}
	if len(anim.frames) < 2 {
		t.Fatalf("got %d frames, want at least 2 (this fixture is meant to be animated)", len(anim.frames))
	}
	want := image.Rect(0, 0, 320, 320)
	if anim.Bounds() != want {
		t.Errorf("Bounds() = %v, want %v", anim.Bounds(), want)
	}
	if anim.total <= 0 {
		t.Error("total duration = 0, want positive (delays should have been captured)")
	}
	// Every composited frame should be full-canvas, whatever its
	// original (possibly partial) bounds were - the whole point of
	// compositing.
	for i, frame := range anim.frames {
		if frame.Bounds() != want {
			t.Errorf("frame %d bounds = %v, want %v (full canvas)", i, frame.Bounds(), want)
		}
	}
}

// TestAnimatedImageCurrentFrame checks frame selection as a pure
// function of now - no real clock involved.
func TestAnimatedImageCurrentFrame(t *testing.T) {
	f0 := image.NewUniform(color.Black)
	f1 := image.NewUniform(color.White)
	f2 := image.NewUniform(color.Gray{0x80})
	anim := &AnimatedImage{
		frames: []image.Image{f0, f1, f2},
		delays: []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond},
		total:  60 * time.Millisecond,
	}

	cases := []struct {
		now  time.Duration
		want image.Image
	}{
		{0, f0},
		{5 * time.Millisecond, f0},
		{10 * time.Millisecond, f1}, // exactly at the boundary: next frame
		{25 * time.Millisecond, f1},
		{30 * time.Millisecond, f2},
		{59 * time.Millisecond, f2},
		{60 * time.Millisecond, f0}, // wraps around (looping)
		{65 * time.Millisecond, f0},
		{125 * time.Millisecond, f0}, // 125 % 60 = 5, still within frame 0's window
		{-1, f0},                     // negative now clamps to 0
	}
	for _, tc := range cases {
		got := anim.CurrentFrame(tc.now)
		if got != tc.want {
			t.Errorf("CurrentFrame(%v) = %v, want %v", tc.now, got, tc.want)
		}
	}
}
