package providers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoogleChatSuccess(t *testing.T) {
	var gotPath, gotKey string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.URL.Query().Get("key")
		gotBody = readJSONBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"你"},{"text":"好"}]}}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":6}}`)
	}))
	defer srv.Close()
	p, err := New(Spec{Kind: "google", BaseURL: srv.URL, Model: "gemini-x", APIKey: "gk", MaxRetries: 2})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := p.Chat(context.Background(), Request{System: "sys", User: "hello", Temperature: 0.5, MaxTokens: 80})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "你好" || resp.PromptTokens != 4 || resp.CompletionTokens != 6 {
		t.Errorf("unexpected response: %+v", resp)
	}
	if gotPath != "/v1beta/models/gemini-x:generateContent" {
		t.Errorf("path = %q", gotPath)
	}
	if gotKey != "gk" {
		t.Errorf("key = %q", gotKey)
	}
	si, ok := gotBody["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatalf("systemInstruction = %v", gotBody["systemInstruction"])
	}
	parts := si["parts"].([]any)
	if len(parts) != 1 || parts[0].(map[string]any)["text"] != "sys" {
		t.Errorf("systemInstruction.parts = %v", parts)
	}
	contents, ok := gotBody["contents"].([]any)
	if !ok || len(contents) != 1 {
		t.Fatalf("contents = %v", gotBody["contents"])
	}
	c0 := contents[0].(map[string]any)
	if c0["role"] != "user" {
		t.Errorf("contents[0].role = %v", c0["role"])
	}
	cparts := c0["parts"].([]any)
	if len(cparts) != 1 || cparts[0].(map[string]any)["text"] != "hello" {
		t.Errorf("contents[0].parts = %v", cparts)
	}
	gc, ok := gotBody["generationConfig"].(map[string]any)
	if !ok {
		t.Fatalf("generationConfig = %v", gotBody["generationConfig"])
	}
	if gc["temperature"] != 0.5 {
		t.Errorf("temperature = %v", gc["temperature"])
	}
	if gc["maxOutputTokens"] != float64(80) {
		t.Errorf("maxOutputTokens = %v", gc["maxOutputTokens"])
	}
}

func TestGoogleChatBaseTrailingSlash(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1}}`)
	}))
	defer srv.Close()
	p, _ := New(Spec{Kind: "google", BaseURL: srv.URL + "/", Model: "gemini-x", APIKey: "gk"})
	if _, err := p.Chat(context.Background(), Request{User: "hi"}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1beta/models/gemini-x:generateContent" {
		t.Errorf("path = %q", gotPath)
	}
}
