package browser

import (
	"encoding/json"
	"errors"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/arnodel/whynot/browser/internal/whymark"
)

// A whymark document's state lives in its pages. With persistence on, the
// engine keeps them on disk as well as in memory, so that a page made in an
// earlier session can be carried on from; and the browser remembers the
// latest page of each document that asks to be resumed (its header's
// resume hint), and carries on from it when the document is opened again.

// EnablePersistence keeps whymark documents' pages in dir, and the latest
// page of each document asking to be resumed. Call it before the first
// document is opened.
func (a *App) EnablePersistence(dir string) error {
	a.latestFile = filepath.Join(dir, "latest.json")
	a.latest = map[string]string{}
	data, err := os.ReadFile(a.latestFile)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(data, &a.latest); err != nil {
			return err
		}
	}
	a.engineWith(whymark.WithStore(whymark.FileStore{Dir: filepath.Join(dir, "pages")}))
	return nil
}

// engineWith makes the engine anew with options, on top of those it had.
func (a *App) engineWith(options ...whymark.Option) {
	a.engineOptions = append(a.engineOptions, options...)
	a.engine = whymark.NewEngine(a.registry, a.engineOptions...)
}

// documentKey returns the URL a document is remembered by: location,
// without its query or fragment.
func documentKey(location *url.URL) string {
	u := *location
	u.RawQuery, u.ForceQuery, u.Fragment, u.RawFragment = "", false, "", ""
	return u.String()
}

// latestPage returns the latest page of the document at location, if it
// asks to be resumed and location names it alone, with no query or
// passage.
func (a *App) latestPage(location *url.URL) string {
	if location.RawQuery != "" || location.Fragment != "" {
		return ""
	}
	return a.latest[documentKey(location)]
}

// noteLatest notes the page h as its document's latest, if the document
// asks to be resumed and persistence is on.
func (a *App) noteLatest(h whymark.Header) {
	if !h.Resume || a.latestFile == "" {
		return
	}
	a.latest[documentKey(h.Location)] = h.PageID
	data, err := json.MarshalIndent(a.latest, "", "  ")
	if err == nil {
		err = os.WriteFile(a.latestFile, data, 0o600)
	}
	if err != nil {
		log.Printf("remembering the latest page of %s: %v", h.Location, err)
	}
}

// sameDocument reports whether x and y name the same document, whatever
// their queries and fragments.
func sameDocument(x, y *url.URL) bool {
	return documentKey(x) == documentKey(y)
}

// A whymark page has a URL of its own, its page URL: its location, with
// its page-id after its passage, separated by "@", as in
// "todo.md#to-do@k3j9x2m4qp7r5t8w". Opening it carries on from that page,
// in any whynot on the machine keeping it (see EnablePersistence). The
// fragment is the user agent's business, never sent to the engine, and a
// passage's id, a heading's anchor, never has an "@".

// pageURL returns the page URL of the page h.
func pageURL(h whymark.Header) *url.URL {
	u := *h.Location
	u.Fragment, u.RawFragment = u.Fragment+"@"+h.PageID, ""
	return &u
}

// splitPageURL returns, if u is a page URL, its location, without the
// page-id, and the page-id.
func splitPageURL(u *url.URL) (*url.URL, string, bool) {
	passage, id, ok := strings.Cut(u.Fragment, "@")
	if !ok || id == "" {
		return u, "", false
	}
	location := *u
	location.Fragment, location.RawFragment = passage, ""
	return &location, id, true
}
