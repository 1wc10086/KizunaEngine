package translator

import (
	"strings"
	"testing"
)

func TestChunkIndexes(t *testing.T) {
	chunks := chunkIndexes(10, 3)
	if len(chunks) != 4 {
		t.Fatalf("chunks = %d, want 4", len(chunks))
	}
	want := [][]int{{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, {9}}
	for i, w := range want {
		if len(chunks[i]) != len(w) {
			t.Fatalf("chunk %d = %v, want %v", i, chunks[i], w)
		}
		for j, v := range w {
			if chunks[i][j] != v {
				t.Fatalf("chunk %d = %v, want %v", i, chunks[i], w)
			}
		}
	}
	if got := chunkIndexes(0, 3); len(got) != 0 {
		t.Fatalf("empty = %v", got)
	}
	if got := chunkIndexes(2, 5); len(got) != 1 || len(got[0]) != 2 {
		t.Fatalf("small = %v", got)
	}
}

func TestParseChunkResponse(t *testing.T) {
	got, missing, extra := parseChunkResponse("[1] 你好\n[2] 世界\n", 2)
	if len(missing) != 0 || len(extra) != 0 {
		t.Fatalf("missing=%v extra=%v", missing, extra)
	}
	if got[1] != "你好" || got[2] != "世界" {
		t.Fatalf("got=%v", got)
	}
	got, missing, extra = parseChunkResponse("[1] 你好\n[3] 多余\n", 2)
	if len(missing) != 1 || missing[0] != 2 {
		t.Fatalf("missing=%v", missing)
	}
	if len(extra) != 1 || extra[0] != 3 {
		t.Fatalf("extra=%v", extra)
	}
	if _, ok := got[3]; ok {
		t.Fatalf("extra should be dropped: %v", got)
	}
	got, missing, _ = parseChunkResponse("[1] 第一行\n续行\n[2] 第二条\n", 2)
	if len(missing) != 0 {
		t.Fatalf("missing=%v", missing)
	}
	if !strings.HasPrefix(got[1], "第一行") || !strings.Contains(got[1], "续行") {
		t.Fatalf("continuation=%q", got[1])
	}
}

func TestLooksJapanese(t *testing.T) {
	for _, s := range []string{"こんにちは", "カタカナ", "漢字とひらがな", "の"} {
		if !looksJapanese(s) {
			t.Fatalf("%q should look japanese", s)
		}
	}
	for _, s := range []string{"", "中文訳文", "ABC123", "，。！「」", "漢字"} {
		if looksJapanese(s) {
			t.Fatalf("%q should not look japanese", s)
		}
	}
	if !containsJapanese("オープニングを見ますか？") {
		t.Fatal("choice line should contain japanese")
	}
}

func TestGlossary(t *testing.T) {
	entries := parseGlossary("明里=阿明\n先生＝老师\nX→Y\nbadline\n=A\nB=\n")
	if len(entries) != 3 {
		t.Fatalf("entries=%v", entries)
	}
	matched := matchGlossary(entries, []string{"明里来了", "无关"})
	if len(matched) != 1 || matched[0].dst != "阿明" {
		t.Fatalf("matched=%v", matched)
	}
	if f := formatGlossary(matched); !strings.Contains(f, "明里") || !strings.Contains(f, "阿明") {
		t.Fatalf("formatted=%q", f)
	}
}

func TestNormalizeRequest(t *testing.T) {
	cfg, err := normalizeRequest(Request{Source: "a", Target: "b", Feature: "f"})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if cfg.concurrency != 4 || cfg.chunkSize != 30 || cfg.retryLimit != 0 {
		t.Fatalf("defaults=%+v", cfg)
	}
	if cfg.targetLang != "简体中文" || cfg.contextLines != 2 {
		t.Fatalf("defaults=%+v", cfg)
	}
	if _, err := normalizeRequest(Request{}); err == nil {
		t.Fatal("want error for empty request")
	}
	cfg, err = normalizeRequest(Request{
		Source: "a", Target: "b", Feature: "f",
		Params: Params{Concurrency: 100, ChunkSize: 500, JapaneseRetryLimit: 50, ContextLines: 50},
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if cfg.concurrency != 32 || cfg.chunkSize != 200 || cfg.retryLimit != 10 || cfg.contextLines != 10 {
		t.Fatalf("clamped=%+v", cfg)
	}
}

func TestPromptBuilders(t *testing.T) {
	sys := buildSystemPrompt("简体中文")
	for _, want := range []string{"[i]", "假名", "术语", "成人"} {
		if !strings.Contains(sys, want) {
			t.Fatalf("system prompt missing %q", want)
		}
	}
	items := []promptItem{{text: "こんにちは"}, {text: "さようなら"}}
	user := buildChunkUserPrompt(items, []glossaryEntry{{src: "明里", dst: "阿明"}}, "简体中文")
	if !strings.Contains(user, "[1] こんにちは") || !strings.Contains(user, "[2] さようなら") {
		t.Fatalf("chunk prompt=%q", user)
	}
	if !strings.Contains(user, chunkMarker) || !strings.Contains(user, "明里") {
		t.Fatalf("chunk prompt markers=%q", user)
	}
	single := buildSingleUserPrompt("おはよう", nil, "简体中文")
	if !strings.Contains(single, singleMarker) || !strings.Contains(single, "[1] おはよう") {
		t.Fatalf("single prompt=%q", single)
	}
	review := buildReviewUserPrompt("原文", "译文", []string{"上"}, []string{"下"}, "简体中文")
	if !strings.Contains(review, reviewMarker) || !strings.Contains(review, "KEEP") {
		t.Fatalf("review prompt=%q", review)
	}
}

func TestStatsEvent(t *testing.T) {
	s := newRunStats()
	s.addRequest(10, 20)
	s.addFailed()
	s.addDoneUnits(5)
	ev := s.event()
	if ev.Type != "stats" || ev.Requests != 2 || ev.Failed != 1 {
		t.Fatalf("event=%+v", ev)
	}
	if ev.PromptTokens != 10 || ev.CompletionTokens != 20 {
		t.Fatalf("event=%+v", ev)
	}
	if ev.ElapsedMs < 0 || ev.UnitsPerSecond < 0 {
		t.Fatalf("event=%+v", ev)
	}
}
