package main

import (
	"image"
	"image/color"

	"gioui.org/font/gofont"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/arnodel/whynot/backends/giobackend"
	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/internal/browser"
)

// prompt is the text field shown while a link waits for the reader's
// text (see browser.App.Prompting).
type prompt struct {
	editor   widget.Editor
	theme    *material.Theme
	open     bool // shown last frame
	renderer *giobackend.Renderer
	faces    fonts.FaceSelector
}

func newPrompt(faces fonts.FaceSelector, renderer *giobackend.Renderer) *prompt {
	p := &prompt{theme: material.NewTheme(), renderer: renderer, faces: faces}
	// As for the address bar: Gio's default shaper has no fonts in the
	// browser.
	p.theme.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	p.editor.SingleLine = true
	p.editor.Submit = true
	return p
}

// update opens the field when a link starts waiting, and handles Enter,
// which follows the link with the text, and Escape, which cancels.
func (p *prompt) update(gtx layout.Context, app *browser.App) {
	if !app.Prompting() {
		p.open = false
		return
	}
	if !p.open {
		p.open = true
		p.editor.SetText("")
		gtx.Execute(key.FocusCmd{Tag: &p.editor})
		gtx.Execute(key.SoftKeyboardCmd{Show: true}) // see updateAddressBar
	}
	for {
		e, ok := p.editor.Update(gtx)
		if !ok {
			break
		}
		if submit, ok := e.(widget.SubmitEvent); ok {
			p.open = false
			app.SubmitPrompt(submit.Text)
			return
		}
	}
	for {
		e, ok := gtx.Event(key.Filter{Focus: &p.editor, Name: key.NameEscape})
		if !ok {
			break
		}
		if ke, ok := e.(key.Event); ok && ke.State == key.Press {
			p.open = false
			app.CancelPrompt()
			return
		}
	}
}

// layout draws the field, over the dimmed document below the toolbar.
func (p *prompt) layout(gtx layout.Context, toolbarHeight int) {
	if !p.open {
		return
	}
	size := gtx.Constraints.Max
	cv := p.renderer.NewCanvas(gtx.Ops, image.Rect(0, 0, size.X, size.Y))
	cv.DrawRect(0, toolbarHeight, size.X, size.Y-toolbarHeight, color.RGBA{0, 0, 0, 0x99})

	w := min(gtx.Dp(560), size.X-gtx.Dp(32))
	x, y := (size.X-w)/2, toolbarHeight+gtx.Dp(48)
	box := image.Rect(x, y, x+w, y+gtx.Dp(96))
	cv.DrawRect(box.Min.X, box.Min.Y, box.Dx(), box.Dy(), color.RGBA{0x2A, 0x2A, 0x2A, 0xFF})
	pad := gtx.Dp(14)
	if face, err := p.faces.SelectFace(fonts.TextStyle{Size: 13}, float64(gtx.Metric.PxPerDp)*72); err == nil {
		cv.DrawText("Type your reply, then press Enter. Escape cancels.", face, box.Min.X+pad, box.Min.Y+pad+face.Metrics().Ascent.Ceil(), color.RGBA{0xAA, 0xAA, 0xAA, 0xFF})
	}
	field := image.Rect(box.Min.X+pad, box.Min.Y+gtx.Dp(40), box.Max.X-pad, box.Max.Y-pad)
	cv.DrawRect(field.Min.X, field.Min.Y, field.Dx(), field.Dy(), color.RGBA{0x14, 0x14, 0x14, 0xFF})

	inset := gtx.Dp(8)
	stack := op.Offset(field.Min.Add(image.Pt(inset, inset))).Push(gtx.Ops)
	gtx.Constraints = layout.Exact(field.Size().Sub(image.Pt(2*inset, 2*inset)))
	e := material.Editor(p.theme, &p.editor, "")
	e.Color = color.NRGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF}
	e.TextSize = unit.Sp(16)
	e.Layout(gtx)
	stack.Pop()
}
