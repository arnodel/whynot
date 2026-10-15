package browser

import (
	"bytes"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/arnodel/whynot/browser/internal/whymark"
)

// A whymark page asks for the reader's text with an input block: a fenced
// code block whose info string is "input", the field's name and,
// optionally, where to send it (a link written as "#passage" or "?query";
// by default, the page's own passage), and whose content is the
// placeholder:
//
//	```input say
//	Say something…
//	```
//
// The browser shows a text field instead of the block, for as long as the
// page is current. Sending it takes a turn from the page, with the text as
// the field's input. In other Markdown viewers, the block shows as code.

// inputField is what a page's input block declares.
type inputField struct {
	name, target, placeholder string
}

// pageInput is a page's input field, as its Markdown arrives, and the
// reader's draft for it.
type pageInput struct {
	mu    sync.Mutex
	field *inputField
	// draft is what the reader typed in the field, or sent from it, when
	// the page was last current.
	draft string
}

func (p *pageInput) set(f *inputField) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.field = f
}

func (p *pageInput) get() *inputField {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.field
}

// inputFence matches the opening fence of an input block, at the top level
// of a page.
var inputFence = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})[ \t]*input(?:[ \t]+(\\S+))?(?:[ \t]+(\\S+))?[ \t]*$")

// inputFilter passes a page's Markdown on to w without its input blocks
// and its controls headings (see controls.go), which it passes on to
// source, if set, with the rest, and records the last input block's field
// in input.
type inputFilter struct {
	w      io.Writer
	source io.Writer
	input  *pageInput

	pending []byte // the end of what was written, an unfinished line
	// passing says the unfinished line is being passed on as it arrives:
	// its start showed it's an ordinary line (see passable).
	passing bool
	// skipBlank says a line was just taken out, so a blank line after it
	// is too: it separated what was taken out from what follows.
	skipBlank bool
	// fence is the fence of the code block being passed through, or
	// "" outside one; field is the input block being read, with its
	// fence in inputFence.
	fence      string
	field      *inputField
	inputFence string
	lines      []string
}

func (f *inputFilter) Write(p []byte) (int, error) {
	f.pending = append(f.pending, p...)
	for {
		i := bytes.IndexByte(f.pending, '\n')
		if i < 0 {
			break
		}
		line := string(f.pending[:i+1])
		f.pending = f.pending[i+1:]
		var err error
		if f.passing {
			f.passing = false
			err = f.pass(line)
		} else {
			err = f.line(line)
		}
		if err != nil {
			return len(p), err
		}
	}
	// A paragraph is often a single line, arriving word by word.
	if len(f.pending) > 0 && (f.passing || f.passable()) {
		f.passing = true
		line := string(f.pending)
		f.pending = f.pending[:0]
		if err := f.pass(line); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

// passable reports whether the unfinished line can be passed on before
// its end: it's not in an input block, and its start shows it can't be a
// fence or a controls heading, which only start with up to three spaces
// and a backquote, a tilde or a hash.
func (f *inputFilter) passable() bool {
	if f.field != nil {
		return false
	}
	start := bytes.TrimLeft(f.pending, " ")
	switch {
	case len(f.pending)-len(start) > 3:
		return true
	case len(start) == 0:
		return false
	}
	return !bytes.ContainsAny(start[:1], "`~#")
}

// pass passes on part of an ordinary line.
func (f *inputFilter) pass(s string) error {
	f.skipBlank = false
	if err := f.write(f.w, s); err != nil {
		return err
	}
	return f.write(f.source, s)
}

// Close passes on the end of the page.
func (f *inputFilter) Close() error {
	if len(f.pending) > 0 {
		var err error
		if f.passing {
			err = f.pass(string(f.pending))
		} else {
			err = f.line(string(f.pending))
		}
		if err != nil {
			return err
		}
		f.pending = nil
	}
	return nil
}

// line handles a line of the page, its newline included.
func (f *inputFilter) line(line string) error {
	trimmed := strings.TrimRight(line, "\r\n")
	closes := func(fence string) bool {
		t := strings.TrimSpace(trimmed)
		return len(t) >= len(fence) && t[0] == fence[0] && strings.Trim(t, t[:1]) == ""
	}
	switch {
	case f.field != nil:
		if closes(f.inputFence) {
			f.field.placeholder = strings.TrimSpace(strings.Join(f.lines, " "))
			f.input.set(f.field)
			f.field, f.lines, f.skipBlank = nil, nil, true
		} else {
			f.lines = append(f.lines, strings.TrimSpace(trimmed))
		}
		return nil
	case f.fence != "":
		if closes(f.fence) {
			f.fence = ""
		}
	default:
		if skip := f.skipBlank; skip {
			f.skipBlank = false
			if strings.TrimSpace(trimmed) == "" {
				return f.write(f.source, line)
			}
		}
		if m := inputFence.FindStringSubmatch(trimmed); m != nil && m[2] != "" {
			f.field, f.inputFence = &inputField{name: m[2], target: m[3]}, m[1]
			return nil
		}
		if controlsHeading.MatchString(trimmed) {
			// Quiet: kept only where the controls are looked for.
			f.skipBlank = true
			return f.write(f.source, line)
		}
		if t := strings.TrimLeft(trimmed, " "); strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			f.fence = t[:strings.IndexFunc(t+"x", func(r rune) bool { return r != rune(t[0]) })]
		}
	}
	if err := f.write(f.w, line); err != nil {
		return err
	}
	return f.write(f.source, line)
}

// write writes line to w, if it's set.
func (f *inputFilter) write(w io.Writer, line string) error {
	if w == nil {
		return nil
	}
	_, err := io.WriteString(w, line)
	return err
}

// currentInput returns the current page's input, if it's a whymark page,
// and it's showing: not under an overlay.
func (a *App) currentInput() (*whymarkPage, *inputField) {
	if a.overlaid != nil {
		return nil, nil
	}
	p := a.pageOf(a.Panel.View().Document())
	if p == nil {
		return nil, nil
	}
	return p, p.input.get()
}

// Input reports whether the current page has an input field, and its
// placeholder. The app shows a text field for it, whose text it passes to
// SubmitInput, keeping what's typed with SetInputDraft.
func (a *App) Input() (placeholder string, ok bool) {
	_, f := a.currentInput()
	if f == nil {
		return "", false
	}
	return f.placeholder, true
}

// SubmitInput sends text from the current page's input field: a turn from
// the page, to the field's target, with text as the field's input. The
// text stays the page's draft, so that coming back to the page shows what
// was sent from it.
func (a *App) SubmitInput(text string) {
	p, f := a.currentInput()
	if f == nil {
		return
	}
	target := f.target
	if target == "" {
		target = "?" // the page's own passage
	}
	req, ok := whymark.Follow(p.header, target)
	if !ok {
		return
	}
	req.Input = map[string]string{f.name: text}
	p.input.mu.Lock()
	p.input.draft = text
	p.input.mu.Unlock()
	a.followTurn(req)
}

// InputDraft returns the text for the current page's input field: what the
// reader typed in it, or sent from it, when the page was last current,
// empty for a new page. version changes whenever the current page does,
// when the field should show the text.
func (a *App) InputDraft() (text string, version int) {
	if view := a.Panel.View(); view != a.draftView {
		a.draftView = view
		a.draftVersion++
	}
	if p := a.pageOf(a.Panel.View().Document()); p != nil {
		p.input.mu.Lock()
		defer p.input.mu.Unlock()
		return p.input.draft, a.draftVersion
	}
	return "", a.draftVersion
}

// SetInputDraft records the text in the current page's input field, as the
// reader types, so that it's there when they come back to the page.
func (a *App) SetInputDraft(text string) {
	if p := a.pageOf(a.Panel.View().Document()); p != nil {
		p.input.mu.Lock()
		defer p.input.mu.Unlock()
		p.input.draft = text
	}
}

// Loading reports whether the current page is still arriving.
func (a *App) Loading() bool {
	return !a.Panel.View().Document().Complete()
}

// SetBottomBar sets the height, in pixels, of what the app draws below the
// document, such as an input field: the document is shown above it.
func (a *App) SetBottomBar(height int) { a.bottomBar = height }

// ClipboardText returns the clipboard's text, for pasting into a text
// field.
func ClipboardText() (string, error) { return readClipboard() }
