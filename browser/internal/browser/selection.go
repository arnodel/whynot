package browser

import (
	"log"
	"strings"

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

// selectedViews returns the shown Views with a selection, in order: in
// journal mode, a selection can run across pages.
func (a *App) selectedViews() []*whynot.View {
	var views []*whynot.View
	for _, v := range a.shownViews() {
		if v.HasSelection() {
			views = append(views, v)
		}
	}
	return views
}

// HasSelection reports whether text is selected.
func (a *App) HasSelection() bool { return len(a.selectedViews()) > 0 }

// CopySelection puts the selected text on the clipboard, as Markdown, if
// there is some: across pages of the journal, each page's, separated by
// a thematic break, as CopyMarkdown separates them. Whether it worked
// shows on the actions page.
func (a *App) CopySelection() {
	views := a.selectedViews()
	if len(views) == 0 {
		return
	}
	var parts []string
	for _, v := range views {
		parts = append(parts, v.SelectedMarkdown())
	}
	a.copied, a.copyErr = true, clipboardWriter(strings.Join(parts, "\n\n---\n\n"))
	if a.copyErr != nil {
		log.Printf("copying the selection: %v", a.copyErr)
	}
}

// SelectAll selects the whole current page, or in journal mode, if it's
// already all selected, the whole journal.
func (a *App) SelectAll() {
	if a.overlaid != nil {
		return
	}
	if j := a.stacked(); j != nil {
		current := j.stack.views[len(j.stack.views)-1]
		if current.AllSelected() {
			for _, v := range j.stack.views {
				v.SelectAll()
			}
			return
		}
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
