package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// OpenAIStreamer streams completions from any OpenAI-compatible /v1/chat/completions endpoint.
type OpenAIStreamer struct {
	client openai.Client
	model  string
}

// NewOpenAIStreamer builds a streamer. baseURL is the API root, e.g. https://api.openai.com
// for OpenAI itself or any OpenAI-compatible gateway URL.
func NewOpenAIStreamer(apiKey, model, baseURL string) *OpenAIStreamer {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
	}
	return &OpenAIStreamer{
		client: openai.NewClient(opts...),
		model:  model,
	}
}

// Stream opens a streaming chat completion and pushes normalized Chunks to the returned channel.
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
	// Ask the server to send usage in the final chunk so we can report it in Done.
	params.StreamOptions.IncludeUsage = openai.Bool(true)

	stream := s.client.Chat.Completions.NewStreaming(ctx, params)
	go func() {
		defer close(out)
		var inputTokens, outputTokens int64
		for stream.Next() {
			evt := stream.Current()
			if len(evt.Choices) > 0 {
				delta := evt.Choices[0].Delta
				if delta.Content != "" {
					out <- TextDelta{Text: delta.Content}
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
		out <- Done{Usage: Usage{InputTokens: int(inputTokens), OutputTokens: int(outputTokens)}}
	}()

	return out, nil
}

// classifyOpenAIErr maps OpenAI SDK / HTTP errors onto our sentinel set.
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
