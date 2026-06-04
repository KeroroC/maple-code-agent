package provider

import "fmt"

// StreamerConfig captures the subset of user configuration that the provider layer
// needs. The config package builds this from YAML; passing it in keeps the provider
// package free of config-package imports.
type StreamerConfig struct {
	Protocol     string
	Model        string
	BaseURL      string
	APIKey       string
	Thinking     ThinkingConfig
}

// NewStreamer returns a Streamer that matches the given protocol. The base URL is
// taken from cfg.BaseURL, which means the same constructor works for "openai" (pointing
// at api.openai.com) and "openai-compatible" (pointing at a third-party gateway).
func NewStreamer(cfg StreamerConfig) (Streamer, error) {
	switch cfg.Protocol {
	case "anthropic":
		return NewAnthropicStreamer(cfg.APIKey, cfg.Model, cfg.BaseURL, cfg.Thinking), nil
	case "openai", "openai-compatible":
		return NewOpenAIStreamer(cfg.APIKey, cfg.Model, cfg.BaseURL), nil
	default:
		return nil, fmt.Errorf("unsupported protocol %q", cfg.Protocol)
	}
}
