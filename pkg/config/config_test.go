package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteTemplate_CreatesFileWithAnthropicDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := WriteTemplate(path); err != nil {
		t.Fatalf("WriteTemplate: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	got := string(data)
	if !contains(got, "protocol: anthropic") {
		t.Errorf("template missing 'protocol: anthropic', got:\n%s", got)
	}
	if !contains(got, "Please fill") {
		t.Errorf("template missing guidance to fill in api_key, got:\n%s", got)
	}
}

func TestLoad_FileMissing_WritesTemplateAndReturnsErrNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	_, err := Load(path)

	if !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("expected ErrConfigNotFound, got %v", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("expected template file to be created at %s, got stat err: %v", path, statErr)
	}
}

func TestValidate_EmptyAPIKey_ReturnsError(t *testing.T) {
	c := &Config{Protocol: "anthropic", Model: "claude-3-5-sonnet", APIKey: ""}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for empty api_key, got nil")
	}
	if !contains(err.Error(), "api_key") {
		t.Errorf("error should mention api_key, got: %v", err)
	}
}

func TestValidate_ThinkingEnabledWithNonAnthropic_ReturnsError(t *testing.T) {
	c := &Config{
		Protocol: "openai",
		Model:    "gpt-4",
		APIKey:   "sk-test",
		Thinking: Thinking{Enabled: true, BudgetTokens: 1024},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for thinking with non-anthropic protocol, got nil")
	}
	if !contains(err.Error(), "thinking") || !contains(err.Error(), "anthropic") {
		t.Errorf("error should mention thinking and anthropic, got: %v", err)
	}
}

func TestValidate_AnthropicWithThinkingEnabled_OK(t *testing.T) {
	c := &Config{
		Protocol: "anthropic",
		Model:    "claude-3-5-sonnet",
		APIKey:   "sk-test",
		Thinking: Thinking{Enabled: true, BudgetTokens: 1024},
	}
	if err := c.Validate(); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestValidate_AllValid_OK(t *testing.T) {
	c := &Config{Protocol: "anthropic", Model: "claude-3-5-sonnet", APIKey: "sk-test"}
	if err := c.Validate(); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestLoad_ValidConfig_ReturnsParsed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yamlData := `protocol: openai
model: gpt-4
base_url: https://api.openai.com
api_key: sk-test-123
thinking:
  enabled: false
  budget_tokens: 0
system_prompt: ""
`
	if err := os.WriteFile(path, []byte(yamlData), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Protocol != "openai" {
		t.Errorf("Protocol = %q, want openai", got.Protocol)
	}
	if got.Model != "gpt-4" {
		t.Errorf("Model = %q, want gpt-4", got.Model)
	}
	if got.APIKey != "sk-test-123" {
		t.Errorf("APIKey = %q, want sk-test-123", got.APIKey)
	}
}

func TestLoad_InvalidYAML_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	bad := "protocol: anthropic\n  bad indent: x\n"
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for malformed yaml, got nil")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
