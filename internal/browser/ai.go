package browser

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AI is a language model that whynot asks for text: the pages of claude:
// URLs, and what lua: pages ask with ai{...}.
type AI interface {
	// Start sends req, and returns once the reply starts: reading the
	// reply gives its text as it arrives. It ends with ErrAICutOff if the
	// reply was cut off, or ErrAIDeclined if the model declined to answer.
	// Closing it stops the request.
	Start(ctx context.Context, req AIRequest) (io.ReadCloser, error)
}

// AIRequest is a request to an AI.
type AIRequest struct {
	// Model is a tier, "fast", "balanced" or "smart", which the AI maps to
	// one of its models, or a model of its own by name. Empty means
	// "balanced".
	Model string `json:"model,omitempty"`
	// Effort, if set, is how much the model should think: "low",
	// "medium", "high", "xhigh" or "max".
	Effort   string      `json:"effort,omitempty"`
	System   string      `json:"system,omitempty"`
	Messages []AIMessage `json:"messages"`
	// Schema, if set, is the JSON schema the reply must follow: the reply
	// is then that JSON.
	Schema map[string]any `json:"schema,omitempty"`
}

// AIMessage is a turn of a conversation with an AI.
type AIMessage struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

var (
	ErrAICutOff   = errors.New("the reply was cut off")
	ErrAIDeclined = errors.New("the model declined to answer")
)

// Anthropic is an AI on Anthropic's Messages API.
type Anthropic struct {
	// APIKey authenticates to the API.
	APIKey string
	// Fast, Balanced and Smart are the models for each tier; empty means
	// claude-haiku-4-5, claude-sonnet-5 and claude-opus-5.
	Fast, Balanced, Smart string
	// Endpoint is the Messages API URL; empty means Anthropic's.
	Endpoint string
	// Client makes the requests; nil means one with anthropicTimeout.
	Client *http.Client
}

const (
	anthropicEndpoint = "https://api.anthropic.com/v1/messages"
	// anthropicTimeout is longer than httpTimeout because a reply takes
	// tens of seconds.
	anthropicTimeout = 5 * time.Minute
)

// model returns the model for name, a tier or a model.
func (a *Anthropic) model(name string) string {
	pick := func(model, fallback string) string {
		if model != "" {
			return model
		}
		return fallback
	}
	switch name {
	case "", "balanced":
		return pick(a.Balanced, "claude-sonnet-5")
	case "fast":
		return pick(a.Fast, "claude-haiku-4-5")
	case "smart":
		return pick(a.Smart, "claude-opus-5")
	}
	return name
}

func (a *Anthropic) Start(ctx context.Context, req AIRequest) (io.ReadCloser, error) {
	model := a.model(req.Model)
	body := map[string]any{
		"model":      model,
		"max_tokens": 16000,
		"messages":   req.Messages,
		"stream":     true,
	}
	if req.System != "" {
		body["system"] = req.System
	}
	config := map[string]any{}
	// Haiku 4.5 doesn't take an effort.
	if req.Effort != "" && !strings.HasPrefix(model, "claude-haiku-4-5") {
		config["effort"] = req.Effort
	}
	if req.Schema != nil {
		config["format"] = map[string]any{"type": "json_schema", "schema": req.Schema}
	}
	if len(config) > 0 {
		body["output_config"] = config
	}
	// For the models whose classifiers can decline a request: another
	// model answers instead.
	fallbacks := model == "claude-opus-5" || model == "claude-fable-5-1"
	if fallbacks {
		body["fallbacks"] = "default"
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	endpoint := a.Endpoint
	if endpoint == "" {
		endpoint = anthropicEndpoint
	}
	ctx, cancel := context.WithCancel(ctx)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		cancel()
		return nil, err
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("x-api-key", a.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	if fallbacks {
		httpReq.Header.Set("anthropic-beta", "server-side-fallback-2026-07-01")
	}
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: anthropicTimeout}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		cancel()
		var reply struct{ Error *anthropicError }
		if json.NewDecoder(resp.Body).Decode(&reply) == nil && reply.Error != nil {
			return nil, reply.Error
		}
		return nil, fmt.Errorf("Messages API: %s", resp.Status)
	}

	pr, pw := io.Pipe()
	go func() {
		defer resp.Body.Close()
		pw.CloseWithError(readAnthropicStream(resp.Body, func(text string) error {
			_, err := io.WriteString(pw, text)
			return err
		}))
	}()
	return cancelOnClose{ReadCloser: pr, cancel: cancel}, nil
}

// anthropicError is the error object of the Messages API, in an error
// response or an error event.
type anthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (e *anthropicError) Error() string { return "Messages API: " + e.Type + ": " + e.Message }

// readAnthropicStream reads the server-sent events of a streaming reply,
// calling text with each piece of the reply's text.
func readAnthropicStream(stream io.Reader, text func(string) error) error {
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
			Error *anthropicError
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return fmt.Errorf("Messages API: %w", err)
		}
		switch {
		case event.Type == "error" && event.Error != nil:
			return event.Error
		case event.Type == "content_block_delta" && event.Delta.Type == "text_delta":
			if err := text(event.Delta.Text); err != nil {
				return err
			}
		case event.Type == "message_delta" && event.Delta.StopReason == "max_tokens":
			return ErrAICutOff
		case event.Type == "message_delta" && event.Delta.StopReason == "refusal":
			return ErrAIDeclined
		}
	}
	return scanner.Err()
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

// readText reads r to its end, calling text with each piece read.
func readText(r io.Reader, text func(string) error) error {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if err := text(string(buf[:n])); err != nil {
				return err
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
