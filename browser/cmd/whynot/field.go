package main

import (
	"image"
	"image/color"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/exp/textinput"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"golang.org/x/image/font"

	"github.com/arnodel/whynot/browser/internal/browser"
	"github.com/arnodel/whynot/canvas"
	"github.com/arnodel/whynot/fonts"
)

// inputField is the one-line text field for the current page's input
// block (see browser.App.Input), opened from the strip below the document.
// The Composer handles typing, including through an input method; the
// field handles editing keys.
type inputField struct {
	open  bool // shown: text is kept until it closes
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
	// listener turns speech into the field's text (see the microphone
	// button). sendHeard is set when the send button is pressed while
	// it's busy: the text is sent once its last words are in.
	listener  *browser.Listener
	sendHeard bool
	// micHover and sendHover are whether the pointer is over the
	// microphone button and the send button.
	micHover, sendHover bool
}

func newInputField() *inputField {
	p := &inputField{listener: browser.NewListener(browser.DefaultRecognizer(), nil)}
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

// submitField closes the field and sends its text, if it has some, on
// the next update (see game.submit).
func (g *game) submitField() {
	p := g.field
	p.composer.Confirm()
	p.open = false
	p.listener.Cancel()
	g.submit = p.text
}

// closeField closes the field, keeping its text as the page's draft.
func (g *game) closeField() {
	p := g.field
	p.composer.Cancel()
	p.open = false
	p.listener.Cancel()
	g.app.SetInputDraft(p.text)
}

// listen starts the listener, or stops it if it's listening.
func (p *inputField) listen() {
	if p.listener.Listening() {
		p.listener.Stop()
		return
	}
	p.composer.Confirm()
	p.listener.Start(p.text)
}

func (p *inputField) insert(s string) {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(s) // one line
	p.text = p.text[:p.caret] + s + p.text[p.caret:]
	p.caret += len(s)
}

// openField opens the field, with what was typed or sent on this page
// before.
func (g *game) openField() {
	p := g.field
	draft, _ := g.app.InputDraft()
	p.open, p.text, p.caret, p.composition = true, draft, len(draft), ""
}

// updateField handles a tick's input while the field is open: typing,
// editing, Enter to send the text, and Escape or a click outside the
// field to close it, keeping what was typed.
func (g *game) updateField() {
	p := g.field
	if _, ok := g.app.Input(); !ok {
		p.open = false // the page changed under it
		p.listener.Cancel()
		return
	}
	handled, err := p.composer.Update()
	if err != nil {
		log.Printf("text input: %v", err)
	}
	if text, ok := p.listener.Text(); ok {
		// What's been said so far.
		p.text, p.caret = text, len(text)
	}
	if p.sendHeard {
		// Waiting for the last words.
		if !p.listener.Busy() {
			p.sendHeard = false
			g.submitField()
		}
		return
	}
	// Clicks first: while the input method has text in progress, the
	// Composer handles every update, and the buttons must still work.
	x, y, _, clicked := pointerState()
	_, _, send, mic := g.fieldRects()
	p.micHover, p.sendHover = image.Pt(x, y).In(mic), image.Pt(x, y).In(send)
	if clicked {
		switch {
		case p.micHover:
			p.listen()
		case p.sendHover && p.listener.Busy():
			p.listener.Stop()
			p.sendHeard = true
		case p.sendHover:
			g.submitField()
		case !image.Pt(x, y).In(g.fieldBox()):
			g.closeField()
		}
		return
	}
	if handled {
		return
	}
	shortcut := ebiten.IsKeyPressed(ebiten.KeyMeta) || ebiten.IsKeyPressed(ebiten.KeyControl)
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter):
		g.submitField()
	case inpututil.IsKeyJustPressed(ebiten.KeyEscape):
		g.closeField()
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

// promptButtons returns where the send and microphone buttons go at the
// right end of box: squares inset from its edges, the microphone last.
// Each is an empty rectangle when it isn't shown: send unless withSend,
// as when there's text to send, and the microphone where dictation isn't
// available (see browser.Listener.Available).
func (g *game) promptButtons(box image.Rectangle, inset int, withSend bool) (send, mic image.Rectangle) {
	size := box.Dy() - 2*inset
	right := box.Max.X - inset
	next := func() image.Rectangle {
		r := image.Rect(right-size, box.Min.Y+inset, right, box.Max.Y-inset)
		right -= size + inset
		return r
	}
	if g.field.listener.Available() {
		mic = next()
	}
	if withSend {
		send = next()
	}
	return send, mic
}

// drawPromptButtons draws the send and microphone buttons, where they're
// shown: the microphone pressed while it listens, so that it reads
// as the way to stop it.
func (g *game) drawPromptButtons(dst *ebiten.Image, cv canvas.Canvas, send, mic image.Rectangle) {
	p := g.field
	if !send.Empty() {
		drawButton(dst, cv, sendIcon, send, true, buttonState{hover: p.sendHover})
	}
	if !mic.Empty() {
		if fill, ok := p.listener.MicColor(time.Now()); ok {
			cv.DrawRect(mic.Min.X, mic.Min.Y, mic.Dx(), mic.Dy(), fill)
			drawIcon(dst, micIcon, mic, color.White)
		} else {
			drawButton(dst, cv, micIcon, mic, true, buttonState{hover: p.micHover})
		}
	}
}

// fieldBox is where the field is drawn: a box across the top of the
// document, at most promptWidth logical pixels wide.
func (g *game) fieldBox() image.Rectangle {
	const promptWidth, promptHeight, margin = 560, 96, 16
	s := g.deviceScale
	w := min(int(promptWidth*s), g.width-int(2*margin*s))
	x := (g.width - w) / 2
	y := g.toolbarHeight + int(48*s)
	return image.Rect(x, y, x+w, y+int(promptHeight*s))
}

// fieldRects returns where, in fieldBox, the field's hint, its text and
// its buttons go (see promptButtons): send while there's text, dictated
// or still being composed included.
func (g *game) fieldRects() (label, field, send, mic image.Rectangle) {
	s := g.deviceScale
	box := g.fieldBox()
	pad := int(14 * s)
	label = image.Rect(box.Min.X+pad, box.Min.Y+pad/2, box.Max.X-pad, box.Min.Y+pad/2+int(24*s))
	field = image.Rect(box.Min.X+pad, label.Max.Y+pad/2, box.Max.X-pad, box.Max.Y-pad)
	p := g.field
	send, mic = g.promptButtons(field, 0, p.text != "" || p.composition != "")
	for _, b := range []image.Rectangle{mic, send} {
		if !b.Empty() {
			field.Max.X = b.Min.X - pad/2
		}
	}
	return label, field, send, mic
}

// drawField draws the field over the dimmed document, while it's open.
func (g *game) drawField(dst *ebiten.Image, cv canvas.Canvas) {
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
	box := g.fieldBox()
	cv.DrawRect(box.Min.X, box.Min.Y, box.Dx(), box.Dy(), color.RGBA{0x2A, 0x2A, 0x2A, 0xFF})
	drawOutline(cv, box, color.RGBA{0x55, 0x55, 0x55, 0xFF})

	label, field, send, mic := g.fieldRects()
	hint := "Type, then press Enter. Escape leaves it for later."
	if placeholder, _ := g.app.Input(); placeholder != "" {
		hint = placeholder + "  (Enter sends, Escape leaves it for later)"
	}
	if g.field.sendHeard {
		hint = "Sending…"
	}
	cv.DrawText(hint, labelFace, label.Min.X, baselineIn(labelFace, label), color.RGBA{0xAA, 0xAA, 0xAA, 0xFF})

	cv.DrawRect(field.Min.X, field.Min.Y, field.Dx(), field.Dy(), color.RGBA{0x14, 0x14, 0x14, 0xFF})
	g.drawPromptButtons(dst, cv, send, mic)

	// The text, with the input method's composition at the caret, scrolled
	// so the caret shows.
	p := g.field
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

// stripHeight is the height of the strip below the document, in logical
// pixels, while the page has an input block.
const stripHeight = 36

// updateStrip shows the strip while the current page has an input block
// (see browser.App.Input), and opens the field when it's clicked or Enter
// is pressed.
func (g *game) updateStrip(cursor image.Point, clicked bool) {
	_, ok := g.app.Input()
	switch {
	case ok:
		g.strip = true
	// A page usually has its input block at its end: until the page is
	// complete, the strip stays.
	case !g.app.Loading():
		g.strip = false
	}
	if !g.strip || !ok {
		return
	}
	send, mic := g.stripButtons()
	g.field.micHover, g.field.sendHover = cursor.In(mic), cursor.In(send)
	switch {
	case clicked && g.field.micHover:
		g.openField()
		g.field.listen()
		return
	case clicked && g.field.sendHover:
		g.submit, _ = g.app.InputDraft()
		return
	}
	if (clicked && cursor.In(g.stripBox())) || inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		g.openField()
	}
}

// stripButtons is promptButtons for the strip: send while there's a draft.
func (g *game) stripButtons() (send, mic image.Rectangle) {
	draft, _ := g.app.InputDraft()
	return g.promptButtons(g.stripBox(), int(6*g.deviceScale), draft != "")
}

// stripBox is where the strip is drawn: below the document.
func (g *game) stripBox() image.Rectangle {
	return image.Rect(0, g.outsideHeightPx()-g.stripHeightPx(), g.width, g.outsideHeightPx())
}

// stripHeightPx is the strip's height in physical pixels, or 0 when it
// isn't shown.
func (g *game) stripHeightPx() int {
	if !g.strip {
		return 0
	}
	return int(stripHeight * g.deviceScale)
}

// drawStrip draws the strip, with the input block's placeholder, or the
// draft if there's one, and its buttons at its right end.
func (g *game) drawStrip(dst *ebiten.Image, cv canvas.Canvas) {
	box := g.stripBox()
	cv.DrawRect(box.Min.X, box.Min.Y, box.Dx(), box.Dy(), color.RGBA{0x20, 0x20, 0x20, 0xFF})
	cv.DrawRect(box.Min.X, box.Min.Y, box.Dx(), max(1, int(g.deviceScale)), color.RGBA{0x40, 0x40, 0x40, 0xFF})
	face, err := g.toolbarFaceSelector.SelectFace(fonts.TextStyle{Size: 14}, g.deviceScale*72)
	if err != nil {
		return
	}
	label, clr := "Press Enter to write", color.RGBA{0x99, 0x99, 0x99, 0xFF}
	if placeholder, ok := g.app.Input(); ok && placeholder != "" {
		label = placeholder + "  (Enter)"
	}
	if draft, _ := g.app.InputDraft(); draft != "" {
		label, clr = draft, color.RGBA{0xCC, 0xCC, 0xCC, 0xFF}
	}
	cv.DrawText(label, face, box.Min.X+int(16*g.deviceScale), baselineIn(face, box), clr)
	send, mic := g.stripButtons()
	g.drawPromptButtons(dst, cv, send, mic)
}
