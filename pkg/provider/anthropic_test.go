package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sseAnthropic streams a fixed list of Anthropic SSE event payloads in the wire format
// the SDK expects: `event: <type>\ndata: <json>\n\n`.
func sseAnthropic(events []sseEvent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, _ := w.(http.Flusher)
		for _, e := range events {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.eventType, e.data)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

type sseEvent struct {
	eventType string
	data      string
}

func TestAnthropicStreamer_TextDeltaMappedToTextChunk(t *testing.T) {
	srv := httptest.NewServer(sseAnthropic([]sseEvent{
		{eventType: "message_start", data: `{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","model":"claude","content":[],"usage":{"input_tokens":10,"output_tokens":0}}}`},
		{eventType: "content_block_start", data: `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{eventType: "content_block_delta", data: `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello "}}`},
		{eventType: "content_block_delta", data: `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"world"}}`},
		{eventType: "content_block_stop", data: `{"type":"content_block_stop","index":0}`},
		{eventType: "message_delta", data: `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`},
		{eventType: "message_stop", data: `{"type":"message_stop"}`},
	}))
	defer srv.Close()

	s := NewAnthropicStreamer("test-key", "claude-test", srv.URL, ThinkingConfig{Enabled: false})
	ch, err := s.Stream(context.Background(), "system", []Turn{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var text, thinking strings.Builder
	var sawDone bool
	var usage Usage
	for c := range ch {
		switch v := c.(type) {
		case TextDelta:
			text.WriteString(v.Text)
		case ThinkingDelta:
			thinking.WriteString(v.Text)
		case Done:
			sawDone = true
			usage = v.Usage
		}
	}

	if text.String() != "Hello world" {
		t.Errorf("text = %q, want %q", text.String(), "Hello world")
	}
	if thinking.Len() != 0 {
		t.Errorf("expected no thinking deltas, got %q", thinking.String())
	}
	if !sawDone {
		t.Error("expected Done chunk at end of stream")
	}
	if usage.InputTokens != 10 || usage.OutputTokens != 2 {
		t.Errorf("usage = %+v, want {10 2}", usage)
	}
}

func TestAnthropicStreamer_ThinkingDeltaMappedToThinkingChunk(t *testing.T) {
	srv := httptest.NewServer(sseAnthropic([]sseEvent{
		{eventType: "message_start", data: `{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","model":"claude","content":[],"usage":{"input_tokens":5,"output_tokens":0}}}`},
		{eventType: "content_block_start", data: `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`},
		{eventType: "content_block_delta", data: `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"let me "}}`},
		{eventType: "content_block_delta", data: `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"think"}}`},
		{eventType: "content_block_delta", data: `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"answer"}}`},
		{eventType: "message_stop", data: `{"type":"message_stop"}`},
	}))
	defer srv.Close()

	s := NewAnthropicStreamer("test-key", "claude-test", srv.URL, ThinkingConfig{Enabled: true, BudgetTokens: 1024})
	ch, _ := s.Stream(context.Background(), "system", []Turn{{Role: "user", Content: "hi"}})

	var text, thinking strings.Builder
	for c := range ch {
		switch v := c.(type) {
		case TextDelta:
			text.WriteString(v.Text)
		case ThinkingDelta:
			thinking.WriteString(v.Text)
		case StreamError:
			t.Fatalf("unexpected stream error: %v", v.Err)
		}
	}

	if thinking.String() != "let me think" {
		t.Errorf("thinking = %q, want %q", thinking.String(), "let me think")
	}
	if text.String() != "answer" {
		t.Errorf("text = %q, want %q", text.String(), "answer")
	}
}

func TestAnthropicStreamer_UnauthorizedMapsToErrAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid api key"}}`)
	}))
	defer srv.Close()

	s := NewAnthropicStreamer("bad", "claude-test", srv.URL, ThinkingConfig{Enabled: false})
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
