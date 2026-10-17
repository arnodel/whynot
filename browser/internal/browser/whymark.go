package browser

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
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
	// trailFrom is the page before it, for a page opened from elsewhere,
	// such as from its page URL: the journal shows its trail, the pages
	// leading to it, once loaded (see loadTrail); trailLoading says it's
	// been asked for.
	trailFrom    string
	trailLoading bool
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
	return a.pageFrom(r, req, true)
}

// pageFrom returns the page r, as Turn returns pages, answering req, as a
// Document (see takeTurn). latest says it's a new page, its document's
// latest, rather than one made earlier, shown again.
func (a *App) pageFrom(r io.ReadCloser, req whymark.Request, latest bool) (*whynot.Document, error) {
	br := bufio.NewReader(r)
	h, err := whymark.ReadHeader(br)
	if err != nil {
		r.Close()
		return nil, err
	}
	if latest {
		a.noteLatest(h)
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

// startTurn takes the turn req in the background, and returns its page at
// once, empty: taking a turn fetches and compiles the document, and the
// page's header comes with its first output, which can wait for an AI.
// Until the header arrives, the page has only the request's location
// (see addHeaders); a turn that fails says so on the page. latest is as
// for pageFrom.
func (a *App) startTurn(req whymark.Request, latest bool) *whynot.Document {
	doc, w := a.parser.Stream(whynot.WithBaseURL(req.Location))
	page := &whymarkPage{header: whymark.Header{Location: req.Location}, request: req}
	filter := &inputFilter{w: w, source: &page.source, input: &page.input}
	if a.pages == nil {
		a.pages = map[*whynot.Document]*whymarkPage{}
	}
	a.pages[doc] = page
	a.watch(doc)
	notify := a.OnDocumentChange
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer w.Close()
		defer filter.Close()
		r, err := a.engine.Turn(context.Background(), req)
		if err != nil {
			log.Printf("taking a turn of %s: %v", req.Location, err)
			fmt.Fprintf(filter, "The page couldn't be made: %v\n", err)
			return
		}
		defer r.Close()
		br := bufio.NewReader(r)
		h, err := whymark.ReadHeader(br)
		if err != nil {
			log.Printf("taking a turn of %s: %v", req.Location, err)
			fmt.Fprintf(filter, "The page couldn't be made: %v\n", err)
			return
		}
		a.headersMu.Lock()
		a.headers = append(a.headers, arrivedHeader{doc: doc, header: h, latest: latest})
		a.headersMu.Unlock()
		if notify != nil {
			notify()
		}
		if _, err := io.Copy(filter, br); err != nil {
			log.Printf("taking a turn of %s: %v", h.Location, err)
		}
	}()
	if a.syncTurns {
		<-done
		a.addHeaders()
	}
	return doc
}

// An arrivedHeader is a page's header, arrived in the background (see
// startTurn), for the next Update to apply.
type arrivedHeader struct {
	doc    *whynot.Document
	header whymark.Header
	latest bool
}

// addHeaders applies the headers that arrived since it last ran: the
// page's location, its title and its journal hint, if it's current, and
// its location in history, if it was left meanwhile.
func (a *App) addHeaders() {
	a.headersMu.Lock()
	headers := a.headers
	a.headers = nil
	a.headersMu.Unlock()
	for _, arrived := range headers {
		p := a.pages[arrived.doc]
		if p == nil {
			continue
		}
		p.header = arrived.header
		if arrived.latest {
			a.noteLatest(p.header)
		}
		location := pageURL(p.header)
		for i := range a.history {
			if a.history[i].view.Document() == arrived.doc {
				a.history[i].location = location
			}
		}
		// No Panel yet at launch: Open then reads the header.
		if a.Panel != nil && a.overlaid == nil && a.Panel.View().Document() == arrived.doc {
			a.location = location
			a.journalHint(arrived.doc)
			a.restack() // the journal's pages, with their locations
		}
	}
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
