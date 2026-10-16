package browser

import (
	"image"
	"image/color"
	"log"
	"net/url"
	"slices"
	"time"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/canvas"
	"github.com/arnodel/whynot/input"
	"github.com/arnodel/whynot/styles/simpletheme"
)

// journal is journal mode's state: the stack showing the trail of pages,
// and what each of its children is.
type journal struct {
	stack      *ViewStack
	controller *StackController
	pages      []trailPage
}

// trailPage is a page of the trail: where it was loaded from, and the
// first history entry for it, or -1 for the current page.
type trailPage struct {
	location *url.URL
	first    int
}

// commandStep and pageOverlap are how far ScrollUp and ScrollDown move,
// in logical pixels, and the fraction of a page PageUp and PageDown keep
// in view, as a Panel's do.
const (
	commandStep = 40
	pageOverlap = 0.1
)

// Journal reports whether journal mode is on (see ToggleJournal).
func (a *App) Journal() bool { return a.journal != nil }

// stacked returns journal mode's state while its stack is what's shown:
// in journal mode, unless the TOC is showing instead. It's nil otherwise.
func (a *App) stacked() *journal {
	if a.overlaid != nil {
		return nil
	}
	return a.journal
}

// ToggleJournal turns journal mode on or off. In journal mode, the pages
// history went through are shown above the current one, oldest first, as
// one scrollable column: each page in its own View, the same one it has
// alone, so turning it on or off loses nothing. Turning it on keeps the
// current page where it was, with the trail above it.
//
// A link followed from an earlier page goes on from that page: the pages
// that came after it are dropped from history. A no-op while the TOC is
// showing.
func (a *App) ToggleJournal() {
	if a.overlaid != nil {
		return
	}
	a.arrival = arrival{}
	if a.journal != nil {
		a.journal = nil
		a.Panel.SetBounds(a.bounds)
		a.Panel.SetScale(a.deviceScale)
		a.Panel.SetZoom(a.zoom)
		a.Panel.SetView(a.Panel.View()) // its scrollbar back
		return
	}
	a.journal = &journal{}
	a.restack()
}

// restack rebuilds the journal's stack from the trail, keeping the page
// at its top there if it's still in the trail, else putting the current
// page there, each at its own scroll position. A no-op outside journal
// mode.
func (a *App) restack() {
	j := a.journal
	if j == nil {
		return
	}
	var top *whynot.View
	if j.stack != nil {
		top = j.stack.views[j.stack.top]
	}
	var views []*whynot.View
	j.pages = nil
	add := func(v *whynot.View, location *url.URL, first int) {
		if n := len(views); n > 0 && views[n-1] == v {
			// An in-page jump: the same page, now at location.
			j.pages[n-1].location = location
			return
		}
		views = append(views, v)
		j.pages = append(j.pages, trailPage{location: location, first: first})
	}
	for i, e := range a.history {
		add(e.view, e.location, i)
	}
	add(a.Panel.View(), a.location, -1)
	j.pages[len(j.pages)-1].first = -1
	// Earlier pages without their controls; the current one, whole.
	for i := range len(views) - 1 {
		views[i] = a.trailView(views[i])
	}

	background, separator := a.stackColors()
	for _, v := range views {
		a.prepare(v)
	}
	j.stack = NewViewStack(background, separator, views...)
	j.stack.top = len(views) - 1
	cutTop := -1
	if i := slices.Index(views, top); i >= 0 {
		j.stack.top = i
	} else if top != nil {
		// The page at the top has lost its controls, joining the trail:
		// its end, at the bottom, is where the reader was.
		cutTop = slices.IndexFunc(views, func(v *whynot.View) bool { return v != top && a.fullView(v) == top })
	}
	j.stack.SetBounds(a.bounds)
	if cutTop >= 0 {
		j.stack.top = cutTop
		views[cutTop].ScrollToEnd()
		j.stack.clampEnd()
	}
	j.stack.SetScrollbar(a.stackScrollbar())
	j.controller = NewStackController(j.stack)
	// It knows where the pointer is, without waiting for it to move.
	j.controller.pointer, j.controller.hasPointer = a.pointer, a.hasPointer
}

// SetScrollbar sets whether what's shown draws its own scrollbar: the
// current page's View, or in journal mode the stack of pages. A program
// drawing a scrollbar of its own leaves it off (see JournalStack).
func (a *App) SetScrollbar(on bool) {
	a.scrollbar = on
	a.Panel.SetScrollbar(on)
	if j := a.journal; j != nil {
		j.stack.SetScrollbar(a.stackScrollbar())
	}
}

// stackScrollbar returns the style of the journal stack's own scrollbar:
// the current theme's, or nil if it's off.
func (a *App) stackScrollbar() *simpletheme.Scrollbar {
	if !a.scrollbar {
		return nil
	}
	theme := simpletheme.Light()
	if a.darkTheme {
		theme = simpletheme.Dark()
	}
	return &theme.Scrollbar
}

// JournalStack returns the stack of pages shown in journal mode, or nil
// outside it: for a program drawing its own scrollbar for it (see its
// VisibleRange and ScrollToRatio).
func (a *App) JournalStack() *ViewStack {
	if j := a.stacked(); j != nil {
		return j.stack
	}
	return nil
}

// prepare gives v, a page of the journal, the current scale, zoom and
// StyleSheet, without its own scrollbar.
func (a *App) prepare(v *whynot.View) {
	v.SetStyleSheet(a.styleSheet)
	v.SetScale(a.deviceScale)
	v.SetZoom(a.zoom)
	v.SetScrollbar(false)
}

// stackColors returns the colors of the current theme the journal's
// stack draws with itself.
func (a *App) stackColors() (background, separator color.Color) {
	theme := simpletheme.Light()
	if a.darkTheme {
		theme = simpletheme.Dark()
	}
	return theme.BackgroundColor, theme.ThematicBreakColor
}

// Frame applies a frame's input, at now, to what's shown: the current
// page, or in journal mode the stack, and follows the links clicked (see
// HandleEvents).
func (a *App) Frame(events []input.Event, now time.Duration) {
	a.noteInput(events, now)
	j := a.stacked()
	if j == nil {
		a.HandleEvents(a.Panel.Frame(events, now))
		return
	}
	for _, e := range j.controller.Frame(events, now) {
		if e.Child < 0 || e.Child == len(j.pages)-1 {
			a.HandleEvents([]whynot.Event{e.Event})
			continue
		}
		switch ev := e.Event.(type) {
		case whynot.LinkClick:
			a.followFrom(e.Child, ev.Destination)
		case whynot.AnchorClick:
			if a.pageOf(j.stack.views[e.Child].Document()) != nil {
				// A passage, on a page of a whymark document.
				a.followFrom(e.Child, "#"+url.PathEscape(ev.ID))
				break
			}
			a.arrival = arrival{}
			j.stack.scrollToFragment(e.Child, ev.ID)
		}
		if a.journal != j {
			return // the trail changed: the rest is stale
		}
	}
}

// followFrom follows dest, a link in page i of the trail, an earlier one
// than the current page: history goes back to that page, and on to the
// new one. A link to that page itself scrolls the stack there.
func (a *App) followFrom(i int, dest string) {
	page := a.journal.pages[i]
	if req, ok := a.whymarkLink(a.journal.stack.views[i].Document(), dest); ok {
		// A turn from that page of a whymark document.
		doc, err := a.takeTurn(req)
		if err != nil {
			log.Printf("taking a turn of %s: %v", req.Location, err)
			return
		}
		a.branchFrom(i, doc, req.Location)
		return
	}
	resolved, err := resolveAgainst(page.location, dest)
	if err != nil {
		log.Printf("link destination %q: %v", dest, err)
		return
	}
	if samePage(page.location, resolved) && a.pageOf(a.journal.stack.views[i].Document()) == nil {
		a.arrival = arrival{}
		a.journal.stack.scrollToFragment(i, resolved.Fragment)
		return
	}
	doc, err := a.Load(resolved)
	if err != nil {
		if !openIfWebPage(err, resolved) {
			log.Printf("loading %s: %v", resolved, err)
		}
		return
	}
	a.branchFrom(i, doc, resolved)
}

// branchFrom shows doc, loaded from location by a link in page i of the
// trail, as if the pages after page i hadn't been visited.
func (a *App) branchFrom(i int, doc *whynot.Document, location *url.URL) {
	page := a.journal.pages[i]
	view := a.fullView(a.journal.stack.views[i])
	a.history, a.future = a.history[:page.first], nil
	a.Panel.SetView(view)
	a.location = page.location
	a.show(doc, location)
}

// Draw draws what's shown, at now: the current page, or in journal mode
// the stack.
func (a *App) Draw(dst canvas.Canvas, now time.Duration) {
	if j := a.stacked(); j != nil {
		j.stack.Draw(dst, now)
	} else {
		a.Panel.Draw(dst, now)
	}
	a.drawWaiting(dst, now)
}

// Animating reports whether frames must keep coming even without input,
// as for a fling or a scrollbar fading out, or while the current page is
// still arriving, for the waiting dots.
func (a *App) Animating() bool {
	if a.overlaid == nil && !a.Panel.View().Document().Complete() {
		return true
	}
	if j := a.stacked(); j != nil {
		return j.controller.Animating()
	}
	return a.Panel.Animating()
}

// ViewAt returns the View shown at (x, y): the current page's, or in
// journal mode, the page's there, if any.
func (a *App) ViewAt(x, y int) (*whynot.View, bool) {
	j := a.stacked()
	if j == nil {
		return a.Panel.View(), image.Pt(x, y).In(a.Panel.Bounds())
	}
	if i := j.controller.childAt(image.Pt(x, y)); i >= 0 {
		return j.stack.views[i], true
	}
	return nil, false
}

// ScrollUp, ScrollDown, PageUp and PageDown scroll what's shown as a
// Panel's methods do, as the reader asked: the end of a document still
// arriving stops being followed.
func (a *App) ScrollUp()   { a.scrollBy(-commandStep*a.deviceScale, a.Panel.ScrollUp) }
func (a *App) ScrollDown() { a.scrollBy(commandStep*a.deviceScale, a.Panel.ScrollDown) }
func (a *App) PageUp()     { a.scrollBy(-a.page(), a.Panel.PageUp) }
func (a *App) PageDown()   { a.scrollBy(a.page(), a.Panel.PageDown) }

// page is how far PageUp and PageDown move.
func (a *App) page() float64 {
	return float64(a.bounds.Dy()) * (1 - pageOverlap)
}

// scrollBy scrolls the journal's stack by dy, or calls alone outside
// journal mode.
func (a *App) scrollBy(dy float64, alone func()) {
	a.arrival = arrival{}
	if j := a.stacked(); j != nil {
		j.stack.ScrollBy(dy)
		return
	}
	alone()
}

// ScrollLeft and ScrollRight scroll a block wider than the page, such as
// a code block, as a Panel's do.
func (a *App) ScrollLeft() {
	if j := a.stacked(); j != nil {
		j.controller.ScrollLeft()
		return
	}
	a.Panel.ScrollLeft()
}

func (a *App) ScrollRight() {
	if j := a.stacked(); j != nil {
		j.controller.ScrollRight()
		return
	}
	a.Panel.ScrollRight()
}
