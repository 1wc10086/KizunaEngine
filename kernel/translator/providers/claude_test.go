package providers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClaudeChatSuccess(t *testing.T) {
	var gotPath, gotKey, gotVersion string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		gotBody = readJSONBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"bonjour"}],"usage":{"input_tokens":7,"output_tokens":9}}`)
	}))
	defer srv.Close()
	p, err := New(Spec{Kind: "claude", BaseURL: srv.URL, Model: "claude-x", APIKey: "ck", MaxRetries: 2})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := p.Chat(context.Background(), Request{System: "sys", User: "hello", Temperature: 0.3, MaxTokens: 50})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "bonjour" || resp.PromptTokens != 7 || resp.CompletionTokens != 9 {
		t.Errorf("unexpected response: %+v", resp)
	}
	if gotPath != "/v1/messages" {
		t.Errorf("path = %q, want /v1/messages", gotPath)
	}
	if gotKey != "ck" {
		t.Errorf("x-api-key = %q", gotKey)
	}
	if gotVersion != "2023-06-01" {
		t.Errorf("anthropic-version = %q", gotVersion)
	}
	if gotBody["model"] != "claude-x" {
		t.Errorf("model = %v", gotBody["model"])
	}
	if gotBody["max_tokens"] != float64(50) {
		t.Errorf("max_tokens = %v", gotBody["max_tokens"])
	}
	if gotBody["temperature"] != 0.3 {
		t.Errorf("temperature = %v", gotBody["temperature"])
	}
	if gotBody["system"] != "sys" {
		t.Errorf("system = %v", gotBody["system"])
	}
	msgs, ok := gotBody["messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Fatalf("messages = %v", gotBody["messages"])
	}
	m0 := msgs[0].(map[string]any)
	if m0["role"] != "user" || m0["content"] != "hello" {
		t.Errorf("messages[0] = %v", m0)
	}
}

func TestClaudeChatBaseURLWithV1(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer srv.Close()
	p, _ := New(Spec{Kind: "claude", BaseURL: srv.URL + "/v1", Model: "claude-x", APIKey: "ck"})
	if _, err := p.Chat(context.Background(), Request{User: "hi"}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/messages" {
		t.Errorf("path = %q, want /v1/messages", gotPath)
	}
}
