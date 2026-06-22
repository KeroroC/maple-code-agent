package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// OpenAIStreamer 从任何 OpenAI 兼容的 /v1/chat/completions 端点流式获取补全。
type OpenAIStreamer struct {
	client openai.Client
	model  string
	tools  []openai.ChatCompletionToolParam
}

// NewOpenAIStreamer 构建流式传输器。baseURL 是 API 根地址，例如 OpenAI 自身的
// https://api.openai.com 或任何 OpenAI 兼容的网关 URL。
func NewOpenAIStreamer(apiKey, model, baseURL string, tools []openai.ChatCompletionToolParam) *OpenAIStreamer {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
	}
	return &OpenAIStreamer{
		client: openai.NewClient(opts...),
		model:  model,
		tools:  tools,
	}
}

// Stream 打开流式聊天补全并将标准化的 Chunk 推送到返回的通道。
func (s *OpenAIStreamer) Stream(ctx context.Context, system string, turns []Turn) (<-chan Chunk, error) {
	out := make(chan Chunk, 32)

	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(turns)+1)
	if system != "" {
		messages = append(messages, openai.SystemMessage(system))
	}
	for _, t := range turns {
		switch t.Role {
		case "user":
			messages = append(messages, openai.UserMessage(t.Content))
		case "assistant":
			messages = append(messages, openai.AssistantMessage(t.Content))
		default:
			messages = append(messages, openai.UserMessage(t.Content))
		}
	}

	params := openai.ChatCompletionNewParams{
		Model:    s.model,
		Messages: messages,
	}
	// 请求服务器在最后一个 chunk 中发送 usage，以便在 Done 中报告。
	params.StreamOptions.IncludeUsage = openai.Bool(true)
	if len(s.tools) > 0 {
		params.Tools = s.tools
	}

	stream := s.client.Chat.Completions.NewStreaming(ctx, params)
	go func() {
		defer close(out)
		type toolCallState struct {
			id   string
			name string
			args string
		}
		toolCalls := make(map[int]*toolCallState)
		finishReason := ""
		var inputTokens, outputTokens int64
		for stream.Next() {
			evt := stream.Current()
			if len(evt.Choices) > 0 {
				delta := evt.Choices[0].Delta
				if delta.Content != "" {
					out <- TextDelta{Text: delta.Content}
				}
				for _, tc := range delta.ToolCalls {
					if tc.Index >= 0 {
						idx := int(tc.Index)
						state, ok := toolCalls[idx]
						if !ok {
							state = &toolCallState{}
							toolCalls[idx] = state
						}
						if tc.ID != "" {
							state.id = tc.ID
						}
						if tc.Function.Name != "" {
							state.name = tc.Function.Name
						}
						if tc.Function.Arguments != "" {
							state.args += tc.Function.Arguments
						}
					}
				}
				if evt.Choices[0].FinishReason == "tool_calls" {
					finishReason = "tool_calls"
				}
			}
			if evt.Usage.PromptTokens > 0 || evt.Usage.CompletionTokens > 0 {
				inputTokens = evt.Usage.PromptTokens
				outputTokens = evt.Usage.CompletionTokens
			}
		}
		if err := stream.Err(); err != nil {
			out <- StreamError{Err: classifyOpenAIErr(err)}
			return
		}
		if finishReason == "tool_calls" {
			if tc, ok := toolCalls[0]; ok {
				out <- ToolCallDelta{
					CallID:   tc.id,
					ToolName: tc.name,
					ArgsJSON: json.RawMessage(tc.args),
				}
			}
		} else {
			out <- Done{Usage: Usage{InputTokens: int(inputTokens), OutputTokens: int(outputTokens)}}
		}
	}()

	return out, nil
}

// classifyOpenAIErr 将 OpenAI SDK / HTTP 错误映射到我们的哨兵错误集。
func classifyOpenAIErr(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 401:
			return fmt.Errorf("%w: %s", ErrAuth, apiErr.Error())
		case 429:
			return fmt.Errorf("%w: %s", ErrRateLimit, apiErr.Error())
		case 400:
			msg := apiErr.Error()
			if contains(msg, "context_length") || contains(msg, "context length") || contains(msg, "too long") {
				return fmt.Errorf("%w: %s", ErrContextLength, msg)
			}
		}
	}
	if errors.Is(err, context.Canceled) {
		return ErrCanceled
	}
	return err
}
