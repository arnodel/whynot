// Package kroki implements whynot.CodeBlockPlugin on top of kroki.io's
// hosted diagram-rendering service (https://kroki.io), so a fenced code
// block in a recognized diagram language renders as an actual diagram
// instead of its raw/highlighted definition text.
package kroki

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/arnodel/whynot"
)

// defaultBaseURL is kroki.io's own public instance, used unless Renderer
// has its own BaseURL (e.g. a self-hosted instance).
const defaultBaseURL = "https://kroki.io"

// Renderer implements whynot.CodeBlockPlugin, rendering recognized
// fenced-code-block languages via Kroki's POST .../{type}/png endpoint
// - no output-size control (Kroki renders at whatever the underlying
// tool's native resolution is), but that's fine: whynot's own fitWidth
// never scales an image up, only down to fit the column. PNG rather
// than SVG because whynot has no SVG decoder.
type Renderer struct {
	// BaseURL overrides the default https://kroki.io - e.g. a
	// self-hosted instance. Empty uses the default.
	BaseURL string
}

var _ whynot.CodeBlockPlugin = Renderer{}

// diagramTypes maps a fenced code block's language to Kroki's own
// diagram-type slug - currently just mermaid, the one this package was
// built for. Adding another Kroki-supported diagram type later (e.g.
// "plantuml", "graphviz") is a one-line addition here.
var diagramTypes = map[string]string{
	"mermaid": "mermaid",
}

func (r Renderer) CanHandle(language string) bool {
	_, ok := diagramTypes[language]
	return ok
}

func (r Renderer) Image(language, code string) whynot.AsyncImage {
	return &diagramImage{baseURL: r.baseURL(), diagramType: diagramTypes[language], code: code}
}

func (r Renderer) baseURL() string {
	if r.BaseURL != "" {
		return r.BaseURL
	}
	return defaultBaseURL
}

// diagramImage implements whynot.AsyncImage for one fenced code block's
// diagram source.
type diagramImage struct {
	baseURL, diagramType, code string
}

// Key identifies this image by its diagram type and exact source text,
// so recompiling identical source (a resize, a reload) reuses the
// cached result instead of re-fetching.
func (d *diagramImage) Key() string {
	return "kroki:" + d.diagramType + ":" + d.code
}

// Fetch POSTs the diagram source to Kroki's own JSON API (rather than
// its GET form, which embeds a zlib+base64 encoding of the source in
// the URL path and has a practical length limit) and returns the
// response body - a PNG on success.
func (d *diagramImage) Fetch() (io.ReadCloser, error) {
	body, err := json.Marshal(struct {
		DiagramSource string `json:"diagram_source"`
	}{d.code})
	if err != nil {
		return nil, err
	}
	resp, err := http.Post(d.baseURL+"/"+d.diagramType+"/png", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, fmt.Errorf("kroki: %s", resp.Status)
	}
	return resp.Body, nil
}
