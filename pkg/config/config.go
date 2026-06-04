// Package config loads, validates, and writes the MapleCode configuration file.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultSystemPrompt is the system prompt used when the YAML file does not override it.
const DefaultSystemPrompt = "You are a coding assistant. Help users write, debug, and improve code.\nBe concise unless asked for details. Output code in markdown blocks."

// ErrConfigNotFound is returned by Load when the config file does not exist on disk.
// After receiving this error, callers should exit cleanly because WriteTemplate has
// already created a starter file at the requested path.
var ErrConfigNotFound = errors.New("config file not found; template written")

// Config holds all user-tunable settings loaded from the YAML file.
type Config struct {
	Protocol     string   `yaml:"protocol"`
	Model        string   `yaml:"model"`
	BaseURL      string   `yaml:"base_url"`
	APIKey       string   `yaml:"api_key"`
	Thinking     Thinking `yaml:"thinking"`
	SystemPrompt string   `yaml:"system_prompt"`
}

// Thinking controls Anthropic extended thinking. Only honored when Protocol is "anthropic".
type Thinking struct {
	Enabled      bool `yaml:"enabled"`
	BudgetTokens int  `yaml:"budget_tokens"`
}

// Load reads the YAML config at path. If the file does not exist, it writes the default
// template and returns ErrConfigNotFound (caller should exit 0 with a friendly message).
// If the file exists but is malformed or invalid, a descriptive error is returned.
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

// Validate checks the parsed config for required fields and cross-field constraints.
func (c *Config) Validate() error {
	if c.APIKey == "" {
		return errors.New("api_key is required in config")
	}
	if c.Thinking.Enabled && c.Protocol != "anthropic" {
		return fmt.Errorf("thinking is only supported with protocol=anthropic, got %q", c.Protocol)
	}
	return nil
}

// DefaultPath returns the conventional config path: $HOME/.maplecode/config.yaml.
// Tests should construct paths explicitly via t.TempDir instead of relying on this.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".maplecode", "config.yaml"), nil
}
