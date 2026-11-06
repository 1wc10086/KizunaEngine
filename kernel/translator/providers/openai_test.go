package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func readJSONBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	return m
}

func TestOpenAIChatSuccess(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		gotBody = readJSONBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"你好"}}],"usage":{"prompt_tokens":3,"completion_tokens":5}}`)
	}))
	defer srv.Close()
	p, err := New(Spec{Kind: "openai", BaseURL: srv.URL, Model: "gpt-x", APIKey: "test-key", MaxRetries: 2})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := p.Chat(context.Background(), Request{System: "sys", User: "hello", Temperature: 0.4, MaxTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "你好" || resp.PromptTokens != 3 || resp.CompletionTokens != 5 {
		t.Errorf("unexpected response: %+v", resp)
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("path = %q, want /v1/chat/completions", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotBody["model"] != "gpt-x" {
		t.Errorf("model = %v", gotBody["model"])
	}
	msgs, ok := gotBody["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("messages = %v", gotBody["messages"])
	}
	m0 := msgs[0].(map[string]any)
	m1 := msgs[1].(map[string]any)
	if m0["role"] != "system" || m0["content"] != "sys" {
		t.Errorf("messages[0] = %v", m0)
	}
	if m1["role"] != "user" || m1["content"] != "hello" {
		t.Errorf("messages[1] = %v", m1)
	}
	if gotBody["temperature"] != 0.4 {
		t.Errorf("temperature = %v", gotBody["temperature"])
	}
	if gotBody["max_tokens"] != float64(100) {
		t.Errorf("max_tokens = %v", gotBody["max_tokens"])
	}
}

func TestOpenAIChatBaseURLWithV1(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer srv.Close()
	p, _ := New(Spec{Kind: "openai", BaseURL: srv.URL + "/v1/", Model: "gpt-x", APIKey: "k"})
	if _, err := p.Chat(context.Background(), Request{User: "hi"}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("path = %q, want /v1/chat/completions", gotPath)
	}
}

func TestOpenAIChatContentArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":[{"type":"text","text":"he"},{"type":"text","text":"llo"}]}}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`)
	}))
	defer srv.Close()
	p, _ := New(Spec{Kind: "openai", BaseURL: srv.URL, Model: "gpt-x", APIKey: "k"})
	resp, err := p.Chat(context.Background(), Request{User: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "hello" {
		t.Errorf("text = %q, want hello", resp.Text)
	}
}

func TestOpenAIChatRetry429Then200(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":"rate limited"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"recovered"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer srv.Close()
	p, _ := New(Spec{Kind: "openai", BaseURL: srv.URL, Model: "gpt-x", APIKey: "k", MaxRetries: 3})
	resp, err := p.Chat(context.Background(), Request{User: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "recovered" {
		t.Errorf("text = %q", resp.Text)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
}

func TestOpenAIChatUnauthorizedNoRetry(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"bad key"}}`)
	}))
	defer srv.Close()
	p, _ := New(Spec{Kind: "openai", BaseURL: srv.URL, Model: "gpt-x", APIKey: "secret-key-abc", MaxRetries: 3})
	_, err := p.Chat(context.Background(), Request{User: "hi"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (no retry on 401)", attempts)
	}
	msg := err.Error()
	if !strings.Contains(msg, "401") {
		t.Errorf("error missing status code: %q", msg)
	}
	if !strings.Contains(msg, "bad key") {
		t.Errorf("error missing body snippet: %q", msg)
	}
	if strings.Contains(msg, "secret-key-abc") {
		t.Errorf("error leaks api key: %q", msg)
	}
}

func TestOpenAIChatErrorBodyTruncated(t *testing.T) {
	longBody := strings.Repeat("x", 500)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, longBody)
	}))
	defer srv.Close()
	p, _ := New(Spec{Kind: "openai", BaseURL: srv.URL, Model: "gpt-x", APIKey: "k"})
	_, err := p.Chat(context.Background(), Request{User: "hi"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), longBody) {
		t.Error("error contains full 500-byte body, want truncation to 300 bytes")
	}
	if !strings.Contains(err.Error(), strings.Repeat("x", 300)) {
		t.Errorf("error missing 300-byte prefix: %q", err.Error())
	}
}

func TestOpenAIChatRetryExhausted(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `boom`)
	}))
	defer srv.Close()
	p, _ := New(Spec{Kind: "openai", BaseURL: srv.URL, Model: "gpt-x", APIKey: "k", MaxRetries: 1})
	_, err := p.Chat(context.Background(), Request{User: "hi"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2 (initial + 1 retry)", attempts)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error missing status code: %q", err.Error())
	}
}
