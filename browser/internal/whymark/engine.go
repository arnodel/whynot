package whymark

import (
	"context"
	"crypto/sha256"
	"fmt"
	"hash/fnv"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/arnodel/golua/code"
	rt "github.com/arnodel/golua/runtime"

	"github.com/arnodel/whynot/browser/internal/whymark/compile"
	"github.com/arnodel/whynot/fetch"
)

// An Engine takes turns of whymark documents: given a request, it runs
// the document and writes the page. It keeps a snapshot behind every page
// it gives out, in memory: the state, and the passage the page shows.
// It's safe for concurrent use.
type Engine struct {
	// Registry is what documents, and their Lua modules, are fetched
	// from.
	Registry *fetch.Registry
	// Capabilities are what the engine can provide to documents declaring
	// them, by name, such as "ai".
	Capabilities map[string]Capability
	// Grant, if set, reports whether the document at a URL may use the
	// capability name, as the reader decides; without it, every document
	// may use every capability it declares.
	Grant func(document *url.URL, name string) bool

	mu       sync.Mutex
	pages    map[string]*snapshot
	lastPage int
	compiled map[[sha256.Size]byte]*compiled
	modules  map[[sha256.Size]byte]*code.Unit
}

// snapshot is what's behind a page-id.
type snapshot struct {
	// done is closed once the page's turn is over and the rest is set.
	done     chan struct{}
	document string // the document's URL, with no query or fragment
	passage  string
	state    any
	// notes is what the page's turn recorded of itself (turn.notes).
	notes any
	// request is the request the page answered, and parent the page it
	// came from (its FromPageID), "" for a document's start: together,
	// pages make a tree of what a reader did.
	request Request
	parent  string
}

// compiled is a document compiled, cached by its content: its program,
// or why it has none, which its pages show.
type compiled struct {
	program  *compile.Program
	settings compile.Settings
	err      error
}

// Turn takes a turn: it runs the document at req.Location, from the page
// req.FromPageID if set, and returns the page as it's written, its header
// first (see ReadHeader), then its Markdown. Closing the page, or
// cancelling ctx, stops the turn.
//
// A document that fails, at compiling or running, still gives a page: one
// with an error message, whose page-id keeps the state it started from.
// An error means there's no page at all: the document can't be fetched,
// isn't a whymark document, or req.FromPageID isn't one of its pages.
func (e *Engine) Turn(ctx context.Context, req Request) (io.ReadCloser, error) {
	document := *req.Location
	document.RawQuery, document.ForceQuery, document.Fragment, document.RawFragment = "", false, "", ""
	source, err := e.fetch(ctx, &document)
	if err != nil {
		return nil, err
	}
	c, err := e.compile(document.String(), source)
	if err != nil {
		return nil, err
	}

	// Where the turn starts from: a page's snapshot, or the document's
	// start.
	start := req.FromPageID == ""
	var state any = &table{}
	passage := ""
	var previous *snapshot
	if !start {
		from, err := e.page(ctx, req.FromPageID)
		if err != nil {
			return nil, err
		}
		if from.document != document.String() {
			return nil, fmt.Errorf("page %s isn't one of %s's", req.FromPageID, &document)
		}
		state, passage = from.state, from.passage
		previous = from
	}
	if req.Location.Fragment != "" {
		passage = req.Location.Fragment
	}
	if passage == "" && c.err == nil {
		passage = c.program.StartPassage()
	}
	query := map[string]string{}
	for k, vs := range req.Location.Query() {
		query[k] = vs[0]
	}

	id, snap := e.newPage(document.String())
	snap.request, snap.parent = req, req.FromPageID
	var granted map[string]Capability
	if c.err == nil {
		granted, c = e.withCapabilities(c, &document)
	}
	ctx, cancel := context.WithCancel(ctx)
	pr, pw := io.Pipe()
	p := &page{
		w:       pw,
		header:  Header{Location: req.Location, PageID: id, Journal: c.settings.Journal},
		auto:    c.err != nil || len(c.program.Passages) > 0,
		program: c.program,
	}
	go func() {
		defer pw.Close()
		// Until the turn is over, the page continues from where it
		// started; a failed turn leaves it so.
		snap.state, snap.passage = state, passage
		defer close(snap.done)
		err := c.err
		if err == nil {
			// golua can't be interrupted: a turn whose page is closed carries
			// on, within its CPU limit, but fails at its next write.
			var out turnOutput
			out, err = run(c.program, p, turnInput{
				state: state, start: start, passage: passage, query: query, input: req.Input,
				previous: previous, pages: e.parent, seed: seed(id),
				engine: e, granted: granted,
				context: &TurnContext{Context: ctx, Document: &document, PageID: id, Request: req},
			})
			if err == nil {
				snap.state, snap.passage, snap.notes = out.state, out.passage, out.notes
			}
		}
		if err == nil || ctx.Err() != nil {
			if err == nil && !p.sent {
				p.send(snap.passage, "")
			}
			return
		}
		if !p.sent {
			p.send(passage, "")
		}
		fmt.Fprintf(pw, "\n\n---\n\n**The page stopped:**\n\n```text\n%s\n```\n", strings.TrimPrefix(err.Error(), "error: "))
	}()
	return cancelOnClose{ReadCloser: pr, cancel: cancel}, nil
}

// seed returns a seed for math.random from the id of the page a turn
// makes: what happens during a page, its rolls included, belongs to it,
// so a new page rolls anew, and a page made again rolls the same.
func seed(pageID string) int64 {
	h := fnv.New64a()
	io.WriteString(h, pageID)
	return int64(h.Sum64())
}

// withCapabilities returns the capabilities a turn of c gets (see
// Engine.capabilities), or, if c requires one it can't have, c compiled
// to that error, for its page to show.
func (e *Engine) withCapabilities(c *compiled, document *url.URL) (map[string]Capability, *compiled) {
	granted, err := e.capabilities(document, c.program.Uses)
	if err != nil {
		return nil, &compiled{program: c.program, settings: c.settings, err: err}
	}
	return granted, c
}

// compileModule returns the Lua module source, named name, compiled by
// golua, from the cache if its content has been seen.
func (e *Engine) compileModule(name string, source []byte) (*code.Unit, error) {
	key := sha256.Sum256(source)
	e.mu.Lock()
	defer e.mu.Unlock()
	if u, ok := e.modules[key]; ok {
		return u, nil
	}
	u, _, err := rt.New(io.Discard).CompileLuaChunk(name, source)
	if err != nil {
		return nil, err
	}
	if e.modules == nil {
		e.modules = map[[sha256.Size]byte]*code.Unit{}
	}
	e.modules[key] = u
	return u, nil
}

// fetch returns the source of the document at location, which must be a
// whymark document.
func (e *Engine) fetch(ctx context.Context, location *url.URL) ([]byte, error) {
	src, err := e.Registry.Resolve(location, "")
	if err != nil {
		return nil, err
	}
	body, _, err := src.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	source, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	if !compile.IsDocument(source) {
		return nil, fmt.Errorf("%s isn't a whymark document: its front matter has no whymark key", location)
	}
	return source, nil
}

// compile returns source, the document name, compiled, from the cache if
// it's been seen. A document that doesn't compile is compiled to its
// error, which its pages show; it's an error only if its front matter
// can't be read.
func (e *Engine) compile(name string, source []byte) (*compiled, error) {
	key := sha256.Sum256(source)
	e.mu.Lock()
	defer e.mu.Unlock()
	if c, ok := e.compiled[key]; ok {
		return c, nil
	}
	settings, err := compile.ReadSettings(source)
	if err != nil {
		return nil, err
	}
	c := &compiled{settings: settings}
	c.program, c.err = compile.Compile(name, source)
	if c.program == nil {
		// For the error page's header.
		c.program = &compile.Program{Settings: settings}
	}
	if e.compiled == nil {
		e.compiled = map[[sha256.Size]byte]*compiled{}
	}
	e.compiled[key] = c
	return c, nil
}

// newPage returns a new page's id and snapshot, for a page of document.
func (e *Engine) newPage(document string) (string, *snapshot) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastPage++
	id := strconv.FormatInt(int64(e.lastPage), 36)
	snap := &snapshot{done: make(chan struct{}), document: document}
	if e.pages == nil {
		e.pages = map[string]*snapshot{}
	}
	e.pages[id] = snap
	return id, snap
}

// parent returns the snapshot of the page s came from, or nil if s
// started its document. Its turn is over, as it was before s's began.
func (e *Engine) parent(s *snapshot) *snapshot {
	if s.parent == "" {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pages[s.parent]
}

// page returns the snapshot behind the page id, once its turn is over.
func (e *Engine) page(ctx context.Context, id string) (*snapshot, error) {
	e.mu.Lock()
	snap, ok := e.pages[id]
	e.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no page %q", id)
	}
	select {
	case <-snap.done:
		return snap, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// cancelOnClose is a page whose closing cancels its turn.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c cancelOnClose) Close() error {
	c.cancel()
	return c.ReadCloser.Close()
}
