package main

import (
	"image"
	"image/color"
	"unicode/utf8"

	"gioui.org/font/gofont"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/arnodel/whynot/backends/giobackend"
	"github.com/arnodel/whynot/browser/internal/browser"
)

// dock is the text field docked below the document while the current page
// has an input block (see browser.App.Input): Enter sends, Shift-Enter
// starts a new line, and Escape leaves the field for the page. It grows
// with its text, up to a third of the window.
type dock struct {
	editor   widget.Editor
	theme    *material.Theme
	renderer *giobackend.Renderer

	shown       bool   // the page has an input block
	placeholder string // its placeholder
	height      int    // as laid out last frame, in pixels
	blur        bool   // leave the field next frame
	version     int    // of the App's draft the field shows
}

func newDock(renderer *giobackend.Renderer) *dock {
	d := &dock{theme: material.NewTheme(), renderer: renderer}
	// As for the address bar: Gio's default shaper has no fonts in the
	// browser.
	d.theme.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	d.editor.Submit = true
	return d
}

// focused reports whether the field has the keyboard.
func (d *dock) focused(gtx layout.Context) bool {
	return d.shown && gtx.Focused(&d.editor)
}

// update shows the field for the current page's input block, focused when
// it appears, and sends its text on Enter.
func (d *dock) update(gtx layout.Context, app *browser.App) {
	placeholder, ok := app.Input()
	switch {
	case ok && !d.shown:
		gtx.Execute(key.FocusCmd{Tag: &d.editor})
		d.shown, d.placeholder = true, placeholder
	case ok:
		d.placeholder = placeholder
	// A page usually has its input block at its end: until the page is
	// complete, the field stays.
	case !app.Loading():
		d.shown = false
	}
	if !d.shown {
		d.height = 0
		return
	}
	if d.blur {
		d.blur = false
		gtx.Execute(key.FocusCmd{})
	}
	// The page's draft, after navigating: empty for a new page, or what
	// was typed or sent when coming back to one. Selected, so typing
	// replaces it.
	if text, version := app.InputDraft(); version != d.version {
		d.version = version
		d.editor.SetText(text)
		d.editor.SetCaret(utf8.RuneCountInString(text), 0)
	}
	for {
		e, ok := d.editor.Update(gtx)
		if !ok {
			break
		}
		switch e := e.(type) {
		case widget.ChangeEvent:
			app.SetInputDraft(d.editor.Text())
		case widget.SubmitEvent:
			if e.Text != "" {
				app.SubmitInput(e.Text)
			}
		}
	}
	for {
		e, ok := gtx.Event(key.Filter{Focus: &d.editor, Name: key.NameEscape})
		if !ok {
			break
		}
		if ke, ok := e.(key.Event); ok && ke.State == key.Press {
			gtx.Execute(key.FocusCmd{})
		}
	}
}

// layout draws the field across the bottom of the window, and records its
// height for the next frame's layout of the document.
func (d *dock) layout(gtx layout.Context) {
	if !d.shown {
		return
	}
	size := gtx.Constraints.Max
	pad := gtx.Dp(10)
	maxText := max(gtx.Dp(20), size.Y/3-2*pad)

	// The text first, in a recording, to know the dock's height.
	macro := op.Record(gtx.Ops)
	tgtx := gtx
	tgtx.Constraints = layout.Constraints{
		Min: image.Pt(size.X-4*pad, 0),
		Max: image.Pt(size.X-4*pad, maxText),
	}
	e := material.Editor(d.theme, &d.editor, d.placeholder)
	e.Color = color.NRGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF}
	e.HintColor = color.NRGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xFF}
	e.TextSize = unit.Sp(16)
	dims := e.Layout(tgtx)
	call := macro.Stop()

	d.height = dims.Size.Y + 4*pad
	top := size.Y - d.height
	cv := d.renderer.NewCanvas(gtx.Ops, image.Rect(0, 0, size.X, size.Y))
	cv.DrawRect(0, top, size.X, d.height, color.RGBA{0x20, 0x20, 0x20, 0xFF})
	cv.DrawRect(0, top, size.X, max(1, gtx.Dp(1)), color.RGBA{0x40, 0x40, 0x40, 0xFF})
	field := color.RGBA{0x14, 0x14, 0x14, 0xFF}
	if gtx.Focused(&d.editor) {
		field = color.RGBA{0x10, 0x10, 0x10, 0xFF}
	}
	cv.DrawRect(pad, top+pad, size.X-2*pad, d.height-2*pad, field)

	stack := op.Offset(image.Pt(2*pad, top+2*pad)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	stack.Pop()
}
