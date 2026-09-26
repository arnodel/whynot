package main

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

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

	faceSelector whynot.FaceSelector
	renderer     *giorenderer.Renderer
}

// update checks each button for a click and dispatches the
// corresponding App action - must run before layout in the same frame:
// widget.Clickable.Layout drains the same click event queue Clicked
// reads from, so calling Clicked after Layout would always see nothing.
func (tb *toolbar) update(gtx layout.Context, app *browser.App) {
	if tb.back.Clicked(gtx) {
		app.Back()
	}
	if tb.forward.Clicked(gtx) {
		app.Forward()
	}
	if tb.reload.Clicked(gtx) {
		app.Reload()
	}
	if tb.zoomOut.Clicked(gtx) {
		app.ZoomOut()
	}
	if tb.zoomIn.Clicked(gtx) {
		app.ZoomIn()
	}
	if tb.theme.Clicked(gtx) {
		app.SetTheme(!app.DarkTheme())
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

// addressBar returns a layout.Widget showing app's current document
// location, or - while hovering a link - that link's destination
// instead, in StyleSheet.HighlightColor to match the hovered link's
// own color in the document.
func (tb *toolbar) addressBar(app *browser.App) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
		})
	}
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
