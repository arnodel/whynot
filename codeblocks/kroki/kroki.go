package kroki

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/arnodel/whynot/codeblocks"
	"github.com/arnodel/whynot/images"
)

// defaultBaseURL is kroki.io's own public instance, used unless a Plugin
// has its own BaseURL (e.g. a self-hosted instance).
const defaultBaseURL = "https://kroki.io"

// Plugin is a codeblocks.Plugin that turns diagram code blocks into
// images, rendered by Kroki. The images are PNGs, since whynot can't
// decode SVG, at the diagram tool's own resolution: a View scales an
// image down to fit, never up.
type Plugin struct {
	// BaseURL overrides the default https://kroki.io - e.g. a
	// self-hosted instance. Empty uses the default.
	BaseURL string
}

var _ codeblocks.Plugin = Plugin{}

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
	return codeblocks.Image{AsyncImage: r.image(language, code)}
}

// image builds an AsyncImage directly - no type of kroki's own needed,
// since a closure already captures everything Fetch needs (baseURL,
// diagramType, code).
func (r Plugin) image(language, code string) images.AsyncImage {
	diagramType := diagramTypes[language]
	baseURL := r.baseURL()
	return images.AsyncImage{
		// Diagram type and exact source text, so recompiling identical
		// source (a resize, a reload) reuses the cached result instead
		// of re-fetching.
		Key: "kroki:" + diagramType + ":" + code,
		// Fetch POSTs the diagram source to Kroki's own JSON API (rather
		// than its GET form, which embeds a zlib+base64 encoding of the
		// source in the URL path and has a practical length limit) and
		// returns the response body - a PNG on success.
		Fetch: func(ctx context.Context) (io.ReadCloser, error) {
			body, err := json.Marshal(struct {
				DiagramSource string `json:"diagram_source"`
			}{code})
			if err != nil {
				return nil, err
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/"+diagramType+"/png", bytes.NewReader(body))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, err
			}
			if resp.StatusCode != http.StatusOK {
				defer resp.Body.Close()
				return nil, fmt.Errorf("kroki: %s", resp.Status)
			}
			return resp.Body, nil
		},
	}
}

func (r Plugin) baseURL() string {
	if r.BaseURL != "" {
		return r.BaseURL
	}
	return defaultBaseURL
}
