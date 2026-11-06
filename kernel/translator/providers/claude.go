package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type claudeProvider struct {
	spec   Spec
	client *http.Client
}

func newClaudeProvider(spec Spec) *claudeProvider {
	return &claudeProvider{spec: spec, client: newHTTPClient(spec)}
}

type claudeWireMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeChatBody struct {
	Model       string              `json:"model"`
	MaxTokens   int                 `json:"max_tokens"`
	Temperature float64             `json:"temperature"`
	System      string              `json:"system"`
	Messages    []claudeWireMessage `json:"messages"`
}

func claudeContentText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []any:
		var b strings.Builder
		for _, item := range t {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if typ, ok := m["type"].(string); ok && typ != "" && typ != "text" {
				continue
			}
			if s, ok := m["text"].(string); ok {
				b.WriteString(s)
			}
		}
		return b.String()
	case map[string]any:
		if s, ok := t["text"].(string); ok {
			return s
		}
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (p *claudeProvider) Chat(ctx context.Context, req Request) (Response, error) {
	base := strings.TrimRight(strings.TrimSpace(p.spec.BaseURL), "/")
	if strings.HasSuffix(base, "/v1/messages") {
	} else {
		if !strings.Contains(base, "/v1") {
			base += "/v1"
		}
		base += "/messages"
	}
	payload := claudeChatBody{
		Model:       p.spec.Model,
		MaxTokens:   effectiveMaxTokens(req.MaxTokens),
		Temperature: req.Temperature,
		System:      req.System,
		Messages: []claudeWireMessage{
			{Role: "user", Content: req.User},
		},
	}
	header := map[string]string{
		"Content-Type":      "application/json",
		"x-api-key":         p.spec.APIKey,
		"anthropic-version": "2023-06-01",
	}
	status, body, err := postJSON(ctx, p.client, base, header, payload, p.spec.MaxRetries, p.spec.APIKey)
	if err != nil {
		return Response{}, err
	}
	if status < 200 || status >= 300 {
		return Response{}, statusError(status, body, p.spec.APIKey)
	}
	var decoded struct {
		Content any `json:"content"`
		Usage   struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return Response{}, fmt.Errorf("decode claude response: %w", err)
	}
	return Response{
		Text:             claudeContentText(decoded.Content),
		PromptTokens:     decoded.Usage.InputTokens,
		CompletionTokens: decoded.Usage.OutputTokens,
	}, nil
}
