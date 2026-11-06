package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type openaiProvider struct {
	spec   Spec
	client *http.Client
}

func newOpenAIProvider(spec Spec) *openaiProvider {
	return &openaiProvider{spec: spec, client: newHTTPClient(spec)}
}

type openaiWireMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiChatBody struct {
	Model       string              `json:"model"`
	Messages    []openaiWireMessage `json:"messages"`
	Temperature float64             `json:"temperature"`
	MaxTokens   int                 `json:"max_tokens"`
}

func openaiContentText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []any:
		var b strings.Builder
		for _, item := range t {
			switch c := item.(type) {
			case string:
				b.WriteString(c)
			case map[string]any:
				if s, ok := c["text"].(string); ok {
					b.WriteString(s)
				}
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

func (p *openaiProvider) Chat(ctx context.Context, req Request) (Response, error) {
	base := strings.TrimRight(strings.TrimSpace(p.spec.BaseURL), "/")
	if !strings.Contains(base, "/v1") {
		base += "/v1"
	}
	url := base
	if !strings.HasSuffix(url, "/chat/completions") {
		url += "/chat/completions"
	}
	payload := openaiChatBody{
		Model: p.spec.Model,
		Messages: []openaiWireMessage{
			{Role: "system", Content: req.System},
			{Role: "user", Content: req.User},
		},
		Temperature: req.Temperature,
		MaxTokens:   effectiveMaxTokens(req.MaxTokens),
	}
	header := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + p.spec.APIKey,
	}
	status, body, err := postJSON(ctx, p.client, url, header, payload, p.spec.MaxRetries, p.spec.APIKey)
	if err != nil {
		return Response{}, err
	}
	if status < 200 || status >= 300 {
		return Response{}, statusError(status, body, p.spec.APIKey)
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return Response{}, fmt.Errorf("decode openai response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return Response{}, fmt.Errorf("openai response has no choices")
	}
	return Response{
		Text:             openaiContentText(decoded.Choices[0].Message.Content),
		PromptTokens:     decoded.Usage.PromptTokens,
		CompletionTokens: decoded.Usage.CompletionTokens,
	}, nil
}
