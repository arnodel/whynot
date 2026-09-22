// Command gio is a runnable example of whynot rendering through
// giorenderer (Gio, gioui.org) instead of ebitenrenderer - v1, static:
// one document, redrawn every frame, no scroll/click wiring yet. See
// examples/panel for the ebiten-backed equivalent this mirrors.
package main

import (
	"fmt"
	"image"
	"log"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/op"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/giorenderer"
)

func exampleDoc() string {
	var b strings.Builder
	b.WriteString("# Gio example\n\n")
	b.WriteString("This document exists to check giorenderer's `Canvas` against the same\n")
	b.WriteString("three primitives ebitenrenderer implements: text (this paragraph, mixed\n")
	b.WriteString("with `inline code` and **bold**), a filled rect (the thematic break\n")
	b.WriteString("below), and an image.\n\n")
	b.WriteString("---\n\n")
	for i := 1; i <= 3; i++ {
		fmt.Fprintf(&b, "## Section %d\n\n", i)
		b.WriteString("Some plain text, and some `monospace code` right next to it, to check\n")
		b.WriteString("baseline alignment when faces mix on one line.\n\n")
	}
	return b.String()
}

func main() {
	go func() {
		if err := run(); err != nil {
			log.Fatal(err)
		}
	}()
	app.Main()
}

func run() error {
	view := whynot.NewView([]byte(exampleDoc()), whynot.NewGoFontFaceSelector(72), whynot.WithStyleSheet(whynot.NewDarkStyleSheet()))
	renderer := giorenderer.New()
	start := time.Now()

	w := new(app.Window)
	var ops op.Ops
	for {
		e := w.Event()
		switch e := e.(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			scale := float64(e.Metric.PxPerDp)
			view.Layout(e.Size.X, e.Size.Y, scale, time.Since(start))

			ops.Reset()
			canvas := renderer.NewCanvas(&ops, image.Rectangle{Max: e.Size})
			view.Draw(canvas, 0, 0)
			e.Frame(&ops)
		}
	}
}
