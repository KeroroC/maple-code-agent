package provider

import "testing"

func TestToAnthropicTools(t *testing.T) {
	metas := []ToolMeta{
		{Name: "read_file", Description: "Read a file", InputSchema: map[string]any{"type": "object"}},
	}
	tools := ToAnthropicTools(metas)
	if len(tools) != 1 {
		t.Fatalf("got %d, want 1", len(tools))
	}
	if tools[0].Name != "read_file" {
		t.Fatalf("got name %s", tools[0].Name)
	}
}

func TestToOpenAITools(t *testing.T) {
	metas := []ToolMeta{
		{Name: "read_file", Description: "Read a file", InputSchema: map[string]any{"type": "object"}},
	}
	tools := ToOpenAITools(metas)
	if len(tools) != 1 {
		t.Fatalf("got %d, want 1", len(tools))
	}
	if tools[0].Function.Name != "read_file" {
		t.Fatalf("got name %s", tools[0].Function.Name)
	}
}
