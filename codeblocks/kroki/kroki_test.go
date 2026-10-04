package kroki

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arnodel/whynot/codeblocks"
)

func TestPluginHandles(t *testing.T) {
	r := Plugin{}
	if !r.Handles("mermaid") {
		t.Error(`Handles("mermaid") = false, want true`)
	}
	if r.Handles("go") {
		t.Error(`Handles("go") = true, want false`)
	}
}

// TestDiagramImageKeyDistinguishesCodeAndType checks Key differs for
// different source text or diagram type - imagecache.Cache's own caching
// relies on this to tell distinct diagrams apart.
func TestDiagramImageKeyDistinguishesCodeAndType(t *testing.T) {
	r := Plugin{}
	a := r.image("mermaid", "graph TD; A-->B;")
	b := r.image("mermaid", "graph TD; A-->C;")
	if a.Key() == b.Key() {
		t.Errorf("Key for different source text matched: %q", a.Key())
	}
	if a.Key() != r.image("mermaid", "graph TD; A-->B;").Key() {
		t.Error("Key differed for identical (language, code) - want a stable cache key")
	}
}

// TestDiagramImageFetchPostsExpectedRequest checks Fetch's request
// shape against a fake Kroki server, rather than the real network: the
// diagram type in the URL path, PNG as the requested format, and the
// diagram source as JSON in the body.
func TestDiagramImageFetchPostsExpectedRequest(t *testing.T) {
	const pngBytes = "not a real PNG, just a fixed body to check round-tripping"
	var gotPath, gotMethod, gotContentType string
	var gotBody struct {
		DiagramSource string `json:"diagram_source"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotPath = req.URL.Path
		gotMethod = req.Method
		gotContentType = req.Header.Get("Content-Type")
		if err := json.NewDecoder(req.Body).Decode(&gotBody); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte(pngBytes))
	}))
	defer server.Close()

	r := Plugin{BaseURL: server.URL}
	img := r.image("mermaid", "graph TD; A-->B;")
	rc, mediaType, err := img.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() = _, _, %v, want nil error", err)
	}
	if mediaType != "image/png" {
		t.Errorf("media type = %q, want image/png", mediaType)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/mermaid/png" {
		t.Errorf("path = %q, want \"/mermaid/png\"", gotPath)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want \"application/json\"", gotContentType)
	}
	if gotBody.DiagramSource != "graph TD; A-->B;" {
		t.Errorf("diagram_source = %q, want the diagram's own source", gotBody.DiagramSource)
	}
	if string(got) != pngBytes {
		t.Errorf("Fetch() body = %q, want %q", got, pngBytes)
	}
}

// TestDiagramImageFetchNonOKStatus checks that a non-200 response is
// reported as an error rather than silently returned as if it were
// image data.
func TestDiagramImageFetchNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, "bad diagram source", http.StatusBadRequest)
	}))
	defer server.Close()

	r := Plugin{BaseURL: server.URL}
	img := r.image("mermaid", "not valid mermaid")
	if _, _, err := img.Fetch(context.Background()); err == nil {
		t.Error("Fetch() with a 400 response = nil error, want one")
	}
}

// TestDiagramImageFetchCancelled checks Fetch gives up when its context
// is cancelled, rather than waiting for Kroki.
func TestDiagramImageFetchCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte("an image"))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	img := Plugin{BaseURL: server.URL}.image("mermaid", "graph TD; A-->B;")
	if _, _, err := img.Fetch(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Fetch() with a cancelled context = %v, want context.Canceled", err)
	}
}

// TestPluginParseIsImage checks Parse wraps the diagram as an Image.
func TestPluginParseIsImage(t *testing.T) {
	r := Plugin{}
	content, ok := r.Parse("mermaid", "graph TD; A-->B;").(codeblocks.Image)
	if !ok {
		t.Fatalf("Parse = %T, want codeblocks.Image", r.Parse("mermaid", "graph TD; A-->B;"))
	}
	if want := r.image("mermaid", "graph TD; A-->B;").Key(); content.Source.Key() != want {
		t.Errorf("Key = %q, want %q", content.Source.Key(), want)
	}
}
