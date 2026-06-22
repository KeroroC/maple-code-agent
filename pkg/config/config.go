// Package config 负责加载、验证和写入 MapleCode 配置文件。
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultSystemPrompt 是当 YAML 文件未覆盖时使用的默认系统提示词。
const DefaultSystemPrompt = "You are a coding assistant. Help users write, debug, and improve code.\nBe concise unless asked for details. Output code in markdown blocks."

// ErrConfigNotFound 由 Load 在配置文件不存在时返回。
// 收到此错误后，调用者应正常退出，因为 WriteTemplate 已在请求路径创建了初始文件。
var ErrConfigNotFound = errors.New("config file not found; template written")

// Config 包含从 YAML 文件加载的所有用户可配置设置。
type Config struct {
	Protocol     string   `yaml:"protocol"`
	Model        string   `yaml:"model"`
	BaseURL      string   `yaml:"base_url"`
	APIKey       string   `yaml:"api_key"`
	Thinking     Thinking `yaml:"thinking"`
	SystemPrompt string   `yaml:"system_prompt"`
}

// Thinking 控制 Anthropic 扩展思考功能。仅当 Protocol 为 "anthropic" 时生效。
type Thinking struct {
	Enabled      bool `yaml:"enabled"`
	BudgetTokens int  `yaml:"budget_tokens"`
}

// Load 读取指定路径的 YAML 配置。如果文件不存在，会写入默认模板并返回
// ErrConfigNotFound（调用者应以 0 退出并显示友好消息）。
// 如果文件存在但格式错误或无效，会返回描述性错误。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if writeErr := WriteTemplate(path); writeErr != nil {
				return nil, fmt.Errorf("write template: %w", writeErr)
			}
			return nil, ErrConfigNotFound
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config yaml: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Validate 检查已解析配置的必填字段和跨字段约束。
func (c *Config) Validate() error {
	if c.APIKey == "" {
		return errors.New("api_key is required in config")
	}
	if c.Thinking.Enabled && c.Protocol != "anthropic" {
		return fmt.Errorf("thinking is only supported with protocol=anthropic, got %q", c.Protocol)
	}
	return nil
}

// DefaultPath 返回常规配置路径：$HOME/.maplecode/config.yaml。
// 测试应通过 t.TempDir 显式构造路径，而不是依赖此函数。
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".maplecode", "config.yaml"), nil
}
