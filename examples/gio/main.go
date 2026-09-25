// Command gio is a runnable example of whynot rendering through
// giorenderer (Gio, gioui.org) instead of ebitenrenderer - a
// giorenderer.Panel showing one document, with scroll, link hover/
// click, and a scrollbar. See examples/panel for the ebiten-backed
// equivalent this mirrors.
package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"strings"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/widget/material"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/giorenderer"
)

func exampleDoc() string {
	var b strings.Builder
	b.WriteString("# Gio example\n\n")
	b.WriteString("This document exists to check giorenderer's `Canvas` against the same\n")
	b.WriteString("three primitives ebitenrenderer implements: text (this paragraph, mixed\n")
	b.WriteString("with `inline code` and **bold**), a filled rect (the thematic break\n")
	b.WriteString("below), and an image. It's also long enough to need scrolling, to check\n")
	b.WriteString("giorenderer.Panel's scroll/hover/click/scrollbar handling.\n\n")
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
	view := whynot.NewView([]byte(exampleDoc()), whynot.NewGoFontFaceSelector(72), whynot.WithStyleSheet(whynot.NewDarkStyleSheet()))

	// material.NewTheme()'s default Palette.Fg (the scrollbar indicator's
	// color) is black - invisible against this document's dark
	// background, so brighten it here rather than in giorenderer itself.
	theme := material.NewTheme()
	theme.Palette.Fg = color.NRGBA{R: 0xe0, G: 0xe0, B: 0xe0, A: 0xff}

	panel := giorenderer.NewPanel(view, giorenderer.New(), image.Rectangle{}, giorenderer.WithScrollbar(), giorenderer.WithTheme(theme))
	panel.OnLinkClick = func(dest string) { log.Printf("clicked: %s", dest) }
	panel.OnLinkHover = func(dest string) {
		if dest != "" {
			log.Printf("hovering: %s", dest)
		}
	}

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

			panel.Update(gtx)
			panel.Draw(gtx)
			e.Frame(gtx.Ops)
		}
	}
}
