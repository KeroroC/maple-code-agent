package provider

import "github.com/openai/openai-go"

// OpenAICompatStreamer is an OpenAIStreamer that points at a third-party gateway
// instead of api.openai.com. The wire protocol is identical; only the base URL
// differs. Config validation already ensures thinking is disabled for this protocol.
type OpenAICompatStreamer struct {
	*OpenAIStreamer
}

// NewOpenAICompatStreamer builds a streamer pointed at a custom base URL.
// The apiKey, model, and baseURL are forwarded to the underlying OpenAI streamer.
func NewOpenAICompatStreamer(apiKey, model, baseURL string, tools []openai.ChatCompletionToolParam) *OpenAICompatStreamer {
	return &OpenAICompatStreamer{
		OpenAIStreamer: NewOpenAIStreamer(apiKey, model, baseURL, tools),
	}
}
