package translator

import (
	"fmt"
	"strings"
	"time"

	"kizuna/kernel/translator/providers"
)

type resolvedConfig struct {
	concurrency  int
	chunkSize    int
	retryLimit   int
	targetLang   string
	temperature  float64
	contextLines int
	glossary     []glossaryEntry
	sourceEnc    string
	targetEnc    string
	provider     providers.Spec
	chunkTokens  int
	singleTokens int
	reviewTokens int
}

func normalizeRequest(req Request) (resolvedConfig, error) {
	if strings.TrimSpace(req.Source) == "" {
		return resolvedConfig{}, fmt.Errorf("translator: empty source")
	}
	if strings.TrimSpace(req.Target) == "" {
		return resolvedConfig{}, fmt.Errorf("translator: empty target")
	}
	if strings.TrimSpace(req.Feature) == "" {
		return resolvedConfig{}, fmt.Errorf("translator: empty feature")
	}
	cfg := resolvedConfig{
		concurrency:  req.Params.Concurrency,
		chunkSize:    req.Params.ChunkSize,
		retryLimit:   req.Params.JapaneseRetryLimit,
		targetLang:   strings.TrimSpace(req.Params.TargetLanguage),
		temperature:  req.Params.Temperature,
		contextLines: req.Params.ContextLines,
		sourceEnc:    strings.ToLower(strings.TrimSpace(req.SourceEncoding)),
		targetEnc:    strings.ToLower(strings.TrimSpace(req.TargetEncoding)),
	}
	if cfg.concurrency <= 0 {
		cfg.concurrency = 4
	}
	if cfg.concurrency > 32 {
		cfg.concurrency = 32
	}
	if cfg.chunkSize <= 0 {
		cfg.chunkSize = 30
	}
	if cfg.chunkSize > 200 {
		cfg.chunkSize = 200
	}
	if cfg.retryLimit < 0 {
		cfg.retryLimit = 0
	}
	if cfg.retryLimit > 10 {
		cfg.retryLimit = 10
	}
	if cfg.targetLang == "" {
		cfg.targetLang = "简体中文"
	}
	if cfg.contextLines < 0 {
		cfg.contextLines = 0
	}
	if cfg.contextLines > 10 {
		cfg.contextLines = 10
	}
	if cfg.contextLines == 0 && req.Params.ContextLines == 0 {
		cfg.contextLines = 2
	}
	timeout := time.Duration(req.Provider.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	maxRetries := req.Provider.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	cfg.provider = providers.Spec{
		Kind:       req.Provider.Kind,
		BaseURL:    req.Provider.BaseURL,
		Model:      req.Provider.Model,
		APIKey:     req.Provider.APIKey,
		Timeout:    timeout,
		MaxRetries: maxRetries,
	}
	cfg.glossary = parseGlossary(req.Params.Glossary)
	cfg.chunkTokens = cfg.chunkSize*64 + 1024
	if cfg.chunkTokens < 1024 {
		cfg.chunkTokens = 1024
	}
	if cfg.chunkTokens > 8192 {
		cfg.chunkTokens = 8192
	}
	cfg.singleTokens = 1024
	cfg.reviewTokens = 1024
	return cfg, nil
}
