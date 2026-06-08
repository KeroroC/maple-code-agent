package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicStreamer streams completions from the Anthropic Messages API.
type AnthropicStreamer struct {
	client anthropic.Client
	model  string
	think  ThinkingConfig
	tools  []anthropic.ToolParam
}

// NewAnthropicStreamer builds a streamer pointed at baseURL (use the official
// https://api.anthropic.com for production). thinking controls whether extended
// thinking is requested; callers must only set thinking.Enabled when the protocol
// is anthropic (config validation enforces this).
func NewAnthropicStreamer(apiKey, model, baseURL string, thinking ThinkingConfig, tools []anthropic.ToolParam) *AnthropicStreamer {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
	}
	return &AnthropicStreamer{
		client: anthropic.NewClient(opts...),
		model:  model,
		think:  thinking,
		tools:  tools,
	}
}

// Stream opens a streaming completion and pushes normalized Chunks to the returned
// channel. The channel is closed on success, on error, or when ctx is canceled.
func (s *AnthropicStreamer) Stream(ctx context.Context, system string, turns []Turn) (<-chan Chunk, error) {
	out := make(chan Chunk, 32)

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(s.model),
		MaxTokens: 8192,
		Messages:  toAnthropicMessages(turns),
	}
	if system != "" {
		params.System = []anthropic.TextBlockParam{{Text: system}}
	}
	if s.think.Enabled {
		params.Thinking = anthropic.ThinkingConfigParamUnion{
			OfEnabled: &anthropic.ThinkingConfigEnabledParam{
				BudgetTokens: int64(s.think.BudgetTokens),
			},
		}
	}
	if len(s.tools) > 0 {
		toolParams := make([]anthropic.ToolUnionParam, len(s.tools))
		for i, t := range s.tools {
			toolParams[i] = anthropic.ToolUnionParam{OfTool: &t}
		}
		params.Tools = toolParams
	}

	stream := s.client.Messages.NewStreaming(ctx, params)
	go func() {
		defer close(out)
		var (
			inputTokens, outputTokens int64
			toolID, toolName          string
			toolInput                 string
			inToolBlock               bool
		)
		for stream.Next() {
			event := stream.Current()
			switch v := event.AsAny().(type) {
			case anthropic.ContentBlockStartEvent:
				if v.ContentBlock.Type == "tool_use" {
					inToolBlock = true
					toolID = v.ContentBlock.ID
					toolName = v.ContentBlock.Name
					toolInput = ""
				}
			case anthropic.ContentBlockDeltaEvent:
				if inToolBlock {
					if jsonDelta, ok := v.Delta.AsAny().(anthropic.InputJSONDelta); ok {
						toolInput += jsonDelta.PartialJSON
					}
				} else {
					delta := v.Delta.AsAny()
					switch d := delta.(type) {
					case anthropic.TextDelta:
						if d.Text != "" {
							out <- TextDelta{Text: d.Text}
						}
					case anthropic.ThinkingDelta:
						if d.Thinking != "" {
							out <- ThinkingDelta{Text: d.Thinking}
						}
					}
				}
			case anthropic.ContentBlockStopEvent:
				if inToolBlock {
					out <- ToolCallDelta{
						CallID:   toolID,
						ToolName: toolName,
						ArgsJSON: json.RawMessage(toolInput),
					}
					inToolBlock = false
					toolID, toolName, toolInput = "", "", ""
				}
			case anthropic.MessageStartEvent:
				inputTokens = v.Message.Usage.InputTokens
			case anthropic.MessageDeltaEvent:
				outputTokens = v.Usage.OutputTokens
			}
		}
		if err := stream.Err(); err != nil {
			out <- StreamError{Err: classifyAnthropicErr(err)}
			return
		}
		out <- Done{Usage: Usage{InputTokens: int(inputTokens), OutputTokens: int(outputTokens)}}
	}()

	return out, nil
}

// toAnthropicMessages converts our Turn slice into the SDK's MessageParam slice,
// skipping the system turn (system is passed separately).
func toAnthropicMessages(turns []Turn) []anthropic.MessageParam {
	out := make([]anthropic.MessageParam, 0, len(turns))
	for _, t := range turns {
		if t.Role == "system" {
			continue
		}
		role := anthropic.MessageParamRoleUser
		if t.Role == "assistant" {
			role = anthropic.MessageParamRoleAssistant
		}
		out = append(out, anthropic.MessageParam{
			Role: role,
			Content: []anthropic.ContentBlockParamUnion{{
				OfText: &anthropic.TextBlockParam{Text: t.Content},
			}},
		})
	}
	return out
}

// classifyAnthropicErr maps SDK/HTTP errors onto our sentinel set.
func classifyAnthropicErr(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		msg := apiErr.Error()
		switch apiErr.StatusCode {
		case 401:
			return fmt.Errorf("%w: %s", ErrAuth, msg)
		case 429:
			return fmt.Errorf("%w: %s", ErrRateLimit, msg)
		case 400:
			if contains(msg, "context") || contains(msg, "too long") || contains(msg, "maximum") {
				return fmt.Errorf("%w: %s", ErrContextLength, msg)
			}
		}
	}
	if errors.Is(err, context.Canceled) {
		return ErrCanceled
	}
	return err
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
