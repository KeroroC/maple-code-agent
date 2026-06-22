package provider

import "fmt"

// StreamerConfig 捕获提供者层所需的用户配置子集。
// config 包从 YAML 构建此结构；传入它可以使 provider 包不依赖 config 包的导入。
type StreamerConfig struct {
	Protocol string
	Model    string
	BaseURL  string
	APIKey   string
	Thinking ThinkingConfig
	Tools    []ToolMeta
}

// NewStreamer 返回与给定协议匹配的 Streamer。基础 URL 取自 cfg.BaseURL，
// 这意味着同一个构造函数适用于 "openai"（指向 api.openai.com）
// 和 "openai-compatible"（指向第三方网关）。
func NewStreamer(cfg StreamerConfig) (Streamer, error) {
	switch cfg.Protocol {
	case "anthropic":
		return NewAnthropicStreamer(cfg.APIKey, cfg.Model, cfg.BaseURL, cfg.Thinking, ToAnthropicTools(cfg.Tools)), nil
	case "openai":
		return NewOpenAIStreamer(cfg.APIKey, cfg.Model, cfg.BaseURL, ToOpenAITools(cfg.Tools)), nil
	case "openai-compatible":
		return NewOpenAICompatStreamer(cfg.APIKey, cfg.Model, cfg.BaseURL, ToOpenAITools(cfg.Tools)), nil
	default:
		return nil, fmt.Errorf("unsupported protocol %q", cfg.Protocol)
	}
}
