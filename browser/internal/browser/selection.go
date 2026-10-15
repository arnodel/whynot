package browser

import (
	"log"

	"github.com/arnodel/whynot"
)

// shownViews returns the Views showing: the current page's, or in journal
// mode the trail's.
func (a *App) shownViews() []*whynot.View {
	if j := a.stacked(); j != nil {
		return j.stack.views
	}
	if a.overlaid != nil {
		// The document under an overlay, such as the actions page.
		return []*whynot.View{a.overlaid}
	}
	return []*whynot.View{a.Panel.View()}
}

// selectedView returns the shown View with a selection, or nil: in
// journal mode, selecting in one page clears the others (see
// keepOneSelection).
func (a *App) selectedView() *whynot.View {
	for _, v := range a.shownViews() {
		if v.HasSelection() {
			return v
		}
	}
	return nil
}

// HasSelection reports whether text is selected.
func (a *App) HasSelection() bool { return a.selectedView() != nil }

// CopySelection puts the selected text on the clipboard, as Markdown, if
// there is some. Whether it worked shows on the actions page, as for
// CopyMarkdown.
func (a *App) CopySelection() {
	v := a.selectedView()
	if v == nil {
		return
	}
	a.copied, a.copyErr = true, clipboardWriter(v.SelectedMarkdown())
	if a.copyErr != nil {
		log.Printf("copying the selection: %v", a.copyErr)
	}
}

// SelectAll selects the whole current page.
func (a *App) SelectAll() {
	if a.overlaid != nil {
		return
	}
	if j := a.stacked(); j != nil {
		current := j.stack.views[len(j.stack.views)-1]
		a.keepOneSelection(current)
		current.SelectAll()
		return
	}
	a.Panel.View().SelectAll()
}

// keepOneSelection clears the selection of the shown Views but v: in
// journal mode, there's one selection at a time.
func (a *App) keepOneSelection(v *whynot.View) {
	for _, other := range a.shownViews() {
		if other != v {
			other.ClearSelection()
		}
	}
}

// Cursor returns what the pointer should look like where it is: over a
// link, over text, or elsewhere. The app shows it with its toolkit's
// cursors.
func (a *App) Cursor() whynot.Cursor {
	if a.Panel == nil {
		return whynot.CursorDefault
	}
	cursor := whynot.CursorDefault
	for _, v := range a.shownViews() {
		cursor = max(cursor, v.Cursor())
	}
	return cursor
}
