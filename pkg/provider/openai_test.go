package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go"
)

// sseOpenAI streams OpenAI chat completion chunks in the format
// `data: <json>\n\n` terminated by `data: [DONE]\n\n`.
func sseOpenAI(chunks []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			if flusher != nil {
				flusher.Flush()
		}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func openAIChunk(content string, withUsage bool) string {
	type delta struct {
		Content string `json:"content"`
	}
	type choice struct {
		Index int    `json:"index"`
		Delta delta  `json:"delta"`
	}
	type usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	}
	payload := map[string]any{
		"id":      "chatcmpl-1",
		"object":  "chat.completion.chunk",
		"created": 1700000000,
		"model":   "gpt-4",
		"choices": []choice{{Index: 0, Delta: delta{Content: content}}},
	}
	if withUsage {
		payload["usage"] = usage{PromptTokens: 11, CompletionTokens: 5}
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

func TestOpenAIStreamer_TextDeltasThenDone(t *testing.T) {
	srv := httptest.NewServer(sseOpenAI([]string{
		openAIChunk("hello ", false),
		openAIChunk("there", false),
		openAIChunk("", true), // usage-only tail chunk
	}))
	defer srv.Close()

	s := NewOpenAIStreamer("test-key", "gpt-4", srv.URL, nil)
	ch, err := s.Stream(context.Background(), "system", []Turn{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var text strings.Builder
	var sawDone bool
	var usage Usage
	for c := range ch {
		switch v := c.(type) {
		case TextDelta:
			text.WriteString(v.Text)
		case Done:
			sawDone = true
			usage = v.Usage
		case StreamError:
			t.Fatalf("unexpected stream error: %v", v.Err)
		}
	}

	if text.String() != "hello there" {
		t.Errorf("text = %q, want %q", text.String(), "hello there")
	}
	if !sawDone {
		t.Error("expected Done at end of stream")
	}
	if usage.InputTokens != 11 || usage.OutputTokens != 5 {
		t.Errorf("usage = %+v, want {11 5}", usage)
	}
}

func TestOpenAIStreamer_UnauthorizedMapsToErrAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"invalid api key","type":"invalid_request_error"}}`)
	}))
	defer srv.Close()

	s := NewOpenAIStreamer("bad", "gpt-4", srv.URL, nil)
	ch, err := s.Stream(context.Background(), "system", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	for c := range ch {
		if se, ok := c.(StreamError); ok {
			if !errors.Is(se.Err, ErrAuth) {
				t.Errorf("expected ErrAuth, got %v", se.Err)
			}
			return
		}
	}
	t.Fatal("expected StreamError, got nothing")
}

func TestOpenAIToolCallStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		events := []string{
			`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read_file","arguments":""}}]}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path"}}]}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\":\"main.go\"}"}}]}}]}`,
			`data: {"choices":[{"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		}
		for _, evt := range events {
			fmt.Fprint(w, evt+"\n\n")
			flusher.Flush()
		}
	}))
	defer server.Close()

	s := NewOpenAIStreamer("test-key", "gpt-4o", server.URL, nil)
	ch, err := s.Stream(context.Background(), "test", []Turn{{Role: "user", Content: "read main.go"}})
	if err != nil {
		t.Fatal(err)
	}
	var gotTool *ToolCallDelta
	var gotDone *Done
	for c := range ch {
		switch v := c.(type) {
		case ToolCallDelta:
			gotTool = &v
		case Done:
			gotDone = &v
		}
	}
	if gotTool == nil {
		t.Fatal("expected ToolCallDelta")
	}
	if gotTool.ToolName != "read_file" {
		t.Fatalf("got tool name %s", gotTool.ToolName)
	}
	if gotTool.CallID != "call_1" {
		t.Fatalf("got call ID %s", gotTool.CallID)
	}
	expected := `{"path":"main.go"}`
	if string(gotTool.ArgsJSON) != expected {
		t.Fatalf("got args %s, want %s", gotTool.ArgsJSON, expected)
	}
	if gotDone != nil {
		t.Fatal("should not emit Done when tool_calls finish reason")
	}
}

func TestOpenAIToolCallStreamWithText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		events := []string{
			`data: {"choices":[{"delta":{"content":"Let me read that for you."}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_2","function":{"name":"read_file","arguments":""}}]}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\"main.go\"}"}}]}}]}`,
			`data: {"choices":[{"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		}
		for _, evt := range events {
			fmt.Fprint(w, evt+"\n\n")
			flusher.Flush()
		}
	}))
	defer server.Close()

	s := NewOpenAIStreamer("test-key", "gpt-4o", server.URL, nil)
	ch, err := s.Stream(context.Background(), "test", []Turn{{Role: "user", Content: "read main.go"}})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	var gotTool *ToolCallDelta
	for c := range ch {
		switch v := c.(type) {
		case TextDelta:
			text += v.Text
		case ToolCallDelta:
			gotTool = &v
		}
	}
	if text != "Let me read that for you." {
		t.Fatalf("got text %q", text)
	}
	if gotTool == nil {
		t.Fatal("expected ToolCallDelta")
	}
	if gotTool.ToolName != "read_file" {
		t.Fatalf("got tool name %s", gotTool.ToolName)
	}
}

func TestOpenAIToolCallStreamMultipleTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		events := []string{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","function":{"name":"read_file","arguments":"{\"path\":\"a.go\"}"}}]}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_b","function":{"name":"write_file","arguments":"{\"path\":\"b.go\",\"content\":\"hello\"}"}}]}}]}`,
			`data: {"choices":[{"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		}
		for _, evt := range events {
			fmt.Fprint(w, evt+"\n\n")
			flusher.Flush()
		}
	}))
	defer server.Close()

	s := NewOpenAIStreamer("test-key", "gpt-4o", server.URL, nil)
	ch, err := s.Stream(context.Background(), "test", []Turn{{Role: "user", Content: "do stuff"}})
	if err != nil {
		t.Fatal(err)
	}
	var tools []ToolCallDelta
	for c := range ch {
		if tc, ok := c.(ToolCallDelta); ok {
			tools = append(tools, tc)
		}
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(tools))
	}
	if tools[0].ToolName != "read_file" || tools[0].CallID != "call_a" {
		t.Fatalf("tool 0: %+v", tools[0])
	}
	if tools[1].ToolName != "write_file" || tools[1].CallID != "call_b" {
		t.Fatalf("tool 1: %+v", tools[1])
	}
}

func TestOpenAIToolsPassedToRequest(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the request body
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	tools := []openai.ChatCompletionToolParam{
		{
			Type: "function",
			Function: openai.FunctionDefinitionParam{
				Name:        "test_tool",
				Description: openai.String("A test tool"),
				Parameters: openai.FunctionParameters(map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				}),
			},
		},
	}

	s := NewOpenAIStreamer("test-key", "gpt-4o", server.URL, tools)
	ch, err := s.Stream(context.Background(), "test", []Turn{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
	}
	if !strings.Contains(receivedBody, "test_tool") {
		t.Fatalf("expected request body to contain tool name, got: %s", receivedBody)
	}
}
