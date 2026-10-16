package whymark

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"go.yaml.in/yaml/v3"
)

// A Request is what a turn starts from: where to go, from which page,
// with what was typed.
type Request struct {
	// Location is the document's URL, with an optional query and an
	// optional fragment naming a passage. With no passage, the turn shows
	// the passage of the page it's from, or the start.
	Location *url.URL
	// FromPageID is the page-id of the page the request comes from, or ""
	// to start the document.
	FromPageID string
	// Input is the named fields typed in, if any.
	Input map[string]string
}

// A Header is what a page says about itself, before its Markdown.
type Header struct {
	// Location is where the page is: the request's location, with its
	// passage. A page's links resolve against it (see Follow).
	Location *url.URL
	// PageID identifies the page, for requests from it (see
	// Request.FromPageID). It's opaque: only the engine knows what's
	// behind it.
	PageID string
	// Title is the page's title.
	Title string
	// Journal is a hint that the document reads best as a trail.
	Journal bool
	// Resume is a hint that opening the document again should carry on
	// from this page, its latest.
	Resume bool
}

// header is a Header as written in a page's front matter.
type header struct {
	Location string `yaml:"location"`
	PageID   string `yaml:"page-id"`
	Title    string `yaml:"title,omitempty"`
	Journal  bool   `yaml:"journal,omitempty"`
	Resume   bool   `yaml:"resume,omitempty"`
}

// writeHeader writes h to w as a page's front matter.
func writeHeader(w io.Writer, h Header) error {
	data, err := yaml.Marshal(header{Location: h.Location.String(), PageID: h.PageID, Title: h.Title, Journal: h.Journal, Resume: h.Resume})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "---\n%s---\n\n", data)
	return err
}

// ReadHeader reads a page's header from the start of r, leaving r at the
// start of its Markdown.
func ReadHeader(r *bufio.Reader) (Header, error) {
	var h Header
	if line, err := r.ReadString('\n'); err != nil || strings.TrimRight(line, "\r\n") != "---" {
		return h, errors.New("a page starts with its header, between --- lines")
	}
	var front bytes.Buffer
	for {
		line, err := r.ReadString('\n')
		if strings.TrimRight(line, "\r\n") == "---" {
			break
		}
		if err != nil {
			return h, errors.New("a page's header ends with a --- line")
		}
		front.WriteString(line)
	}
	var raw header
	if err := yaml.Unmarshal(front.Bytes(), &raw); err != nil {
		return h, fmt.Errorf("a page's header: %w", err)
	}
	location, err := url.Parse(raw.Location)
	if err != nil || raw.Location == "" {
		return h, fmt.Errorf("a page's header: location %q", raw.Location)
	}
	return Header{Location: location, PageID: raw.PageID, Title: raw.Title, Journal: raw.Journal, Resume: raw.Resume}, nil
}

// Follow returns the request for a link, as written, on the page with
// header h, and whether it continues the page's trail. A link written as
// "#passage" or "?query" does: it's sent with the page's page-id, to a
// passage of the same document with no query, or with a query to the
// page's own passage. Any other link doesn't: one naming a whymark
// document, even this one, starts it, and one to anything else is an
// ordinary link.
func Follow(h Header, link string) (Request, bool) {
	if !strings.HasPrefix(link, "#") && !strings.HasPrefix(link, "?") {
		return Request{}, false
	}
	ref, err := url.Parse(link)
	if err != nil {
		return Request{}, false
	}
	location := *h.Location
	location.RawQuery, location.ForceQuery = ref.RawQuery, false
	location.Fragment, location.RawFragment = ref.Fragment, ref.RawFragment
	return Request{Location: &location, FromPageID: h.PageID}, true
}
