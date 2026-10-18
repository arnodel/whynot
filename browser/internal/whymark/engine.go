package whymark

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

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
	registry     *fetch.Registry
	capabilities map[string]Capability
	grant        func(document *url.URL, name string) bool
	clock        func() time.Time
	store        Store

	mu       sync.Mutex
	pages    map[string]*snapshot
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
	// time is when the page was made (turn.time).
	time time.Time
	// output is the page as its turn wrote it, header and all, so that
	// it can be shown again (see Engine.Saved).
	output bytes.Buffer
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
// NewEngine returns an Engine fetching documents, and their Lua modules,
// from registry.
func NewEngine(registry *fetch.Registry, options ...Option) *Engine {
	e := &Engine{registry: registry, capabilities: map[string]Capability{}, clock: time.Now}
	for _, o := range options {
		o(e)
	}
	return e
}

// An Option configures an Engine made by NewEngine.
type Option func(*Engine)

// WithCapabilities gives the engine capabilities to provide to the
// documents declaring them. It panics if two have the same name.
func WithCapabilities(capabilities ...Capability) Option {
	return func(e *Engine) {
		for _, c := range capabilities {
			name := c.Name()
			if e.capabilities[name] != nil {
				panic(fmt.Sprintf("whymark: two capabilities named %q", name))
			}
			e.capabilities[name] = c
		}
	}
}

// WithClock makes the engine tell the time with clock, rather than
// time.Now: for tests, or for an engine replaying pages.
func WithClock(clock func() time.Time) Option {
	return func(e *Engine) { e.clock = clock }
}

// WithStore makes the engine keep its pages in store as well as in
// memory, so that pages made by an earlier run of the program can be
// continued from.
func WithStore(store Store) Option {
	return func(e *Engine) { e.store = store }
}

// WithGrant makes the engine ask grant whether the document at a URL may
// use the capability name, as the reader decides. Without it, every
// document may use every capability it declares that the engine has.
func WithGrant(grant func(document *url.URL, name string) bool) Option {
	return func(e *Engine) { e.grant = grant }
}

// An error means there's no page at all: the document can't be fetched,
// isn't a whymark document, or req.FromPageID isn't one of its pages.
func (e *Engine) Turn(ctx context.Context, req Request) (io.ReadCloser, error) {
	document := documentURL(req.Location)
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
	// What's written is also kept, for the page to be shown again.
	out := io.MultiWriter(pw, &snap.output)
	p := &page{
		w:       out,
		header:  Header{Location: req.Location, PageID: id, Journal: c.settings.Journal, Resume: c.settings.Resume},
		auto:    c.err != nil || len(c.program.Passages) > 0,
		program: c.program,
	}
	go func() {
		defer pw.Close()
		// Until the turn is over, the page continues from where it
		// started; a failed turn leaves it so.
		snap.state, snap.passage = state, passage
		defer func() {
			e.save(id, snap)
			close(snap.done)
		}()
		err := c.err
		if err == nil {
			// golua can't be interrupted: a turn whose page is closed carries
			// on, within its CPU limit, but fails at its next write.
			var out turnOutput
			out, err = run(c.program, p, turnInput{
				state: state, start: start, passage: passage, query: query, input: req.Input,
				previous: previous, pages: e.parent, seed: seed(id), time: snap.time,
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
		fmt.Fprintf(out, "\n\n---\n\n**The page stopped:**\n\n```text\n%s\n```\n", strings.TrimPrefix(err.Error(), "error: "))
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
// Engine.granted), or, if c requires one it can't have, c compiled
// to that error, for its page to show.
func (e *Engine) withCapabilities(c *compiled, document *url.URL) (map[string]Capability, *compiled) {
	granted, err := e.granted(document, c.program.Uses)
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
	src, err := e.registry.Resolve(location, "")
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
	id := newPageID()
	snap := &snapshot{done: make(chan struct{}), document: document, time: e.clock()}
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
	p, _ := e.lookup(s.parent)
	return p
}

// lookup returns the snapshot behind the page id: in memory, or else
// from the store, if the engine has one and it has the page.
func (e *Engine) lookup(id string) (*snapshot, error) {
	e.mu.Lock()
	snap, ok := e.pages[id]
	e.mu.Unlock()
	if ok || e.store == nil {
		return snap, nil
	}
	saved, ok, err := e.store.LoadPage(id)
	if err != nil || !ok {
		return nil, err
	}
	if snap, err = restored(saved); err != nil {
		return nil, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pages == nil {
		e.pages = map[string]*snapshot{}
	}
	e.pages[id] = snap
	return snap, nil
}

// save saves the page id to the engine's store, if it has one, once its
// turn is over.
func (e *Engine) save(id string, snap *snapshot) {
	if e.store == nil {
		return
	}
	if err := e.store.SavePage(id, snap.saved()); err != nil {
		log.Printf("whymark: saving page %s: %v", id, err)
	}
}

// newPageID returns a new page id: random, so that pages made by different
// runs of the program, kept in a Store, don't clash.
func newPageID() string {
	var b [10]byte
	rand.Read(b[:])
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:]))
}

// Visit records that the page id was visited now, as when a user agent
// shows it: in the engine's Store, if it records visits (see
// PageVisitor). A page's creation is its first visit. It can be slow, as
// it writes to the Store.
func (e *Engine) Visit(id string) error {
	if v, ok := e.store.(PageVisitor); ok {
		return v.Visit(id, e.clock())
	}
	return nil
}

// DocumentOf returns the name a document's pages are kept under (see
// Engine.Pages): its location, with no query or fragment.
func DocumentOf(location *url.URL) string {
	u := documentURL(location)
	return u.String()
}

func documentURL(location *url.URL) url.URL {
	u := *location
	u.RawQuery, u.ForceQuery, u.Fragment, u.RawFragment = "", false, "", ""
	return u
}

// Pages lists the pages of document (see DocumentOf), by
// when they were made: those in the engine's Store, if it can list them
// (see PageLister), and those this engine made that it hasn't saved, once
// their turn is over.
func (e *Engine) Pages(document string) ([]PageInfo, error) {
	var pages []PageInfo
	seen := map[string]bool{}
	if lister, ok := e.store.(PageLister); ok {
		stored, err := lister.Pages(document)
		if err != nil {
			return nil, err
		}
		for _, p := range stored {
			seen[p.ID] = true
		}
		pages = stored
	}
	e.mu.Lock()
	for id, snap := range e.pages {
		select {
		case <-snap.done:
		default:
			continue // its turn is still setting it
		}
		if snap.document == document && !seen[id] {
			pages = append(pages, PageInfo{ID: id, Parent: snap.parent, Passage: snap.passage, Time: snap.time})
		}
	}
	e.mu.Unlock()
	sort.SliceStable(pages, func(i, j int) bool { return pages[i].Time.Before(pages[j].Time) })
	return pages, nil
}

// ErrNoOutput is Saved's error for a page whose output wasn't kept, such
// as one saved by an earlier version: it can still be continued from.
var ErrNoOutput = errors.New("the page's output wasn't kept")

// Saved returns the page id as its turn wrote it, header and all, as Turn
// returns pages, and the request it answered: to show it again, rather
// than make a new page from it. It waits for the page's turn to be over.
func (e *Engine) Saved(ctx context.Context, id string) (io.ReadCloser, Request, error) {
	snap, err := e.page(ctx, id)
	if err != nil {
		return nil, Request{}, err
	}
	if snap.output.Len() == 0 {
		return nil, Request{}, ErrNoOutput
	}
	return io.NopCloser(bytes.NewReader(snap.output.Bytes())), snap.request, nil
}

// page returns the snapshot behind the page id, once its turn is over.
func (e *Engine) page(ctx context.Context, id string) (*snapshot, error) {
	snap, err := e.lookup(id)
	if err != nil {
		return nil, err
	}
	if snap == nil {
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
