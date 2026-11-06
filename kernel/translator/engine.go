package translator

import (
	"kizuna/kernel/translator/providers"
)

type ProviderSpec struct {
	Kind           string `json:"kind"`
	BaseURL        string `json:"baseUrl"`
	Model          string `json:"model"`
	APIKey         string `json:"apiKey"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	MaxRetries     int    `json:"maxRetries"`
}

type Params struct {
	Concurrency        int     `json:"concurrency"`
	ChunkSize          int     `json:"chunkSize"`
	JapaneseRetryLimit int     `json:"japaneseRetryLimit"`
	TargetLanguage     string  `json:"targetLanguage"`
	Temperature        float64 `json:"temperature"`
	Glossary           string  `json:"glossary"`
	ContextLines       int     `json:"contextLines"`
}

type Request struct {
	Source         string       `json:"source"`
	Target         string       `json:"target"`
	Feature        string       `json:"feature"`
	SourceEncoding string       `json:"sourceEncoding"`
	TargetEncoding string       `json:"targetEncoding"`
	Provider       ProviderSpec `json:"provider"`
	Params         Params       `json:"params"`
}

type Event struct {
	Type             string  `json:"type"`
	Level            string  `json:"level,omitempty"`
	Message          string  `json:"message,omitempty"`
	Current          int     `json:"current,omitempty"`
	Total            int     `json:"total,omitempty"`
	Percent          float64 `json:"percent,omitempty"`
	PromptTokens     int     `json:"promptTokens,omitempty"`
	CompletionTokens int     `json:"completionTokens,omitempty"`
	Requests         int     `json:"requests,omitempty"`
	Failed           int     `json:"failed,omitempty"`
	ElapsedMs        int64   `json:"elapsedMs,omitempty"`
	UnitsPerSecond   float64 `json:"unitsPerSecond,omitempty"`
	Count            int     `json:"count,omitempty"`
	Round            int     `json:"round,omitempty"`
	OK               *bool   `json:"ok,omitempty"`
	OutputPath       string  `json:"outputPath,omitempty"`
	Translated       int     `json:"translated,omitempty"`
	Skipped          int     `json:"skipped,omitempty"`
	GlossaryTerms    int     `json:"glossaryTerms,omitempty"`
}

var ProviderFactory func(providers.Spec) (providers.Provider, error) = providers.New

type Runner struct{}
