package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type googleProvider struct {
	spec   Spec
	client *http.Client
}

func newGoogleProvider(spec Spec) *googleProvider {
	return &googleProvider{spec: spec, client: newHTTPClient(spec)}
}

type googlePart struct {
	Text string `json:"text"`
}

type googleContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []googlePart `json:"parts"`
}

type googleGenerationConfig struct {
	Temperature     float64 `json:"temperature"`
	MaxOutputTokens int     `json:"maxOutputTokens"`
}

type googleChatBody struct {
	SystemInstruction googleContent          `json:"systemInstruction"`
	Contents          []googleContent        `json:"contents"`
	GenerationConfig  googleGenerationConfig `json:"generationConfig"`
}

func (p *googleProvider) Chat(ctx context.Context, req Request) (Response, error) {
	base := strings.TrimRight(strings.TrimSpace(p.spec.BaseURL), "/")
	if !strings.Contains(base, "/v1beta") {
		base += "/v1beta"
	}
	endpoint := base + "/models/" + url.PathEscape(p.spec.Model) + ":generateContent?key=" + url.QueryEscape(p.spec.APIKey)
	payload := googleChatBody{
		SystemInstruction: googleContent{Parts: []googlePart{{Text: req.System}}},
		Contents: []googleContent{
			{Role: "user", Parts: []googlePart{{Text: req.User}}},
		},
		GenerationConfig: googleGenerationConfig{
			Temperature:     req.Temperature,
			MaxOutputTokens: effectiveMaxTokens(req.MaxTokens),
		},
	}
	header := map[string]string{
		"Content-Type": "application/json",
	}
	status, body, err := postJSON(ctx, p.client, endpoint, header, payload, p.spec.MaxRetries, p.spec.APIKey)
	if err != nil {
		return Response{}, err
	}
	if status < 200 || status >= 300 {
		return Response{}, statusError(status, body, p.spec.APIKey)
	}
	var decoded struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return Response{}, fmt.Errorf("decode google response: %w", err)
	}
	if len(decoded.Candidates) == 0 {
		return Response{}, fmt.Errorf("google response has no candidates")
	}
	var b strings.Builder
	for _, part := range decoded.Candidates[0].Content.Parts {
		b.WriteString(part.Text)
	}
	return Response{
		Text:             b.String(),
		PromptTokens:     decoded.UsageMetadata.PromptTokenCount,
		CompletionTokens: decoded.UsageMetadata.CandidatesTokenCount,
	}, nil
}
