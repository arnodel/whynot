package main

import (
	"image"
	"image/color"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/exp/textinput"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"golang.org/x/image/font"

	"github.com/arnodel/whynot/canvas"
	"github.com/arnodel/whynot/fonts"
	"github.com/arnodel/whynot/internal/browser"
)

// promptField is the one-line text field shown while a link waits for the
// reader's text (see browser.App.Prompting). The Composer handles typing,
// including through an input method; the field handles editing keys.
type promptField struct {
	open  bool // shown last tick: text is kept until it closes
	text  string
	caret int // in bytes

	composer textinput.Composer
	// composition is the input method's text not yet committed, shown at
	// the caret, and compositionCaret the caret within it.
	composition      string
	compositionCaret int
	// caretBounds is where the caret was last drawn, in logical pixels,
	// for the input method's window.
	caretBounds image.Rectangle
}

func newPromptField() *promptField {
	p := &promptField{}
	p.composer.OnNewSession = func() *textinput.SessionOptions {
		return &textinput.SessionOptions{
			CaretBounds:     p.caretBounds,
			TextBeforeCaret: p.text[:p.caret],
			TextAfterCaret:  p.text[p.caret:],
		}
	}
	p.composer.OnComposition = func(c *textinput.Composition) {
		p.composition = c.Text()
		p.compositionCaret, _ = c.SelectionRangeInBytes()
	}
	p.composer.OnCommit = func(c *textinput.Commit) {
		if before, after := c.IsSurroundingTextReplaced(); before || after {
			// The session's surrounding text is the whole line.
			newBefore, newAfter := c.SurroundingText()
			p.text = newBefore + c.Text() + newAfter
			p.caret = len(newBefore) + len(c.Text())
			return
		}
		p.insert(c.Text())
	}
	return p
}

func (p *promptField) insert(s string) {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(s) // one line
	p.text = p.text[:p.caret] + s + p.text[p.caret:]
	p.caret += len(s)
}

// updatePrompt handles a tick's input while a link waits for text: typing,
// editing, Enter to follow the link, and Escape or a click outside the
// field to cancel.
func (g *game) updatePrompt() {
	p := g.prompt
	if !p.open {
		p.open, p.text, p.caret, p.composition = true, "", 0, ""
	}
	handled, err := p.composer.Update()
	if err != nil {
		log.Printf("text input: %v", err)
	}
	if handled {
		return
	}
	x, y, _, clicked := pointerState()
	shortcut := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter):
		p.composer.Confirm()
		p.open = false
		g.app.SubmitPrompt(p.text)
	case inpututil.IsKeyJustPressed(ebiten.KeyEscape) || (clicked && !image.Pt(x, y).In(g.promptBox())):
		p.composer.Cancel()
		p.open = false
		g.app.CancelPrompt()
	case shortcut && inpututil.IsKeyJustPressed(ebiten.KeyV):
		p.composer.Confirm()
		if text, err := browser.ClipboardText(); err == nil {
			p.insert(text)
		}
	case keyRepeat(ebiten.KeyBackspace):
		p.composer.Confirm()
		if p.caret > 0 {
			_, n := utf8.DecodeLastRuneInString(p.text[:p.caret])
			p.text = p.text[:p.caret-n] + p.text[p.caret:]
			p.caret -= n
		}
	case keyRepeat(ebiten.KeyLeft):
		p.composer.Confirm()
		_, n := utf8.DecodeLastRuneInString(p.text[:p.caret])
		p.caret -= n
	case keyRepeat(ebiten.KeyRight):
		p.composer.Confirm()
		_, n := utf8.DecodeRuneInString(p.text[p.caret:])
		p.caret += n
	case inpututil.IsKeyJustPressed(ebiten.KeyHome):
		p.composer.Confirm()
		p.caret = 0
	case inpututil.IsKeyJustPressed(ebiten.KeyEnd):
		p.composer.Confirm()
		p.caret = len(p.text)
	}
}

// promptBox is where the prompt is drawn: a box across the top of the
// document, at most promptWidth logical pixels wide.
func (g *game) promptBox() image.Rectangle {
	const promptWidth, promptHeight, margin = 560, 96, 16
	s := g.deviceScale
	w := min(int(promptWidth*s), g.width-int(2*margin*s))
	x := (g.width - w) / 2
	y := g.toolbarHeight + int(48*s)
	return image.Rect(x, y, x+w, y+int(promptHeight*s))
}

// drawPrompt draws the prompt over the dimmed document, while a link
// waits for the reader's text.
func (g *game) drawPrompt(cv canvas.Canvas) {
	s := g.deviceScale
	labelFace, err := g.toolbarFaceSelector.SelectFace(fonts.TextStyle{Size: 13}, s*72)
	if err != nil {
		return
	}
	textFace, err := g.toolbarFaceSelector.SelectFace(fonts.TextStyle{Size: 16}, s*72)
	if err != nil {
		return
	}
	cv.DrawRect(0, g.toolbarHeight, g.width, g.outsideHeightPx()-g.toolbarHeight, color.RGBA{0, 0, 0, 0x99})
	box := g.promptBox()
	cv.DrawRect(box.Min.X, box.Min.Y, box.Dx(), box.Dy(), color.RGBA{0x2A, 0x2A, 0x2A, 0xFF})
	drawOutline(cv, box, color.RGBA{0x55, 0x55, 0x55, 0xFF})

	pad := int(14 * s)
	label := image.Rect(box.Min.X+pad, box.Min.Y+pad/2, box.Max.X-pad, box.Min.Y+pad/2+int(24*s))
	cv.DrawText("Type your reply, then press Enter. Escape cancels.", labelFace, label.Min.X, baselineIn(labelFace, label), color.RGBA{0xAA, 0xAA, 0xAA, 0xFF})

	field := image.Rect(box.Min.X+pad, label.Max.Y+pad/2, box.Max.X-pad, box.Max.Y-pad)
	cv.DrawRect(field.Min.X, field.Min.Y, field.Dx(), field.Dy(), color.RGBA{0x14, 0x14, 0x14, 0xFF})

	// The text, with the input method's composition at the caret, scrolled
	// so the caret shows.
	p := g.prompt
	text := p.text[:p.caret] + p.composition + p.text[p.caret:]
	caret := p.caret + p.compositionCaret
	inner := field.Inset(int(8 * s))
	for font.MeasureString(textFace, text[:caret]).Ceil() > inner.Dx() {
		_, n := utf8.DecodeRuneInString(text)
		text, caret = text[n:], caret-n
	}
	baseline := baselineIn(textFace, field)
	cv.DrawText(text, textFace, inner.Min.X, baseline, color.RGBA{0xEE, 0xEE, 0xEE, 0xFF})
	m := textFace.Metrics()
	caretX := inner.Min.X + font.MeasureString(textFace, text[:caret]).Ceil()
	caretRect := image.Rect(caretX, baseline-m.Ascent.Ceil(), caretX+max(1, int(s)), baseline+m.Descent.Ceil())
	cv.DrawRect(caretRect.Min.X, caretRect.Min.Y, caretRect.Dx(), caretRect.Dy(), color.RGBA{0xEE, 0xEE, 0xEE, 0xFF})
	p.caretBounds = image.Rect(int(float64(caretRect.Min.X)/s), int(float64(caretRect.Min.Y)/s), int(float64(caretRect.Max.X)/s), int(float64(caretRect.Max.Y)/s))
}

// outsideHeightPx is the window's height in physical pixels.
func (g *game) outsideHeightPx() int {
	return int(float64(g.outsideHeight) * g.deviceScale)
}
