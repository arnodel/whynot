package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/arnodel/whynot/fetch"
)

// ClaudeResolver resolves claude: URLs, such as claude:the 1919 eclipse,
// to Markdown pages that Claude writes on request, through the Messages
// API.
//
// Every claude: link in a page it generates gets a from=<page id> query,
// so that following it sends Claude the chain of pages that led there:
// each page follows on from the previous ones, and going Back and taking
// another link starts a new branch.
type ClaudeResolver struct {
	// APIKey authenticates to the Messages API.
	APIKey string
	// Model is the model ID, such as "claude-sonnet-5".
	Model string
	// Endpoint is the Messages API URL; empty means Anthropic's.
	Endpoint string
	// Client makes the requests; nil means one with claudeTimeout.
	Client *http.Client

	mu     sync.Mutex
	pages  map[string]claudePage
	nextID int
}

// claudePage is a generated page, kept so that a request from one of its
// links can send it as context.
type claudePage struct {
	request  string
	markdown string // as Claude wrote it, before rewriteClaudeLinks
	from     string // the id of the page whose link requested it, or ""
}

const (
	claudeEndpoint = "https://api.anthropic.com/v1/messages"
	// claudeTimeout is longer than httpTimeout because writing a page
	// takes tens of seconds.
	claudeTimeout = 5 * time.Minute
	// claudeMaxChain bounds how many earlier pages a request sends, to
	// bound its cost.
	claudeMaxChain = 8
)

const claudeSystemPrompt = `You write the pages of a Markdown browser. Each user message is a request for a page: a topic, a question, or the text of a link the reader followed. Reply with the page itself, in Markdown, with no preamble or closing remarks.

- Start with a level-1 heading naming the page, and organize it with headings.
- Make it self-contained and good to read on its own: about 500 to 1200 words, unless the request calls for something else.
- Link to further pages the reader might want next, written as [link text](<claude:short request>), where the request is a few words saying what the page is about, for example [the expedition](<claude:the 1919 solar eclipse expedition>). Put them inline where they fit, and optionally in a short list at the end.
- Link to the real web only when citing a specific real source that you're confident exists.
- Where a diagram helps, you can include one in Mermaid, in a ` + "```mermaid" + ` fenced code block.
- Earlier messages, if any, are the pages the reader came through to get here, most recent last. Follow on from them without repeating them.`

func (*ClaudeResolver) Schemes() []string { return []string{"claude"} }

func (r *ClaudeResolver) Resolve(u *url.URL) (fetch.Source, error) {
	request := u.Opaque
	if request == "" {
		request = u.Host + u.Path
	} else if unescaped, err := url.PathUnescape(request); err == nil {
		request = unescaped
	}
	request = strings.TrimSpace(request)
	if request == "" {
		return nil, fmt.Errorf("%s: empty request", u)
	}
	return claudeSource{resolver: r, request: request, from: u.Query().Get("from")}, nil
}

// claudeSource is a request for a page, made from the page with id from.
type claudeSource struct {
	resolver *ClaudeResolver
	request  string
	from     string
}

func (s claudeSource) Key() string { return s.request + "?from=" + s.from }

func (s claudeSource) Fetch(ctx context.Context) (io.ReadCloser, string, error) {
	r := s.resolver
	markdown, err := r.generate(ctx, r.messages(s.request, s.from))
	if err != nil {
		return nil, "", err
	}
	id := r.store(claudePage{request: s.request, markdown: markdown, from: s.from})
	return io.NopCloser(strings.NewReader(rewriteClaudeLinks(markdown, id))), "text/markdown", nil
}

// store keeps page and returns its new id.
func (r *ClaudeResolver) store(page claudePage) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pages == nil {
		r.pages = map[string]claudePage{}
	}
	r.nextID++
	id := fmt.Sprintf("p%d", r.nextID)
	r.pages[id] = page
	return id
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// messages returns the conversation for request: the chain of pages
// ending with the page from, as alternating requests and pages, then
// request.
func (r *ClaudeResolver) messages(request, from string) []claudeMessage {
	r.mu.Lock()
	var chain []claudePage
	for id := from; id != "" && len(chain) < claudeMaxChain; {
		page, ok := r.pages[id]
		if !ok {
			break
		}
		chain = append(chain, page)
		id = page.from
	}
	r.mu.Unlock()

	var msgs []claudeMessage
	for i := len(chain) - 1; i >= 0; i-- {
		msgs = append(msgs,
			claudeMessage{Role: "user", Content: chain[i].request},
			claudeMessage{Role: "assistant", Content: chain[i].markdown})
	}
	return append(msgs, claudeMessage{Role: "user", Content: request})
}

// generate sends msgs to the Messages API and returns the text of the
// reply.
func (r *ClaudeResolver) generate(ctx context.Context, msgs []claudeMessage) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":      r.Model,
		"max_tokens": 16000,
		"system":     claudeSystemPrompt,
		"messages":   msgs,
	})
	if err != nil {
		return "", err
	}
	endpoint := r.Endpoint
	if endpoint == "" {
		endpoint = claudeEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", r.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: claudeTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var reply struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Error      *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		return "", fmt.Errorf("Messages API: %s: %w", resp.Status, err)
	}
	if reply.Error != nil {
		return "", fmt.Errorf("Messages API: %s: %s", reply.Error.Type, reply.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Messages API: %s", resp.Status)
	}
	if reply.StopReason == "refusal" {
		return "", fmt.Errorf("Messages API: Claude declined to write this page")
	}
	var text strings.Builder
	for _, block := range reply.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	if reply.StopReason == "max_tokens" {
		text.WriteString("\n\n*The page was cut off here.*\n")
	}
	return text.String(), nil
}

// claudeLink matches the destination of an inline link to a claude: URL,
// with or without angle brackets: ](<claude:a request>) or
// ](claude:request).
var claudeLink = regexp.MustCompile(`\]\(\s*(?:<claude:([^>\n]*)>|claude:([^\s)]*))`)

// rewriteClaudeLinks adds from=id to every claude: link in markdown, and
// percent-encodes its request, so it needs no angle brackets.
func rewriteClaudeLinks(markdown, id string) string {
	return claudeLink.ReplaceAllStringFunc(markdown, func(m string) string {
		sub := claudeLink.FindStringSubmatch(m)
		request := sub[1] + sub[2]
		if unescaped, err := url.PathUnescape(request); err == nil {
			request = unescaped
		}
		return "](claude:" + url.PathEscape(strings.TrimSpace(request)) + "?from=" + id
	})
}
