package whymark

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// A Store keeps pages beyond an Engine's memory, so that a page made in an
// earlier run of the program can still be continued from (see WithStore):
// a document's state lives in its pages.
type Store interface {
	// SavePage saves the page id, once its turn is over.
	SavePage(id string, p SavedPage) error
	// LoadPage returns the page id, and whether there's one.
	LoadPage(id string) (SavedPage, bool, error)
}

// SavedPage is what's kept of a page: what the turns made from it, and
// pages after it, read (see snapshot).
type SavedPage struct {
	Document string            `json:"document"`
	Passage  string            `json:"passage,omitempty"`
	State    any               `json:"state"`
	Notes    any               `json:"notes,omitempty"`
	Location string            `json:"location"`
	Parent   string            `json:"parent,omitempty"`
	Input    map[string]string `json:"input,omitempty"`
	Time     time.Time         `json:"time"`
	// Title is the page's title, from its header.
	Title string `json:"title,omitempty"`
	// Output is the page as its turn wrote it (see Engine.Saved).
	Output string `json:"output,omitempty"`
}

// A PageLister is a Store that can list a document's pages: what a view of
// the tree of pages needs.
type PageLister interface {
	// Pages lists the pages of document, its URL with no query or
	// fragment, by when they were made.
	Pages(document string) ([]PageInfo, error)
}

// PageInfo is what a list of pages says of each: enough to place it in the
// tree of pages, and label it.
type PageInfo struct {
	ID, Parent     string // Parent is "" for a document's start
	Passage, Title string
	Time           time.Time
}

// titleOf returns the title in the header of output, a page as its turn
// wrote it, or "".
func titleOf(output string) string {
	if output == "" {
		return ""
	}
	h, err := ReadHeader(bufio.NewReader(strings.NewReader(output)))
	if err != nil {
		return ""
	}
	return h.Title
}

// FileStore is a Store keeping each page in a file of its own, named by
// its id, in a directory.
type FileStore struct {
	Dir string
}

// pageID matches the ids the engine makes.
var pageID = regexp.MustCompile(`^[a-z0-9]{1,64}$`)

func (s FileStore) path(id string) (string, error) {
	if !pageID.MatchString(id) {
		return "", fmt.Errorf("not a page id: %q", id)
	}
	return filepath.Join(s.Dir, id+".json"), nil
}

func (s FileStore) SavePage(id string, p SavedPage) error {
	path, err := s.path(id)
	if err != nil {
		return err
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	// Written whole, then renamed, so that a page is never half there.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s FileStore) LoadPage(id string) (SavedPage, bool, error) {
	var p SavedPage
	path, err := s.path(id)
	if err != nil {
		return p, false, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return p, false, nil
	}
	if err != nil {
		return p, false, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // integers stay integers (see decodeValue)
	return p, true, dec.Decode(&p)
}

// saved returns what's kept of the page snap.
func (snap *snapshot) saved() SavedPage {
	return SavedPage{
		Document: snap.document,
		Passage:  snap.passage,
		State:    encodeValue(snap.state),
		Notes:    encodeValue(snap.notes),
		Location: snap.request.Location.String(),
		Parent:   snap.parent,
		Input:    snap.request.Input,
		Time:     snap.time,
		Title:    titleOf(snap.output.String()),
		Output:   snap.output.String(),
	}
}

// restored returns the snapshot of a saved page, its turn over.
func restored(p SavedPage) (*snapshot, error) {
	location, err := url.Parse(p.Location)
	if err != nil {
		return nil, err
	}
	state, err := decodeValue(p.State)
	if err != nil {
		return nil, err
	}
	notes, err := decodeValue(p.Notes)
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	close(done)
	snap := &snapshot{
		done: done, document: p.Document, passage: p.Passage, state: state, notes: notes,
		request: Request{Location: location, FromPageID: p.Parent, Input: p.Input},
		parent:  p.Parent, time: p.Time,
	}
	snap.output.WriteString(p.Output)
	return snap, nil
}

// encodeValue turns plain data made by fromLua into JSON that keeps its
// types, which JSON alone wouldn't: an integer is {"i": n}, a float
// {"f": "1.5"} (exactly, infinities and NaN included), a string
// {"s": "…"}, a boolean {"b": true}, a table {"t": [[key, value], …]}, and
// nil null.
func encodeValue(v any) any {
	switch v := v.(type) {
	case bool:
		return map[string]any{"b": v}
	case int64:
		return map[string]any{"i": v}
	case float64:
		return map[string]any{"f": strconv.FormatFloat(v, 'g', -1, 64)}
	case string:
		return map[string]any{"s": v}
	case *table:
		pairs := make([][2]any, len(v.keys))
		for i := range v.keys {
			pairs[i] = [2]any{encodeValue(v.keys[i]), encodeValue(v.values[i])}
		}
		return map[string]any{"t": pairs}
	}
	return nil
}

// decodeValue undoes encodeValue, on JSON decoded into an any.
func decodeValue(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok || len(m) != 1 {
		return nil, fmt.Errorf("not a saved value: %v", v)
	}
	for k, x := range m {
		switch k {
		case "b":
			b, ok := x.(bool)
			if !ok {
				break
			}
			return b, nil
		case "i":
			n, ok := x.(json.Number)
			if !ok {
				break
			}
			return n.Int64()
		case "f":
			s, ok := x.(string)
			if !ok {
				break
			}
			return strconv.ParseFloat(s, 64)
		case "s":
			s, ok := x.(string)
			if !ok {
				break
			}
			return s, nil
		case "t":
			list, ok := x.([]any)
			if !ok {
				break
			}
			t := &table{}
			for _, pair := range list {
				kv, ok := pair.([]any)
				if !ok || len(kv) != 2 {
					return nil, fmt.Errorf("not a saved table entry: %v", pair)
				}
				key, err := decodeValue(kv[0])
				if err != nil {
					return nil, err
				}
				value, err := decodeValue(kv[1])
				if err != nil {
					return nil, err
				}
				t.keys, t.values = append(t.keys, key), append(t.values, value)
			}
			return t, nil
		}
	}
	return nil, fmt.Errorf("not a saved value: %v", v)
}
