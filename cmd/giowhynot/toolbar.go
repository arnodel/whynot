package main

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/browser"
	"github.com/arnodel/whynot/giorenderer"
)

// toolbarLogicalHeight is the address bar / button row's height, in dp
// - matches cmd/whynot's own toolbarLogicalHeight.
const toolbarLogicalHeight = 36

// toolbar is the back/forward/reload/zoom/theme button row and address
// bar - cmd/giowhynot's own native-Gio counterpart to cmd/whynot's
// hand-rolled draw.go/layout.go/input.go. Unlike the document panel's
// scroll/hover/click (shared via whynot.Interaction), this input layer
// isn't shared with ebitenrenderer: these are plain momentary buttons,
// simple enough that Gio's own widget.Clickable is a better fit than
// extracting a second shared implementation - see the browser package's
// own doc comment.
type toolbar struct {
	back, forward, reload  widget.Clickable
	zoomOut, zoomIn, theme widget.Clickable

	// addressClick wraps the read-only address bar display (only laid
	// out while !editing) purely to detect a click-to-edit, the same
	// way an icon button's own Clickable does.
	addressClick widget.Clickable
	// addressEditor is laid out instead of the read-only display while
	// editing is true - see updateAddressBar.
	addressEditor widget.Editor
	editing       bool
	// editTheme is only for addressEditor's own chrome (cursor,
	// selection, font/shaper) - Gio's own text stack, not whynot's
	// font.Face, and unrelated to giorenderer.Panel's own theme (used
	// only for its scrollbar).
	editTheme *material.Theme

	faceSelector whynot.FaceSelector
	renderer     *giorenderer.Renderer
}

func newToolbar(faceSelector whynot.FaceSelector, renderer *giorenderer.Renderer) *toolbar {
	tb := &toolbar{
		faceSelector: faceSelector,
		renderer:     renderer,
		editTheme:    material.NewTheme(),
	}
	tb.addressEditor.SingleLine = true
	tb.addressEditor.Submit = true
	return tb
}

// update checks each button for a click and dispatches the
// corresponding App action - must run before layout in the same frame:
// widget.Clickable.Layout drains the same click event queue Clicked
// reads from, so calling Clicked after Layout would always see nothing.
func (tb *toolbar) update(gtx layout.Context, app *browser.App) {
	backClicked := tb.back.Clicked(gtx)
	forwardClicked := tb.forward.Clicked(gtx)
	reloadClicked := tb.reload.Clicked(gtx)
	zoomOutClicked := tb.zoomOut.Clicked(gtx)
	zoomInClicked := tb.zoomIn.Clicked(gtx)
	themeClicked := tb.theme.Clicked(gtx)

	// Gio's key-focus model is independent of pointer clicks (clicking
	// a widget that doesn't itself claim key focus never blurs whatever
	// does) - so clicking any of these while editing the address bar
	// needs an explicit cancel, same reason as Panel.OnPress below.
	if backClicked || forwardClicked || reloadClicked || zoomOutClicked || zoomInClicked || themeClicked {
		tb.cancelEdit()
	}

	if backClicked {
		app.Back()
	}
	if forwardClicked {
		app.Forward()
	}
	if reloadClicked {
		app.Reload()
	}
	if zoomOutClicked {
		app.ZoomOut()
	}
	if zoomInClicked {
		app.ZoomIn()
	}
	if themeClicked {
		app.SetTheme(!app.DarkTheme())
	}
	tb.updateAddressBar(gtx, app)
}

// cancelEdit reverts the address bar to its read-only display without
// navigating anywhere. Gio's key-focus model is independent of pointer
// clicks (confirmed against io/input/pointer.go having no focus logic
// at all): a click on something that doesn't itself claim key focus -
// which is everything else in this app - never blurs addressEditor, so
// gtx.Focused(&tb.addressEditor) alone can't detect "clicked away"; this
// is called explicitly instead, from Escape, and from anything else
// that itself represents "the user clicked away" (toolbar buttons in
// update, giorenderer.Panel.OnPress for the document).
func (tb *toolbar) cancelEdit() {
	tb.editing = false
}

// updateAddressBar drives the click-to-edit / submit / cancel state
// machine - see cancelEdit's own doc comment for why blur can't just be
// read off gtx.Focused. Also note: cancelEdit must never run on the
// same frame editing turns true (the click-to-edit branch below runs
// after this one specifically so a same-frame cancelEdit call earlier
// in this same Update pass - e.g. from a stale click - can't undo it).
func (tb *toolbar) updateAddressBar(gtx layout.Context, app *browser.App) {
	if tb.editing {
		for {
			e, ok := tb.addressEditor.Update(gtx)
			if !ok {
				break
			}
			if submit, ok := e.(widget.SubmitEvent); ok {
				if err := app.Navigate(strings.TrimSpace(submit.Text)); err == nil {
					tb.cancelEdit()
				}
			}
		}
		for {
			e, ok := gtx.Event(key.Filter{Focus: &tb.addressEditor, Name: key.NameEscape})
			if !ok {
				break
			}
			if ke, ok := e.(key.Event); ok && ke.State == key.Press {
				tb.cancelEdit()
			}
		}
	}

	if tb.addressClick.Clicked(gtx) {
		tb.addressEditor.SetText(app.Location().String())
		tb.addressEditor.SetCaret(tb.addressEditor.Len(), tb.addressEditor.Len())
		tb.editing = true
		gtx.Execute(key.FocusCmd{Tag: &tb.addressEditor})
	}
}

// layout draws the toolbar at the top of gtx's constraints (full
// width, toolbarLogicalHeight tall) and registers its buttons' hit
// areas for next frame.
func (tb *toolbar) layout(gtx layout.Context, app *browser.App) layout.Dimensions {
	height := gtx.Dp(unit.Dp(toolbarLogicalHeight))
	pad := gtx.Dp(unit.Dp(8))
	btn := height - 2*pad
	width := gtx.Constraints.Max.X

	canvas := tb.renderer.NewCanvas(gtx.Ops, image.Rect(0, 0, width, height))
	canvas.DrawRect(0, 0, width, height, color.RGBA{0x20, 0x20, 0x20, 0xFF})

	themeIcon := browser.LightModeIcon
	if app.DarkTheme() {
		themeIcon = browser.DarkModeIcon
	}

	gtx.Constraints = layout.Exact(image.Pt(width, height))
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(hspace(pad)),
		layout.Rigid(iconButton(&tb.back, browser.BackIcon, btn, app.CanGoBack())),
		layout.Rigid(hspace(pad)),
		layout.Rigid(iconButton(&tb.forward, browser.ForwardIcon, btn, app.CanGoForward())),
		layout.Rigid(hspace(pad)),
		layout.Rigid(iconButton(&tb.reload, browser.ReloadIcon, btn, true)),
		layout.Flexed(1, tb.addressBar(app)),
		layout.Rigid(iconButton(&tb.zoomOut, browser.ZoomOutIcon, btn, true)),
		layout.Rigid(hspace(pad)),
		layout.Rigid(iconButton(&tb.zoomIn, browser.ZoomInIcon, btn, true)),
		layout.Rigid(hspace(pad)),
		layout.Rigid(iconButton(&tb.theme, themeIcon, btn, true)),
		layout.Rigid(hspace(pad)),
	)
}

// addressBar returns a layout.Widget showing - while not editing -
// app's current document location, or - while hovering a link - that
// link's destination instead, in StyleSheet.HighlightColor to match
// the hovered link's own color in the document. Clicking it (via
// addressClick) or the whole widget while editing is true switches to
// a real text field (see updateAddressBar) - typed text is committed
// as a navigation on Enter, discarded on Escape or clicking away.
func (tb *toolbar) addressBar(app *browser.App) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if tb.editing {
				return tb.layoutEditor(gtx)
			}
			return tb.addressClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return tb.layoutLocation(gtx, app)
			})
		})
	}
}

// layoutEditor draws addressEditor in Gio's own font/shaper (see this
// package's own doc notes on the font-stack mismatch) - a minor,
// transient visual difference from the read-only display's whynot-face
// text, only visible while actually typing. Its own natural height
// (line-height plus internal padding) doesn't match the address bar's
// fixed row height, so it's vertically centered within it by hand
// (recording its draw ops, then replaying them at a Y offset) rather
// than left at Editor's own default top alignment - not via
// layout.Center, which centers *both* axes: Editor reports its own
// natural (text-content) width rather than filling the available
// width the way a real address bar needs to, so centering
// horizontally too visibly shrank and centered the whole field.
func (tb *toolbar) layoutEditor(gtx layout.Context) layout.Dimensions {
	e := material.Editor(tb.editTheme, &tb.addressEditor, "")
	e.Color = color.NRGBA{R: 0xCC, G: 0xCC, B: 0xCC, A: 0xFF}
	e.TextSize = unit.Sp(14)

	size := gtx.Constraints.Max
	// Min.X = size.X forces full width, left-aligned (Editor's own
	// natural width is just its text content). Min.Y = 0 (relaxed from
	// whatever exact/forced constraint this widget inherited from the
	// Flex row - Min == Max there, which is why Editor reported its
	// height as the *whole* row and rendered top-aligned within it) so
	// Editor instead reports its own smaller natural single-line
	// height, which offY below then centers.
	gtx.Constraints.Min = image.Pt(size.X, 0)

	macro := op.Record(gtx.Ops)
	dims := e.Layout(gtx)
	call := macro.Stop()

	offY := (size.Y - dims.Size.Y) / 2
	if offY < 0 {
		offY = 0
	}
	stack := op.Offset(image.Pt(0, offY)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	stack.Pop()

	return layout.Dimensions{Size: size}
}

// layoutLocation draws the read-only address bar text via giorenderer,
// matching cmd/whynot's own address bar exactly (same face, same
// middle-truncation for a long path/URL).
func (tb *toolbar) layoutLocation(gtx layout.Context, app *browser.App) layout.Dimensions {
	size := gtx.Constraints.Max
	face, err := tb.faceSelector.SelectFace(whynot.TextStyle{Size: 14})
	if err != nil {
		return layout.Dimensions{Size: size}
	}
	text, textColor := app.Location().String(), color.Color(color.RGBA{0xCC, 0xCC, 0xCC, 0xFF})
	if hoverDest := app.HoverDest(); hoverDest != "" {
		text, textColor = hoverDest, app.StyleSheet().HighlightColor()
	}
	text = truncateMiddle(face, text, size.X)
	canvas := tb.renderer.NewCanvas(gtx.Ops, image.Rectangle{Max: size})
	canvas.DrawText(text, face, 0, baselineIn(face, image.Rectangle{Max: size}), textColor)
	return layout.Dimensions{Size: size}
}

// iconButton returns a layout.Widget drawing an icon button filling a
// size x size box - no border, no margin around the icon, just a
// background fill (for hover/pressed feedback) showing through
// wherever the icon's own transparency lets it. enabled only affects
// appearance; Back is still harmless to click with no history, so
// nothing needs disabling functionally (see buttonColors).
func iconButton(click *widget.Clickable, icon image.Image, size int, enabled bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = layout.Exact(image.Pt(size, size))
		return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			fill, tint := buttonColors(enabled, click.Hovered(), click.Pressed())
			if fill != nil {
				paint.FillShape(gtx.Ops, toNRGBA(fill), clip.Rect(image.Rect(0, 0, size, size)).Op())
			}
			paintIcon(gtx, icon, size, tint)
			return layout.Dimensions{Size: image.Pt(size, size)}
		})
	}
}

// hspace returns a fixed-width, zero-height spacer widget.
func hspace(px int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(px, 0)}
	}
}

// buttonColors picks a button's fill/icon-tint colors for its current
// state - a disabled button ignores hover/pressed entirely, reading as
// inert regardless of where the cursor is. fillColor is nil for "no
// fill", i.e. the toolbar's own background shows through. Matches
// cmd/whynot's own draw.go palette.
func buttonColors(enabled, hovered, pressed bool) (fillColor, iconTint color.Color) {
	switch {
	case !enabled:
		return nil, color.RGBA{0x60, 0x60, 0x60, 0xFF}
	case pressed:
		return color.RGBA{0x50, 0x50, 0x50, 0xFF}, color.White
	case hovered:
		return color.RGBA{0x30, 0x30, 0x30, 0xFF}, color.RGBA{0xF0, 0xF0, 0xF0, 0xFF}
	default:
		return nil, color.RGBA{0xE0, 0xE0, 0xE0, 0xFF}
	}
}

func toNRGBA(c color.Color) color.NRGBA {
	return color.NRGBAModel.Convert(c).(color.NRGBA)
}
