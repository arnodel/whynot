package main

import (
	"image"
	"image/color"
	"image/draw"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
)

// tintedIconKey/tintedIconCache cache a toolbar icon tinted to a given
// color, keyed by (image, color) identity - the same technique
// giorenderer.Renderer.glyphFor uses for colored glyphs (Gio has no
// raster-mask-as-clip primitive to tint at draw time, so color has to
// be baked into the bitmap). Kept private to this package rather than
// added to whynot.Canvas or giorenderer: it's a toolbar-only need, not
// a document-rendering one - see browser package's own doc comment.
type tintedIconKey struct {
	img image.Image
	clr color.Color
}

var tintedIconCache = map[tintedIconKey]paint.ImageOp{}

func tintedIcon(img image.Image, clr color.Color) paint.ImageOp {
	key := tintedIconKey{img, clr}
	if op, ok := tintedIconCache[key]; ok {
		return op
	}
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	draw.DrawMask(rgba, b, image.NewUniform(clr), image.Point{}, img, b.Min, draw.Over)
	imgOp := paint.NewImageOp(rgba)
	tintedIconCache[key] = imgOp
	return imgOp
}

// paintIcon paints icon (assumed square) tinted to clr, scaled to fill
// a size x size box at the ops stream's current origin.
func paintIcon(gtx layout.Context, icon image.Image, size int, clr color.Color) {
	imgOp := tintedIcon(icon, clr)
	b := icon.Bounds()
	scale := float32(size) / float32(b.Dx())
	transform := f32.Affine2D{}.Scale(f32.Pt(0, 0), f32.Pt(scale, scale))
	stack := op.Affine(transform).Push(gtx.Ops)
	imgOp.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	stack.Pop()
}
