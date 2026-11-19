package translator

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"kizuna/kernel/translator/providers"
)

// TestParseNumberedLineTolerance 覆盖模型输出格式漂移：全角括号/数字、
// Markdown 前缀、冒号分隔、代码围栏都不应让整行被判为“缺失”。
func TestParseNumberedLineTolerance(t *testing.T) {
	cases := []struct {
		line string
		idx  int
		body string
	}{
		{"[1] 你好", 1, "你好"},
		{"［２］ 早上好", 2, "早上好"},
		{"[３]：走吧", 3, "走吧"},
		{"- [4] 嗯", 4, "嗯"},
		{"> [5] 好的", 5, "好的"},
		{"`[6] 喂`", 6, "喂"},
		{"```", 0, ""},
	}
	for _, tc := range cases {
		idx, body, ok := parseNumberedLine(tc.line)
		if tc.idx == 0 {
			if ok {
				t.Errorf("%q parsed unexpectedly", tc.line)
			}
			continue
		}
		if !ok || idx != tc.idx || body != tc.body {
			t.Errorf("%q => idx=%d body=%q ok=%v, want %d/%q", tc.line, idx, body, ok, tc.idx, tc.body)
		}
	}
}

type splitProvider struct {
	bigCalls   int
	smallCalls int
}

func (p *splitProvider) Chat(ctx context.Context, req providers.Request) (providers.Response, error) {
	items := 0
	for _, line := range strings.Split(req.User, "\n") {
		if _, _, ok := parseNumberedLine(line); ok {
			items++
		}
	}
	if items > 2 {
		p.bigCalls++
		// 模拟推理模型把预算全烧在思维链上：finish_reason=length 且正文为空。
		return providers.Response{Text: "", FinishReason: "length"}, nil
	}
	p.smallCalls++
	var sb strings.Builder
	n := 0
	for _, line := range strings.Split(req.User, "\n") {
		if idx, body, ok := parseNumberedLine(line); ok {
			n++
			sb.WriteString("[" + strconv.Itoa(idx) + "] 译:" + body + "\n")
		}
	}
	_ = n
	return providers.Response{Text: sb.String(), FinishReason: "stop"}, nil
}

// TestTranslateChunkSplitsOnEmptyResponse 回归“reasoning 吃满 max_tokens，
// 正文为空 → 整块行号缺失 → 逐条重试也失败”的场景：空响应应自动对半拆块后
// 重试，而不是把整块保留原文。
func TestTranslateChunkSplitsOnEmptyResponse(t *testing.T) {
	prov := &splitProvider{}
	p := &pipe{
		cfg:      resolvedConfig{chunkSize: 4, retryLimit: 1, targetLang: "简体中文", singleTokens: 4096, chunkTokens: 4096},
		provider: prov,
		stats:    newRunStats(),
		send:     func(Event) {},
		system:   "sys",
	}
	jobs := []transJob{{orig: "一"}, {orig: "二"}, {orig: "三"}, {orig: "四"}}
	results := make([]string, len(jobs))
	skipped := make([]bool, len(jobs))
	if err := p.translateChunk(context.Background(), []int{0, 1, 2, 3}, jobs, results, skipped); err != nil {
		t.Fatal(err)
	}
	if prov.bigCalls != 1 || prov.smallCalls != 2 {
		t.Fatalf("calls big=%d small=%d, want 1/2", prov.bigCalls, prov.smallCalls)
	}
	for i := range results {
		if skipped[i] || results[i] == "" || results[i] == jobs[i].orig {
			t.Fatalf("unit %d skipped/original: %q skipped=%v", i, results[i], skipped[i])
		}
	}
}

// TestStripSingleFallback 单条重试返回不带 [1] 的正文时也要能用。
func TestStripSingleFallback(t *testing.T) {
	cases := map[string]string{
		"[1] 你好":        "你好",
		"```\n早上好\n```": "早上好",
		"译文：请进":         "请进",
		"\n\n  嗯  \n":   "嗯",
	}
	for in, want := range cases {
		if got := stripSingleFallback(in); got != want {
			t.Errorf("%q => %q, want %q", in, got, want)
		}
	}
}
