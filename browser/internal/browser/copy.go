package browser

import (
	"bytes"
	"log"
	"strings"

	"github.com/arnodel/whynot"
)

// clipboardWriter puts text on the clipboard; tests replace it.
var clipboardWriter = writeClipboard

// CopyMarkdown puts the Markdown of what's showing on the clipboard: in
// journal mode, the trail's pages, oldest first; otherwise, the current
// document. A whymark page is copied without its controls, which only make
// sense while it's current. Whether it worked shows on the actions page.
func (a *App) CopyMarkdown() {
	var parts []string
	for _, doc := range a.copyDocuments() {
		if md := bytes.TrimSpace(a.markdownOf(doc)); len(md) > 0 {
			parts = append(parts, string(md))
		}
	}
	a.copied, a.copyErr = true, clipboardWriter(strings.Join(parts, "\n\n---\n\n")+"\n")
	if a.copyErr != nil {
		log.Printf("copying as Markdown: %v", a.copyErr)
	}
}

// CopyLink puts the current page's URL on the clipboard: for a whymark
// page, its page URL, which any whynot on the machine can carry on from.
// Whether it worked shows on the actions page, as for CopyMarkdown.
func (a *App) CopyLink() {
	a.copied, a.copyErr = true, clipboardWriter(a.location.String())
	if a.copyErr != nil {
		log.Printf("copying the link: %v", a.copyErr)
	}
}

// copyDocuments returns the documents CopyMarkdown copies, oldest first.
func (a *App) copyDocuments() []*whynot.Document {
	current := a.Panel.View()
	if a.overlaid != nil {
		current = a.overlaid
	}
	if a.journal == nil {
		return []*whynot.Document{current.Document()}
	}
	var docs []*whynot.Document
	add := func(v *whynot.View) {
		// The same View twice in a row is an in-page jump.
		if doc := v.Document(); len(docs) == 0 || docs[len(docs)-1] != doc {
			docs = append(docs, doc)
		}
	}
	for _, e := range a.history {
		add(e.view)
	}
	add(current)
	return docs
}

// markdownOf returns doc's Markdown, as it has arrived: a whymark page's
// without its controls, and a document's without its front matter. It's
// nil for a document whose source isn't kept, such as the actions page.
func (a *App) markdownOf(doc *whynot.Document) []byte {
	if p := a.pageOf(doc); p != nil {
		source := p.source.bytes()
		if start := controlsStart(source); start >= 0 {
			source = source[:start]
		}
		return source
	}
	if s := a.sources[doc]; s != nil {
		return withoutFrontMatter(s.bytes())
	}
	return nil
}

// withoutFrontMatter returns source without its front matter, if it starts
// with some.
func withoutFrontMatter(source []byte) []byte {
	head := headOf(source)
	first, _, _ := bytes.Cut(head, []byte("\n"))
	if string(bytes.TrimRight(first, "\r")) != "---" || len(head) == len(first) {
		return source
	}
	return source[len(head):]
}
