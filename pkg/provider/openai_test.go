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

	s := NewOpenAIStreamer("test-key", "gpt-4", srv.URL)
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

	s := NewOpenAIStreamer("bad", "gpt-4", srv.URL)
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
