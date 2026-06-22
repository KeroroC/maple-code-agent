package provider

import "github.com/openai/openai-go"

// OpenAICompatStreamer 是指向第三方网关而非 api.openai.com 的 OpenAIStreamer。
// 线路协议完全相同；只有基础 URL 不同。配置验证已确保此协议禁用思考功能。
type OpenAICompatStreamer struct {
	*OpenAIStreamer
}

// NewOpenAICompatStreamer 构建指向自定义基础 URL 的流式传输器。
// apiKey、model 和 baseURL 转发给底层的 OpenAI 流式传输器。
func NewOpenAICompatStreamer(apiKey, model, baseURL string, tools []openai.ChatCompletionToolParam) *OpenAICompatStreamer {
	return &OpenAICompatStreamer{
		OpenAIStreamer: NewOpenAIStreamer(apiKey, model, baseURL, tools),
	}
}
