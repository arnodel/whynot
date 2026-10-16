//go:build !js

package browser

import (
	"image"
	"io"
	"testing"
	"time"
)

// imageRecorder is a recorder that also records where images are drawn.
type imageRecorder struct {
	recorder
	images []image.Rectangle
}

func (c *imageRecorder) DrawImage(img image.Image, x, y, width, height int) {
	c.images = append(c.images, image.Rect(x, y, x+width, y+height))
}

// TestWaitingDots checks that the waiting dots show under a page that's
// still arriving once it hasn't grown for waitAfter, and not before, nor
// once it's complete.
func TestWaitingDots(t *testing.T) {
	dir := t.TempDir()
	start := writeTempMD(t, dir, "start.md", "# Start\n")
	app := newTestApp(t, dir, start, "# Start\n")
	doc, w := app.parser.Stream()
	io.WriteString(w, "# Thinking\n\nA first line.\n")
	app.watch(doc)
	view := app.NewView(doc)
	app.place(view)
	app.Panel.SetView(view)

	draw := func() *imageRecorder {
		c := &imageRecorder{recorder: recorder{Area: image.Rect(0, 0, testWidth, testHeight)}}
		app.Draw(c, 0)
		return c
	}
	if c := draw(); len(c.images) != 0 {
		t.Errorf("a page that just grew shows %d dots, want none", len(c.images))
	}
	if !app.Animating() {
		t.Error("a page still arriving isn't animating: Gio wouldn't draw the dots")
	}

	app.arrivals.mu.Lock()
	app.arrivals.last[doc] = time.Now().Add(-2 * waitAfter)
	app.arrivals.mu.Unlock()
	c := draw()
	if len(c.images) != 3 {
		t.Fatalf("a page quiet for longer than waitAfter shows %d dots, want 3", len(c.images))
	}
	lastText := 0
	for _, txt := range c.Texts {
		lastText = max(lastText, txt.Y)
	}
	for _, d := range c.images {
		if d.Min.Y <= lastText {
			t.Errorf("a dot at %v isn't below the text, whose last baseline is at %d", d, lastText)
		}
	}

	w.Close()
	if c := draw(); len(c.images) != 0 {
		t.Errorf("a complete page shows %d dots, want none", len(c.images))
	}
}
