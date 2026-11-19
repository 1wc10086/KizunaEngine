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
	Model           string              `json:"model"`
	Messages        []openaiWireMessage `json:"messages"`
	Temperature     float64             `json:"temperature"`
	MaxTokens       int                 `json:"max_tokens"`
	ReasoningEffort string              `json:"reasoning_effort,omitempty"`
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

// effectiveReasoningEffort 只给推理模型发 low：普通对话模型收到未知字段
// 可能直接 400。模型名带 reflection/reasoning 或 o1/o3/o4 前缀时视为推理模型。
func effectiveReasoningEffort(spec Spec) string {
	if effort := strings.TrimSpace(spec.ReasoningEffort); effort != "" {
		return effort
	}
	model := strings.ToLower(strings.TrimSpace(spec.Model))
	switch {
	case strings.Contains(model, "reflection"),
		strings.Contains(model, "reasoning"),
		strings.HasPrefix(model, "o1"),
		strings.HasPrefix(model, "o3"),
		strings.HasPrefix(model, "o4"):
		return "low"
	}
	return ""
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
		Temperature:     req.Temperature,
		MaxTokens:       effectiveMaxTokens(req.MaxTokens),
		ReasoningEffort: effectiveReasoningEffort(p.spec),
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
			FinishReason string `json:"finish_reason"`
			Message      struct {
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
		FinishReason:     decoded.Choices[0].FinishReason,
	}, nil
}
