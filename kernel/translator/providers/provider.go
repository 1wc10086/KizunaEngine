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
