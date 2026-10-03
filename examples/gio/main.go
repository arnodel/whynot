// Command gio is a runnable example of whynot rendering through
// giorenderer (Gio, gioui.org) instead of ebitenrenderer - a
// whynot.Panel showing one document, with scroll, link hover/click, and
// a scrollbar. See examples/panel for the ebiten-backed equivalent this
// mirrors.
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
	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/giorenderer"
	"github.com/arnodel/whynot/styles/simpletheme"
)

func exampleDoc() string {
	var b strings.Builder
	b.WriteString("# Gio example\n\n")
	b.WriteString("This document exists to check giorenderer's `Canvas` against the same\n")
	b.WriteString("three primitives ebitenrenderer implements: text (this paragraph, mixed\n")
	b.WriteString("with `inline code` and **bold**), a filled rect (the thematic break\n")
	b.WriteString("below), and an image. It's also long enough to need scrolling, to check\n")
	b.WriteString("whynot.Panel's scroll/hover/click/scrollbar handling.\n\n")
	b.WriteString("Try the mouse wheel, dragging the scrollbar on the right, and hovering/\n")
	b.WriteString("clicking [this link](https://example.com).\n\n")
	b.WriteString("---\n\n")
	for i := 1; i <= 12; i++ {
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
	view := whynot.NewView(whynot.Parse([]byte(exampleDoc())), fonts.NewGoSelector(), simpletheme.DarkStyleSheet)
	panel := whynot.NewPanel(view, image.Rectangle{})
	panel.SetScrollbar(true)
	panel.OnLinkClick = func(dest string) { log.Printf("clicked: %s", dest) }
	panel.OnLinkHover = func(dest string) {
		if dest != "" {
			log.Printf("hovering: %s", dest)
		}
	}

	renderer := giorenderer.New()
	var in giorenderer.Input
	start := time.Now()

	w := new(app.Window)
	var ops op.Ops
	for {
		e := w.Event()
		switch e := e.(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			bounds := image.Rectangle{Max: e.Size}
			if panel.Bounds() != bounds {
				panel.SetBounds(bounds)
			}
			if scale := float64(e.Metric.PxPerDp); panel.Scale() != scale {
				panel.SetScale(scale)
			}

			now := time.Since(start)
			panel.Frame(in.Source(gtx, panel, bounds).Events(), now)
			// Gio only produces frames when something happens.
			if panel.Animating() {
				gtx.Execute(op.InvalidateCmd{})
			}
			panel.Draw(renderer.NewCanvas(gtx.Ops, bounds), now)
			e.Frame(gtx.Ops)
		}
	}
}
