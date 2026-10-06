package browser

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRewriteClaudeLinks(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"[x](<claude:the 1919 eclipse>)", "[x](claude:the%201919%20eclipse?from=p1)"},
		{"[x](claude:eclipses)", "[x](claude:eclipses?from=p1)"},
		{"[x](claude:solar%20eclipses \"title\")", "[x](claude:solar%20eclipses?from=p1 \"title\")"},
		{"[x](<claude:why is the sky blue?>)", "[x](claude:why%20is%20the%20sky%20blue%3F?from=p1)"},
		{"[x](https://example.com) and [y](#top)", "[x](https://example.com) and [y](#top)"},
	} {
		if got := rewriteClaudeLinks(c.in, "p1"); got != c.want {
			t.Errorf("rewriteClaudeLinks(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestClaudeResolverChain follows a link from one generated page to the
// next, and checks the second request sends the first page as context.
func TestClaudeResolverChain(t *testing.T) {
	var requests [][]claudeMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Messages []claudeMessage }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, body.Messages)
		// The page in pieces, one splitting a link's destination.
		for _, text := range []string{"# " + body.Messages[len(body.Messages)-1].Content, "\n\nSee [more](<claude:", "more on it>)."} {
			delta, _ := json.Marshal(map[string]any{
				"type":  "content_block_delta",
				"delta": map[string]string{"type": "text_delta", "text": text},
			})
			fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", delta)
		}
		io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	registry := NewRegistry(nil, &ClaudeResolver{Endpoint: server.URL})
	first, err := url.Parse("claude:the 1919 eclipse")
	if err != nil {
		t.Fatal(err)
	}
	page, err := LoadDocument(registry, first)
	if err != nil {
		t.Fatal(err)
	}
	if want := "[more](claude:more%20on%20it?from=p1)"; !strings.Contains(string(page), want) {
		t.Fatalf("first page %q doesn't contain %q", page, want)
	}

	second, err := resolveAgainst(first, "claude:more%20on%20it?from=p1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDocument(registry, second); err != nil {
		t.Fatal(err)
	}
	got := requests[1]
	if len(got) != 3 || got[0].Content != "the 1919 eclipse" || got[2].Content != "more on it" ||
		!strings.Contains(got[1].Content, "<claude:more on it>") {
		t.Errorf("second request's messages = %+v, want the first request, its page as Claude wrote it, then the second request", got)
	}
}

func TestClaudeResolverAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	}))
	defer server.Close()

	registry := NewRegistry(nil, &ClaudeResolver{Endpoint: server.URL})
	_, err := LoadDocument(registry, &url.URL{Scheme: "claude", Opaque: "anything"})
	if err == nil || !strings.Contains(err.Error(), "invalid x-api-key") {
		t.Errorf("error = %v, want the API's message", err)
	}
}
