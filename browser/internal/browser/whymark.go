package browser

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"time"

	"github.com/arnodel/whynot"
	"github.com/arnodel/whynot/browser/internal/whymark"
	"github.com/arnodel/whynot/browser/internal/whymark/compile"
)

// whymarkPage is what the browser keeps of a page of a whymark document:
// its header, and the request it answered, which is its identity.
type whymarkPage struct {
	header  whymark.Header
	request whymark.Request
	// input is the page's input field, as its Markdown arrives.
	input pageInput
	// source is the page's Markdown, as it arrives; cut is a View of the
	// page without its controls, for the journal's trail, standing for
	// full, the page's own View (see trailView).
	source    pageSource
	cut, full *whynot.View
}

// errWhymark means loadDocument found a whymark document, which is shown
// through the engine, one turn at a time, not as Markdown.
var errWhymark = errors.New("a whymark document")

// maxFrontMatter bounds how much of a document's start is read to tell a
// whymark document's front matter.
const maxFrontMatter = 64 << 10

// readHead reads the start of a document from br: its front matter, if it
// starts with some, up to maxFrontMatter. The rest stays in br.
func readHead(br *bufio.Reader) []byte {
	var head []byte
	for len(head) < maxFrontMatter {
		line, err := br.ReadSlice('\n')
		head = append(head, line...)
		if err != nil && err != bufio.ErrBufferFull {
			break
		}
		if err == nil {
			trimmed := strings.TrimRight(string(line), "\r\n")
			if len(head) == len(line) && trimmed != "---" {
				break // no front matter
			}
			if len(head) > len(line) && (trimmed == "---" || trimmed == "...") {
				break // its end
			}
		}
	}
	return head
}

// isWhymark reports whether head, the start of a document (see readHead),
// is a whymark document's.
func isWhymark(head []byte) bool {
	return compile.IsDocument(head)
}

// takeTurn takes a turn of a whymark document, and returns its page as a
// Document, as it arrives: complete if it arrives quickly, otherwise
// growing as the rest comes in, in the background. It waits for the
// page's header, which comes before anything slow.
func (a *App) takeTurn(req whymark.Request) (*whynot.Document, error) {
	r, err := a.engine.Turn(context.Background(), req)
	if err != nil {
		return nil, err
	}
	br := bufio.NewReader(r)
	h, err := whymark.ReadHeader(br)
	if err != nil {
		r.Close()
		return nil, err
	}
	doc, w := a.parser.Stream(whynot.WithBaseURL(h.Location))
	page := &whymarkPage{header: h, request: req}
	filter := &inputFilter{w: w, source: &page.source, input: &page.input}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := io.Copy(filter, br); err != nil {
			log.Printf("taking a turn of %s: %v", h.Location, err)
		}
		filter.Close()
		r.Close()
		w.Close()
	}()
	select {
	case <-done:
	case <-time.After(loadWait):
	}
	if a.pages == nil {
		a.pages = map[*whynot.Document]*whymarkPage{}
	}
	a.pages[doc] = page
	a.watch(doc)
	return doc, nil
}

// pageOf returns what the browser keeps of the whymark page doc is, or
// nil if it's not one.
func (a *App) pageOf(doc *whynot.Document) *whymarkPage {
	return a.pages[doc]
}

// whymarkLink returns the request for dest, a link as written on the
// whymark page doc, if it continues the page's trail (see
// whymark.Follow).
func (a *App) whymarkLink(doc *whynot.Document, dest string) (whymark.Request, bool) {
	p := a.pageOf(doc)
	if p == nil {
		return whymark.Request{}, false
	}
	return whymark.Follow(p.header, dest)
}

// headOf returns the start of data, as readHead would.
func headOf(data []byte) []byte {
	return readHead(bufio.NewReader(bytes.NewReader(data)))
}
