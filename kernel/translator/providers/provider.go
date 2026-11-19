package providers

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	System      string
	User        string
	Temperature float64
	MaxTokens   int
}

type Response struct {
	Text             string
	PromptTokens     int
	CompletionTokens int
	// FinishReason 是服务端的结束原因（stop/length/...）。推理模型的思维链
	// 也计入 max_tokens，length 往往意味着正文本被截断，调用方据此分块重试。
	FinishReason string
}

type Provider interface {
	Chat(ctx context.Context, req Request) (Response, error)
}

type Spec struct {
	Kind       string
	BaseURL    string
	Model      string
	APIKey     string
	Timeout    time.Duration
	MaxRetries int
	// ReasoningEffort 为空时由 provider 自动判断：模型名带 reflection 等
	// 推理标记则发 "low"，避免思维链吃掉 max_tokens 导致正文为空/截断。
	ReasoningEffort string
}

func Detect(baseURL string, apiKey string) (string, error) {
	u := strings.ToLower(strings.TrimSpace(baseURL))
	if u == "" {
		return "", fmt.Errorf("providers: empty base url")
	}
	switch {
	case strings.Contains(u, "generativelanguage.googleapis.com"):
		return "google", nil
	case strings.Contains(u, "anthropic"):
		return "claude", nil
	default:
		return "openai", nil
	}
}

func New(spec Spec) (Provider, error) {
	kind := strings.ToLower(strings.TrimSpace(spec.Kind))
	if kind == "" || kind == "auto" {
		detected, err := Detect(spec.BaseURL, spec.APIKey)
		if err != nil {
			return nil, err
		}
		kind = detected
	}
	switch kind {
	case "openai":
		return newOpenAIProvider(spec), nil
	case "claude":
		return newClaudeProvider(spec), nil
	case "google":
		return newGoogleProvider(spec), nil
	default:
		return nil, fmt.Errorf("providers: unknown kind %q", spec.Kind)
	}
}
