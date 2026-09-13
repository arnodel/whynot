package whynot

import (
	"image"
	"image/color"
	"image/draw"
	"image/gif" // registers the GIF format with image.DecodeConfig/image.Decode too
	"io"
	"time"
)

// AnimatedImage holds every frame of a decoded GIF, already composited
// to full-canvas, disposal-correct frames (see decodeAnimatedGIF).
// CurrentFrame is a pure function of now (elapsed time since rendering
// started - RenderingContext.Time) and the animation's own total
// duration - there's no per-animation "start" to track: now is already
// "time since start", so a looping animation just always shows
// whichever frame now % total currently lands in, as if it had been
// playing since time zero. Deterministic and easy to test without any
// real waiting, and no clock-skew concerns between decode time and
// draw time to reason about, since nothing is stamped at decode time.
// Always loops forever.
type AnimatedImage struct {
	frames []image.Image
	delays []time.Duration
	total  time.Duration
}

// Bounds is every frame's shared size - GIF frames may individually be
// smaller (see decodeAnimatedGIF), but every composited frame here is
// full-canvas.
func (a *AnimatedImage) Bounds() image.Rectangle {
	return a.frames[0].Bounds()
}

// CurrentFrame returns whichever frame now lands in, looping forever.
func (a *AnimatedImage) CurrentFrame(now time.Duration) image.Image {
	if len(a.frames) <= 1 || a.total <= 0 {
		return a.frames[0]
	}
	if now < 0 {
		now = 0
	}
	elapsed := now % a.total
	for i, d := range a.delays {
		if elapsed < d {
			return a.frames[i]
		}
		elapsed -= d
	}
	// Rounding at the very end of the loop - land on the last frame
	// rather than panic on an out-of-range index.
	return a.frames[len(a.frames)-1]
}

// minGIFFrameDelay is what a per-frame delay under 2 hundredths of a
// second is treated as instead, matching the convention most
// browsers/viewers use. Some GIFs specify 0 for every frame, leaving
// the rate to "whatever the viewer defaults to" - cycling at that raw
// rate instead (10 frames at, say, 20ms each is a 200ms loop) produces
// frantic flicker rather than a leisurely animation.
const minGIFFrameDelay = 100 * time.Millisecond

// decodeAnimatedGIF decodes r as a GIF and composites every frame into
// a full-canvas image.Image, honoring each frame's Disposal method:
// many real-world GIFs only encode each frame's *changed region*, not
// the whole canvas (a common size optimization), so naively drawing
// each frame's raw data directly would ghost or show stale pixels for
// any GIF relying on that.
//
//   - DisposalNone (or unspecified): leave the canvas as this frame
//     left it for the next one.
//   - DisposalBackground: clear this frame's own rect to the
//     background color before the next frame is drawn.
//   - DisposalPrevious: restore the canvas to its state from just
//     before this frame was drawn, before the next one.
//
// A single-frame GIF decodes fine too (an AnimatedImage of one frame,
// CurrentFrame always returning it) - no separate "is this actually
// animated" branch needed upstream.
func decodeAnimatedGIF(r io.Reader) (*AnimatedImage, error) {
	g, err := gif.DecodeAll(r)
	if err != nil {
		return nil, err
	}

	bounds := image.Rect(0, 0, g.Config.Width, g.Config.Height)
	canvas := image.NewRGBA(bounds)
	backgroundOf := func() image.Image {
		if p, ok := g.Config.ColorModel.(color.Palette); ok && int(g.BackgroundIndex) < len(p) {
			return image.NewUniform(p[g.BackgroundIndex])
		}
		return image.Transparent
	}
	draw.Draw(canvas, bounds, backgroundOf(), image.Point{}, draw.Src)

	frames := make([]image.Image, len(g.Image))
	delays := make([]time.Duration, len(g.Image))
	var total time.Duration
	var beforeThisFrame *image.RGBA

	for i, frame := range g.Image {
		var disposal byte
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}
		if disposal == gif.DisposalPrevious {
			beforeThisFrame = image.NewRGBA(bounds)
			copy(beforeThisFrame.Pix, canvas.Pix)
		}

		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)

		composited := image.NewRGBA(bounds)
		copy(composited.Pix, canvas.Pix)
		frames[i] = composited

		delay := minGIFFrameDelay
		if i < len(g.Delay) && g.Delay[i] >= 2 {
			delay = time.Duration(g.Delay[i]) * 10 * time.Millisecond
		}
		delays[i] = delay
		total += delay

		switch disposal {
		case gif.DisposalBackground:
			draw.Draw(canvas, frame.Bounds(), backgroundOf(), frame.Bounds().Min, draw.Src)
		case gif.DisposalPrevious:
			copy(canvas.Pix, beforeThisFrame.Pix)
		}
	}

	return &AnimatedImage{frames: frames, delays: delays, total: total}, nil
}
