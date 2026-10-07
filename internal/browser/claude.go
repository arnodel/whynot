package browser

import (
	"bufio"
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
	// Instructions, if any, are the reader's: about the pages' style,
	// audience or subject, for example. They come after the built-in
	// ones, which they can override, except for how to write links.
	Instructions string
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

// Fetch starts writing the page, and returns once Claude starts replying:
// the body is the page's Markdown as it arrives. Closing the body, or
// cancelling ctx, stops the request; the text so far is kept as the page.
func (s claudeSource) Fetch(ctx context.Context) (io.ReadCloser, string, error) {
	r := s.resolver
	ctx, cancel := context.WithCancel(ctx)
	resp, err := r.send(ctx, r.systemPrompt(), r.messages(s.request, s.from), 16000)
	if err != nil {
		cancel()
		return nil, "", err
	}
	id := r.newID()
	pr, pw := io.Pipe()
	go func() {
		defer resp.Body.Close()
		var markdown strings.Builder
		links := linkRewriter{w: pw, id: id}
		err := readClaudeStream(resp.Body, func(text string) error {
			markdown.WriteString(text)
			return links.write(text)
		})
		if err == nil {
			err = links.flush()
		}
		r.store(id, claudePage{request: s.request, markdown: markdown.String(), from: s.from})
		pw.CloseWithError(err)
	}()
	return cancelOnClose{ReadCloser: pr, cancel: cancel}, "text/markdown", nil
}

// cancelOnClose is a body whose Close also cancels its request.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c cancelOnClose) Close() error {
	c.cancel()
	return c.ReadCloser.Close()
}

// newID returns the id of a new page.
func (r *ClaudeResolver) newID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	return fmt.Sprintf("p%d", r.nextID)
}

// store keeps page under id.
func (r *ClaudeResolver) store(id string, page claudePage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pages == nil {
		r.pages = map[string]claudePage{}
	}
	r.pages[id] = page
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

// systemPrompt returns the built-in instructions, followed by the
// reader's, if any.
func (r *ClaudeResolver) systemPrompt() string {
	if strings.TrimSpace(r.Instructions) == "" {
		return claudeSystemPrompt
	}
	return claudeSystemPrompt + "\n\nThe reader's instructions follow. They take precedence over the ones above, except for how to write links to further pages.\n\n" + r.Instructions
}

// claudeError is the error object of the Messages API, in an error
// response or an error event.
type claudeError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (e *claudeError) Error() string { return "Messages API: " + e.Type + ": " + e.Message }

// send makes a streaming request for msgs, and returns the response once
// it starts.
func (r *ClaudeResolver) send(ctx context.Context, system string, msgs []claudeMessage, maxTokens int) (*http.Response, error) {
	body, err := json.Marshal(map[string]any{
		"model":      r.Model,
		"max_tokens": maxTokens,
		"system":     system,
		"messages":   msgs,
		"stream":     true,
	})
	if err != nil {
		return nil, err
	}
	endpoint := r.Endpoint
	if endpoint == "" {
		endpoint = claudeEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
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
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		var reply struct{ Error *claudeError }
		if json.NewDecoder(resp.Body).Decode(&reply) == nil && reply.Error != nil {
			return nil, reply.Error
		}
		return nil, fmt.Errorf("Messages API: %s", resp.Status)
	}
	return resp, nil
}

// readClaudeStream reads the server-sent events of a streaming reply,
// calling text with each piece of the reply's text.
func readClaudeStream(stream io.Reader, text func(string) error) error {
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(nil, 1<<20)
	for scanner.Scan() {
		data, ok := strings.CutPrefix(scanner.Text(), "data:")
		if !ok {
			continue
		}
		var event struct {
			Type  string
			Delta struct {
				Type       string
				Text       string
				StopReason string `json:"stop_reason"`
			}
			Error *claudeError
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return fmt.Errorf("Messages API: %w", err)
		}
		var err error
		switch {
		case event.Type == "error" && event.Error != nil:
			return event.Error
		case event.Type == "content_block_delta" && event.Delta.Type == "text_delta":
			err = text(event.Delta.Text)
		case event.Type == "message_delta" && event.Delta.StopReason == "max_tokens":
			err = text("\n\n*The page was cut off here.*\n")
		case event.Type == "message_delta" && event.Delta.StopReason == "refusal":
			err = text("\n\n*Claude declined to write this page.*\n")
		}
		if err != nil {
			return err
		}
	}
	return scanner.Err()
}

// linkRewriter writes text to w with rewriteClaudeLinks applied, holding
// back an unfinished link destination until the rest of it arrives.
type linkRewriter struct {
	w       io.Writer
	id      string
	pending string
}

func (l *linkRewriter) write(text string) error {
	l.pending += text
	cut := len(l.pending)
	if i := strings.LastIndex(l.pending, "]("); i >= 0 && !strings.ContainsAny(l.pending[i:], ")\n") {
		cut = i
	} else if strings.HasSuffix(l.pending, "]") {
		cut--
	}
	_, err := io.WriteString(l.w, rewriteClaudeLinks(l.pending[:cut], l.id))
	l.pending = l.pending[cut:]
	return err
}

func (l *linkRewriter) flush() error {
	_, err := io.WriteString(l.w, rewriteClaudeLinks(l.pending, l.id))
	l.pending = ""
	return err
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
