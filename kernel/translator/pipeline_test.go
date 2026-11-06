package translator

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "kizuna/kernel/brand/bishop/bdt"
	"kizuna/kernel/core"
	"kizuna/kernel/core/textcodec"
	"kizuna/kernel/translator/providers"
)

var errFakeBoom = errors.New("fake provider boom")

type fakeProvider struct {
	mu          sync.Mutex
	chunkCalls  int
	singleCalls int
	reviewCalls int
	cur         int32
	maxConcur   int32
	failLeft    int
	delay       time.Duration
	kanaOnce    map[string]bool
	kanaAlways  map[string]bool
	seen        map[string]int
	reviewText  string
}

func (f *fakeProvider) Chat(ctx context.Context, req providers.Request) (providers.Response, error) {
	cur := atomic.AddInt32(&f.cur, 1)
	for {
		m := atomic.LoadInt32(&f.maxConcur)
		if cur <= m || atomic.CompareAndSwapInt32(&f.maxConcur, m, cur) {
			break
		}
	}
	defer atomic.AddInt32(&f.cur, -1)
	if f.delay > 0 {
		select {
		case <-ctx.Done():
			return providers.Response{}, ctx.Err()
		case <-time.After(f.delay):
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failLeft > 0 {
		f.failLeft--
		return providers.Response{}, errFakeBoom
	}
	switch {
	case strings.Contains(req.User, reviewMarker):
		f.reviewCalls++
		return providers.Response{Text: f.reviewText}, nil
	case strings.Contains(req.User, singleMarker):
		f.singleCalls++
		orig := numberedOrig(req.User, 1)
		return providers.Response{Text: f.render(1, orig)}, nil
	default:
		f.chunkCalls++
		return providers.Response{Text: f.renderChunk(req.User)}, nil
	}
}

func (f *fakeProvider) render(idx int, orig string) string {
	f.seen[orig]++
	if f.kanaAlways[orig] {
		return "[" + strconv.Itoa(idx) + "] " + orig + "のまま"
	}
	if f.kanaOnce[orig] && f.seen[orig] == 1 {
		return "[" + strconv.Itoa(idx) + "] " + orig
	}
	return "[" + strconv.Itoa(idx) + "] 翻訳文" + strconv.Itoa(idx)
}

func (f *fakeProvider) renderChunk(user string) string {
	type item struct {
		idx  int
		orig string
	}
	var items []item
	for _, line := range strings.Split(user, "\n") {
		idx, body, ok := parseNumberedLine(line)
		if !ok {
			continue
		}
		items = append(items, item{idx: idx, orig: body})
	}
	var sb strings.Builder
	for _, it := range items {
		sb.WriteString(f.render(it.idx, it.orig))
		sb.WriteString("\n")
	}
	return sb.String()
}

func numberedOrig(user string, want int) string {
	for _, line := range strings.Split(user, "\n") {
		idx, body, ok := parseNumberedLine(line)
		if ok && idx == want {
			return body
		}
	}
	return ""
}

func buildBDT(t *testing.T, lines []string) []byte {
	t.Helper()
	var raw []byte
	for i, ln := range lines {
		if i > 0 {
			raw = append(raw, '\r', '\n')
		}
		b, err := textcodec.Encode("cp932", ln)
		if err != nil {
			t.Fatalf("encode cp932: %v", err)
		}
		raw = append(raw, b...)
	}
	out := make([]byte, len(raw))
	for i, c := range raw {
		out[i] = ^c
	}
	return out
}

func bdtCodec(t *testing.T) core.Codec {
	t.Helper()
	f, ok := core.Find("bishop.bdt")
	if !ok {
		t.Fatal("bishop.bdt not registered")
	}
	c, ok := f.(core.Codec)
	if !ok {
		t.Fatal("bishop.bdt is not a codec")
	}
	return c
}

func scriptLines() []string {
	return []string{
		"$999-01",
		"!B\t\"こんにちは\"",
		"\"今日はいい天気ですね",
		"@ゲームオーバー",
		"!明里\t\"先生、明里はここにいます\"",
		"CD30",
		"?YN01,02,オープニングを見ますか？",
		"# コメント行",
		"",
		"!B\t\"さようなら\"",
		"\"ありがとう\"",
		"WJ1,999-99",
	}
}

type eventLog struct {
	mu sync.Mutex
	ev []Event
}

func (l *eventLog) emit(ev Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ev = append(l.ev, ev)
}

func (l *eventLog) byType(typ string) []Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Event
	for _, e := range l.ev {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

func withFake(t *testing.T, f *fakeProvider) {
	t.Helper()
	old := ProviderFactory
	ProviderFactory = func(providers.Spec) (providers.Provider, error) {
		return f, nil
	}
	t.Cleanup(func() { ProviderFactory = old })
}

func baseRequest(src string, dst string) Request {
	return Request{
		Source:         src,
		Target:         dst,
		Feature:        "bishop.bdt",
		SourceEncoding: "auto",
		TargetEncoding: "utf8",
		Provider: ProviderSpec{
			Kind:           "openai",
			BaseURL:        "http://fake.invalid/v1",
			Model:          "fake-model",
			APIKey:         "fake-key",
			TimeoutSeconds: 10,
			MaxRetries:     1,
		},
		Params: Params{
			Concurrency:        4,
			ChunkSize:          2,
			JapaneseRetryLimit: 2,
			TargetLanguage:     "简体中文",
			Temperature:        0,
			Glossary:           "明里=阿明",
			ContextLines:       1,
		},
	}
}

func TestBDTSplitAffixes(t *testing.T) {
	c := bdtCodec(t)
	doc, err := c.Decode(buildBDT(t, scriptLines()), "scene.bdt")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	byID := map[string]core.Unit{}
	for _, u := range doc.Units {
		byID[u.ID] = u
	}
	u := byID["line/1"]
	if u.Prefix != "!B\t\"" || u.Text != "こんにちは" || u.Suffix != "\"" {
		t.Fatalf("line/1 split = %+v", u)
	}
	u = byID["line/2"]
	if u.Prefix != "\"" || u.Text != "今日はいい天気ですね" || u.Suffix != "" {
		t.Fatalf("line/2 split = %+v", u)
	}
	u = byID["line/3"]
	if u.Prefix != "@" || u.Text != "ゲームオーバー" || u.Suffix != "" {
		t.Fatalf("line/3 split = %+v", u)
	}
	for _, id := range []string{"line/0", "line/5", "line/6", "line/7", "line/8", "line/11"} {
		r := byID[id]
		if r.Kind != "raw" || r.Prefix != "" || r.Suffix != "" {
			t.Fatalf("%s should stay raw without affixes: %+v", id, r)
		}
	}
}

func TestPipelineEndToEnd(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "scene.bdt")
	dst := filepath.Join(dir, "scene.out.bdt")
	lines := scriptLines()
	if err := os.WriteFile(src, buildBDT(t, lines), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	fake := &fakeProvider{
		delay:      5 * time.Millisecond,
		kanaOnce:   map[string]bool{"こんにちは": true},
		kanaAlways: map[string]bool{"さようなら": true},
		seen:       map[string]int{},
		reviewText: "確定訳文",
	}
	withFake(t, fake)
	log := &eventLog{}
	req := baseRequest(src, dst)
	err := Runner{}.Run(context.Background(), req, log.emit)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := atomic.LoadInt32(&fake.maxConcur); got < 2 {
		t.Fatalf("max concurrency = %d, want >= 2", got)
	}
	fake.mu.Lock()
	chunkCalls := fake.chunkCalls
	singleCalls := fake.singleCalls
	reviewCalls := fake.reviewCalls
	fake.mu.Unlock()
	if chunkCalls != 3 {
		t.Fatalf("chunk calls = %d, want 3", chunkCalls)
	}
	if singleCalls != 3 {
		t.Fatalf("single calls = %d, want 3 (1 kana-once + 2 retry-limit)", singleCalls)
	}
	if reviewCalls != 1 {
		t.Fatalf("review calls = %d, want 1", reviewCalls)
	}
	if errs := log.byType("error"); len(errs) != 0 {
		t.Fatalf("unexpected error events: %+v", errs)
	}
	dones := log.byType("done")
	if len(dones) != 1 {
		t.Fatalf("done events = %d, want 1", len(dones))
	}
	done := dones[0]
	if done.OK == nil || !*done.OK {
		t.Fatalf("done ok = %+v", done.OK)
	}
	if done.OutputPath != dst {
		t.Fatalf("outputPath = %q", done.OutputPath)
	}
	if done.Translated != 6 || done.Skipped != 6 {
		t.Fatalf("translated/skipped = %d/%d, want 6/6", done.Translated, done.Skipped)
	}
	progs := log.byType("progress")
	if len(progs) != 3 {
		t.Fatalf("progress events = %d, want 3", len(progs))
	}
	last := progs[len(progs)-1]
	if last.Current != 6 || last.Total != 6 || last.Percent != 100 {
		t.Fatalf("last progress = %+v", last)
	}
	resids := log.byType("residual")
	if len(resids) != 1 || resids[0].Round != 1 || resids[0].Count != 1 {
		t.Fatalf("residual events = %+v", resids)
	}
	var finalStats *Event
	for _, e := range log.byType("stats") {
		if e.Requests == 7 {
			c := e
			finalStats = &c
		}
	}
	if finalStats == nil {
		t.Fatalf("no stats event with 7 requests")
	}
	if finalStats.Failed != 0 || finalStats.UnitsPerSecond <= 0 {
		t.Fatalf("final stats = %+v", *finalStats)
	}
	found := false
	for _, e := range log.byType("log") {
		if strings.Contains(e.Message, "[3/3]") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing [3/3] chunk log")
	}
	out, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	c := bdtCodec(t)
	doc, err := c.Decode(out, "scene.out.bdt")
	if err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	byID := map[string]core.Unit{}
	for _, u := range doc.Units {
		byID[u.ID] = u
	}
	if got := byID["line/1"].Text; got != "翻訳文1" {
		t.Fatalf("line/1 = %q, want 翻訳文1", got)
	}
	if got := byID["line/9"].Text; got != "確定訳文" {
		t.Fatalf("line/9 = %q, want review text", got)
	}
	if u := byID["line/1"]; u.Prefix != "!B\t\"" || u.Suffix != "\"" {
		t.Fatalf("line/1 affixes lost: %+v", u)
	}
	for _, id := range []string{"line/1", "line/2", "line/3", "line/4", "line/9", "line/10"} {
		if looksJapanese(byID[id].Text) {
			t.Fatalf("%s still has kana: %q", id, byID[id].Text)
		}
	}
	if got := byID["line/5"].Text; got != "CD30" {
		t.Fatalf("raw line changed: %q", got)
	}
	if got := byID["line/6"].Text; got != "?YN01,02,オープニングを見ますか？" {
		t.Fatalf("choice line changed: %q", got)
	}
	again, err := c.Encode(doc)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	if !bytes.Equal(again, out) {
		t.Fatalf("output not stable under re-encode")
	}
}

func TestPipelineNoJapanese(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "cmd.bdt")
	dst := filepath.Join(dir, "cmd.out.bdt")
	lines := []string{"$L1", "CD30", "# comment", "", "WS"}
	raw := buildBDT(t, lines)
	if err := os.WriteFile(src, raw, 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	fake := &fakeProvider{seen: map[string]int{}, reviewText: "x"}
	withFake(t, fake)
	log := &eventLog{}
	err := Runner{}.Run(context.Background(), baseRequest(src, dst), log.emit)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	fake.mu.Lock()
	total := fake.chunkCalls + fake.singleCalls + fake.reviewCalls
	fake.mu.Unlock()
	if total != 0 {
		t.Fatalf("provider calls = %d, want 0", total)
	}
	dones := log.byType("done")
	if len(dones) != 1 || dones[0].OK == nil || !*dones[0].OK {
		t.Fatalf("done = %+v", dones)
	}
	if dones[0].Translated != 0 || dones[0].Skipped != len(lines) {
		t.Fatalf("translated/skipped = %d/%d", dones[0].Translated, dones[0].Skipped)
	}
	out, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if !bytes.Equal(out, raw) {
		t.Fatalf("no-japanese output differs from source")
	}
}

func TestPipelineProviderError(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "scene.bdt")
	dst := filepath.Join(dir, "scene.out.bdt")
	if err := os.WriteFile(src, buildBDT(t, scriptLines()), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	fake := &fakeProvider{failLeft: 100, seen: map[string]int{}}
	withFake(t, fake)
	log := &eventLog{}
	err := Runner{}.Run(context.Background(), baseRequest(src, dst), log.emit)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if errs := log.byType("error"); len(errs) == 0 {
		t.Fatal("missing error event")
	}
	dones := log.byType("done")
	if len(dones) != 1 || dones[0].OK == nil || *dones[0].OK {
		t.Fatalf("done = %+v", dones)
	}
}

func TestPipelineCancel(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "scene.bdt")
	dst := filepath.Join(dir, "scene.out.bdt")
	if err := os.WriteFile(src, buildBDT(t, scriptLines()), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	fake := &fakeProvider{seen: map[string]int{}}
	withFake(t, fake)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	log := &eventLog{}
	err := Runner{}.Run(ctx, baseRequest(src, dst), log.emit)
	if err != nil {
		t.Fatalf("cancel should return nil, got %v", err)
	}
	dones := log.byType("done")
	if len(dones) != 1 || dones[0].OK == nil || *dones[0].OK {
		t.Fatalf("done = %+v", dones)
	}
	warned := false
	for _, e := range log.byType("log") {
		if e.Level == "warn" && strings.Contains(e.Message, "已取消") {
			warned = true
		}
	}
	if !warned {
		t.Fatal("missing cancel warn log")
	}
}
