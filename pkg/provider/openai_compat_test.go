package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompatStreamer_UsesConfiguredBaseURL(t *testing.T) {
	var seenHost string
	srv := httptest.NewServer(sseOpenAI([]string{
		openAIChunk("ok", true),
	}))
	defer srv.Close()
	// Wrap the upstream handler to capture the Host header.
	originalHandler := srv.Config.Handler
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHost = r.Host
		originalHandler.ServeHTTP(w, r)
	})

	s := NewOpenAICompatStreamer("test-key", "custom-model", srv.URL)
	ch, err := s.Stream(context.Background(), "system", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	// Drain so the request is actually made.
	for c := range ch {
		if _, ok := c.(Done); ok {
			break
		}
	}

	if seenHost == "" {
		t.Fatal("server did not receive a request")
	}
	wantHost := strings.TrimPrefix(srv.URL, "http://")
	if seenHost != wantHost {
		t.Errorf("seenHost = %q, want %q", seenHost, wantHost)
	}
}
