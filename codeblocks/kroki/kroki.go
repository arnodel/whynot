package kroki

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/arnodel/whynot/codeblocks"
	"github.com/arnodel/whynot/fetch"
)

// defaultBaseURL is kroki.io's own public instance, used unless a Plugin
// has its own BaseURL (e.g. a self-hosted instance).
const defaultBaseURL = "https://kroki.io"

// Plugin is a [codeblocks.Plugin] that turns diagram code blocks into
// images, rendered by Kroki. The images are PNGs, since whynot can't
// decode SVG, at the diagram tool's own resolution: a View scales an
// image down to fit, never up.
type Plugin struct {
	// BaseURL overrides the default https://kroki.io - e.g. a
	// self-hosted instance. Empty uses the default.
	BaseURL string
}

var (
	_ codeblocks.Plugin = Plugin{}
	_ fetch.Source      = diagram{}
)

// diagramTypes maps a fenced code block's language to Kroki's own
// diagram-type slug - currently just mermaid, the one this package was
// built for. Adding another Kroki-supported diagram type later (e.g.
// "plantuml", "graphviz") is a one-line addition here.
var diagramTypes = map[string]string{
	"mermaid": "mermaid",
}

// Handles reports whether language is a diagram type Kroki renders.
func (r Plugin) Handles(language string) bool {
	_, ok := diagramTypes[language]
	return ok
}

// Parse returns the diagram as an Image, rendered by Kroki.
func (r Plugin) Parse(language, code string) codeblocks.Content {
	return codeblocks.Image{Source: r.image(language, code)}
}

func (r Plugin) image(language, code string) diagram {
	return diagram{baseURL: r.baseURL(), diagramType: diagramTypes[language], code: code}
}

// diagram is a [fetch.Source] for one diagram, rendered by Kroki.
type diagram struct {
	baseURL     string
	diagramType string
	code        string
}

// Key is the diagram type and exact source text, so recompiling
// identical source (a resize, a reload) reuses the cached result instead
// of fetching again.
func (d diagram) Key() string { return d.diagramType + ":" + d.code }

// Fetch POSTs the diagram source to Kroki's JSON API, rather than using
// its GET form, which encodes the source in the URL and so has a
// practical length limit. It returns the response body: a PNG on
// success.
func (d diagram) Fetch(ctx context.Context) (io.ReadCloser, string, error) {
	body, err := json.Marshal(struct {
		DiagramSource string `json:"diagram_source"`
	}{d.code})
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/"+d.diagramType+"/png", bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, "", fmt.Errorf("kroki: %s", resp.Status)
	}
	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	return resp.Body, mediaType, nil
}

func (r Plugin) baseURL() string {
	if r.BaseURL != "" {
		return r.BaseURL
	}
	return defaultBaseURL
}
