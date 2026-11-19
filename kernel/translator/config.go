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
		// 20 条一块：推理模型（如 reflection/beam）每条都要烧思维链，
		// 块太大既容易触发 max_tokens 截断，也会让单次请求等太久。
		cfg.chunkSize = 20
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
		// 最终通检给 4 行上下文：太少看不出称谓/语气，太多只会浪费 token。
		cfg.contextLines = 4
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
		Kind:            req.Provider.Kind,
		BaseURL:         req.Provider.BaseURL,
		Model:           req.Provider.Model,
		APIKey:          req.Provider.APIKey,
		Timeout:         timeout,
		MaxRetries:      maxRetries,
		ReasoningEffort: req.Provider.ReasoningEffort,
	}
	cfg.glossary = parseGlossary(req.Params.Glossary)
	// 推理模型的 reasoning_tokens 也占 max_tokens。实测 reflection/beam:free
	// 翻 30 条时思维链可烧掉 6k+ token，旧公式（768~2944）会让正文为空或
	// 末尾若干行缺失，表现为“行号 N 缺失且单独重试失败”。这里按块大小
	// 给足预算，并把重试/通检的预算也一起抬高。
	cfg.chunkTokens = cfg.chunkSize*160 + 4096
	if cfg.chunkTokens < 4096 {
		cfg.chunkTokens = 4096
	}
	if cfg.chunkTokens > 16384 {
		cfg.chunkTokens = 16384
	}
	cfg.singleTokens = 8192
	cfg.reviewTokens = 4096
	return cfg, nil
}
