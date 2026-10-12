// Package anthropic is an ai.Provider asking Claude, through Anthropic's
// API.
package anthropic

import (
	"context"
	"fmt"
	"io"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/arnodel/whynot/browser/internal/whymark/ai"
)

// Models maps the tiers whymark documents ask for to Claude models, for
// ai.AI's Models.
var Models = map[string]string{
	"fast":     string(anthropic.ModelClaudeHaiku4_5),
	"balanced": string(anthropic.ModelClaudeSonnet5),
	"smart":    string(anthropic.ModelClaudeOpus5),
}

// maxTokens bounds an answer, thinking included: far more than a page
// needs, so that only a runaway answer is cut off.
const maxTokens = 16000

// Provider asks Claude. Its zero value isn't usable: make one with New.
type Provider struct {
	client anthropic.Client
}

var _ ai.Provider = (*Provider)(nil)

// New returns a Provider using the API key apiKey.
func New(apiKey string, options ...option.RequestOption) *Provider {
	return &Provider{client: anthropic.NewClient(append([]option.RequestOption{option.WithAPIKey(apiKey)}, options...)...)}
}

// Ask asks Claude, streaming the answer to w if it isn't nil. A request
// with an effort thinks first, adaptively, with that effort; one with a
// schema gets JSON of that schema, through structured output.
func (p *Provider) Ask(ctx context.Context, req ai.Request, w io.Writer) (string, error) {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: maxTokens,
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}
	for _, m := range req.Messages {
		block := anthropic.NewTextBlock(m.Content)
		if m.Role == "assistant" {
			params.Messages = append(params.Messages, anthropic.NewAssistantMessage(block))
		} else {
			params.Messages = append(params.Messages, anthropic.NewUserMessage(block))
		}
	}
	if req.Effort != "" {
		params.Thinking = anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}}
		params.OutputConfig.Effort = anthropic.OutputConfigEffort(req.Effort)
	}
	if req.Schema != nil {
		params.OutputConfig.Format = anthropic.JSONOutputFormatParam{Schema: req.Schema}
	}

	stream := p.client.Messages.NewStreaming(ctx, params)
	defer stream.Close()
	var message anthropic.Message
	var text []byte
	for stream.Next() {
		event := stream.Current()
		if err := message.Accumulate(event); err != nil {
			return string(text), err
		}
		if event.Type == "content_block_delta" && event.Delta.Type == "text_delta" {
			text = append(text, event.Delta.Text...)
			if w != nil {
				if _, err := io.WriteString(w, event.Delta.Text); err != nil {
					return string(text), err
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return string(text), err
	}
	switch message.StopReason {
	case anthropic.StopReasonMaxTokens:
		return string(text), fmt.Errorf("cut off at its length limit: %w", ai.ErrIncomplete)
	case anthropic.StopReasonRefusal:
		return string(text), fmt.Errorf("declined: %w", ai.ErrIncomplete)
	}
	return string(text), nil
}
