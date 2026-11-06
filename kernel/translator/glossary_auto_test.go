package translator

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"kizuna/kernel/core"
	"kizuna/kernel/translator/providers"
)

type autoFake struct {
	mu           sync.Mutex
	glossCalls   int
	glossUsers   []string
	glossResp    func(user string) string
	glossErr     error
	chunkCalls   int
	chunkPrompts []string
}

func (f *autoFake) Chat(ctx context.Context, req providers.Request) (providers.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Contains(req.User, glossaryMarker) {
		f.glossCalls++
		f.glossUsers = append(f.glossUsers, req.User)
		if f.glossErr != nil {
			return providers.Response{}, f.glossErr
		}
		return providers.Response{Text: f.glossResp(req.User)}, nil
	}
	f.chunkCalls++
	f.chunkPrompts = append(f.chunkPrompts, req.User)
	var sb strings.Builder
	for _, line := range strings.Split(req.User, "\n") {
		idx, _, ok := parseNumberedLine(line)
		if !ok {
			continue
		}
		sb.WriteString("[" + strconv.Itoa(idx) + "] 中文译文" + strconv.Itoa(idx) + "\n")
	}
	return providers.Response{Text: sb.String()}, nil
}

func withAutoFake(t *testing.T, f *autoFake) {
	t.Helper()
	old := ProviderFactory
	ProviderFactory = func(providers.Spec) (providers.Provider, error) {
		return f, nil
	}
	t.Cleanup(func() { ProviderFactory = old })
}

func autoTestDoc() *core.Document {
	units := []core.Unit{
		{ID: "character/0", Kind: "character", Text: "タカシ"},
		{ID: "character/1", Kind: "character", Text: "タカシ"},
		{ID: "name/0", Kind: "name", Text: "リナ"},
		{ID: "name/1", Kind: "name", Text: "12345"},
		{ID: "character/2", Kind: "character", Text: "ABC"},
		{ID: "message/0", Kind: "message", Text: "タカシさんとドラゴンが魔王城で戦った"},
		{ID: "message/1", Kind: "message", Text: "ドラゴンとゴーレムは魔王城に戻った"},
		{ID: "message/2", Kind: "message", Text: "ゴーレムは魔王城を守るドラゴンだ"},
		{ID: "message/3", Kind: "message", Text: "リナは王国から魔王城へ向かった"},
		{ID: "message/4", Kind: "message", Text: "王国の騎士が王国を出た"},
	}
	return &core.Document{Feature: "bishop.bdt", Format: "bdt", Units: units}
}

func TestAutoExtractCandidates(t *testing.T) {
	user := parseGlossary("ドラゴン=飞龙\nリナ=莉娜")
	cands := extractAutoCandidates(autoTestDoc(), user)
	var terms []string
	for _, c := range cands {
		terms = append(terms, c.term)
	}
	got := map[string]bool{}
	for _, s := range terms {
		got[s] = true
	}
	for _, s := range []string{"タカシ", "魔王城", "王国", "ゴーレム"} {
		if !got[s] {
			t.Fatalf("missing candidate %q in %v", s, terms)
		}
	}
	for _, s := range []string{"ドラゴン", "リナ", "12345", "ABC", "騎士"} {
		if got[s] {
			t.Fatalf("candidate %q should be excluded, got %v", s, terms)
		}
	}
	if len(cands) == 0 || cands[0].term != "タカシ" {
		t.Fatalf("role name should sort first, got %v", terms)
	}
	role, kata, kanji := countCandidateOrigins(cands)
	if role != 1 || kata != 1 || kanji != 2 {
		t.Fatalf("origins = %d/%d/%d, want 1/1/2 (terms=%v)", role, kata, kanji, terms)
	}
	if len(cands) != 4 {
		t.Fatalf("candidates = %v, want 4", terms)
	}
	found := false
	for _, c := range cands {
		if c.term == "タカシ" && c.note == "タカシさん" {
			found = true
		}
	}
	if !found {
		t.Fatalf("honorific pattern not recorded for タカシ: %+v", cands)
	}
}

func TestHonorificHits(t *testing.T) {
	hits := honorificHits("山田先生と佐藤ちゃんが鈴木くんを呼んだ")
	if hits["山田"] != "山田先生" {
		t.Fatalf("hits=%v", hits)
	}
	if hits["佐藤"] != "佐藤ちゃん" {
		t.Fatalf("hits=%v", hits)
	}
	if hits["鈴木"] != "鈴木くん" {
		t.Fatalf("hits=%v", hits)
	}
	if len(honorificHits("今日はいい天気ですね")) != 0 {
		t.Fatalf("plain sentence should have no hits")
	}
	if len(honorificHits("123さん")) != 0 {
		t.Fatalf("digits should not match honorific base")
	}
}

func TestAutoExtractCap(t *testing.T) {
	var units []core.Unit
	for i := 0; i < 200; i++ {
		w := "テスト" + string(rune(0x30A2+i/46)) + string(rune(0x30A2+i%46))
		units = append(units, core.Unit{ID: "m/" + strconv.Itoa(i*2), Kind: "message", Text: "あの" + w + "が来た"})
		units = append(units, core.Unit{ID: "m/" + strconv.Itoa(i*2+1), Kind: "message", Text: w + "は強い"})
	}
	cands := extractAutoCandidates(&core.Document{Units: units}, nil)
	if len(cands) != maxAutoGlossaryTerms {
		t.Fatalf("capped candidates = %d, want %d", len(cands), maxAutoGlossaryTerms)
	}
}

func TestParseAutoGlossary(t *testing.T) {
	cands := []autoCandidate{{term: "ドラゴン"}, {term: "魔王城"}, {term: "ゴーレム"}}
	text := "ドラゴン=龙\n[1] 魔王城=魔王城堡\n未知語=乱入\n壊れた行\nゴーレム=魔像\nドラゴン=二重回答\n空=\n自分=自分\n"
	got := parseAutoGlossary(text, cands)
	if len(got) != 3 {
		t.Fatalf("parsed=%+v, want 3", got)
	}
	m := map[string]string{}
	for _, e := range got {
		m[e.src] = e.dst
	}
	if m["ドラゴン"] != "龙" || m["魔王城"] != "魔王城堡" || m["ゴーレム"] != "魔像" {
		t.Fatalf("parsed=%+v", got)
	}
}

func TestAutoGlossarySingleCall(t *testing.T) {
	fake := &autoFake{
		glossResp: func(user string) string {
			return "タカシ=塔卡西\nリナ=丽娜\nドラゴン=龙\n魔王城=魔王城堡\n王国=王国之地\nゴーレム=魔像\n"
		},
	}
	withAutoFake(t, fake)
	log := &eventLog{}
	p := &pipe{cfg: resolvedConfig{targetLang: "简体中文", temperature: 0}, stats: newRunStats(), send: log.emit}
	p.provider = fake
	p.resolveAutoGlossary(context.Background(), autoTestDoc())
	fake.mu.Lock()
	glossCalls := fake.glossCalls
	var user string
	if len(fake.glossUsers) > 0 {
		user = fake.glossUsers[0]
	}
	fake.mu.Unlock()
	if glossCalls != 1 {
		t.Fatalf("glossary calls = %d, want 1", glossCalls)
	}
	for _, s := range []string{"タカシ", "リナ", "魔王城", "王国", "ゴーレム", "原文=译名"} {
		if !strings.Contains(user, s) {
			t.Fatalf("glossary request missing %q", s)
		}
	}
	if p.glossaryTerms != 6 {
		t.Fatalf("glossaryTerms = %d, want 6 (entries=%+v)", p.glossaryTerms, p.cfg.glossary)
	}
	found := false
	for _, e := range log.byType("log") {
		if strings.Contains(e.Message, "自动术语表：候选 6 条（角色名 2 / 片假名 2 / 汉字词 2）") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing candidate log: %+v", log.byType("log"))
	}
	found = false
	for _, e := range log.byType("log") {
		if strings.Contains(e.Message, "自动术语表：模型确认 6 条，合并用户术语 0 条，最终 6 条") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing merge log: %+v", log.byType("log"))
	}
}

func autoBDTLines() []string {
	return []string{
		"\"タカシさんはドラゴンと魔王城で戦った",
		"\"ドラゴンとゴーレムは魔王城に戻った",
		"\"ゴーレムは魔王城を守るドラゴンだ",
		"\"リナは王国からやってきた",
		"\"王国の騎士が王国を守る",
	}
}

func runAutoBDT(t *testing.T, fake *autoFake, userGlossary string) (string, *eventLog) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "scene.bdt")
	dst := filepath.Join(dir, "scene.out.bdt")
	if err := os.WriteFile(src, buildBDT(t, autoBDTLines()), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	withAutoFake(t, fake)
	log := &eventLog{}
	req := baseRequest(src, dst)
	req.Params.Glossary = userGlossary
	req.Params.ChunkSize = 2
	if err := (Runner{}).Run(context.Background(), req, log.emit); err != nil {
		t.Fatalf("run: %v", err)
	}
	return dst, log
}

func chunkTermHits(prompt string) (srcs []string, texts []string) {
	inTerms := false
	for _, line := range strings.Split(prompt, "\n") {
		if strings.Contains(line, "术语表") {
			inTerms = true
			continue
		}
		if inTerms {
			if idx, _, ok := parseNumberedLine(line); ok && idx >= 1 {
				inTerms = false
			} else {
				t := strings.TrimSpace(line)
				if t == "" {
					continue
				}
				if i := strings.Index(t, "＝"); i >= 0 {
					srcs = append(srcs, t[:i])
				}
				continue
			}
		}
		if _, body, ok := parseNumberedLine(line); ok {
			texts = append(texts, body)
		}
	}
	return srcs, texts
}

func checkChunkHits(t *testing.T, prompts []string) int {
	t.Helper()
	total := 0
	for _, pr := range prompts {
		srcs, texts := chunkTermHits(pr)
		for _, s := range srcs {
			hit := false
			for _, tx := range texts {
				if strings.Contains(tx, s) {
					hit = true
					break
				}
			}
			if !hit {
				t.Fatalf("term %q not in chunk texts %v", s, texts)
			}
			total++
		}
	}
	return total
}

func TestAutoGlossaryUserOverride(t *testing.T) {
	fake := &autoFake{
		glossResp: func(user string) string {
			return "タカシ=塔卡西\nドラゴン=龙\n魔王城=魔王城堡\n王国=王国之地\nゴーレム=魔像\nリナ=丽娜\n"
		},
	}
	dst, log := runAutoBDT(t, fake, "ゴーレム=石头人")
	fake.mu.Lock()
	prompts := append([]string{}, fake.chunkPrompts...)
	glossCalls := fake.glossCalls
	fake.mu.Unlock()
	if glossCalls != 1 {
		t.Fatalf("glossary calls = %d, want 1", glossCalls)
	}
	hits := checkChunkHits(t, prompts)
	if hits == 0 {
		t.Fatalf("no glossary hits in chunk prompts")
	}
	joined := strings.Join(prompts, "\n")
	if !strings.Contains(joined, "ゴーレム＝石头人") {
		t.Fatalf("user glossary did not override auto result in prompts")
	}
	if strings.Contains(joined, "魔像") {
		t.Fatalf("auto translation should be overridden by user glossary")
	}
	dones := log.byType("done")
	if len(dones) != 1 || dones[0].OK == nil || !*dones[0].OK {
		t.Fatalf("done=%+v", dones)
	}
	if dones[0].GlossaryTerms != 5 {
		t.Fatalf("glossaryTerms=%d, want 5", dones[0].GlossaryTerms)
	}
	out, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	doc, err := bdtCodec(t).Decode(out, "scene.out.bdt")
	if err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	_ = doc
}

func TestAutoGlossaryFailureTolerant(t *testing.T) {
	fake := &autoFake{glossErr: errFakeBoom}
	dst, log := runAutoBDT(t, fake, "明里=阿明")
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("target missing after glossary failure: %v", err)
	}
	warned := false
	for _, e := range log.byType("log") {
		if e.Level == "warn" && strings.Contains(e.Message, "自动术语表") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("missing warn log for glossary failure: %+v", log.byType("log"))
	}
	dones := log.byType("done")
	if len(dones) != 1 || dones[0].OK == nil || !*dones[0].OK {
		t.Fatalf("done=%+v", dones)
	}
	if dones[0].GlossaryTerms != 1 {
		t.Fatalf("glossaryTerms=%d, want 1 (user only)", dones[0].GlossaryTerms)
	}
}

func TestAutoGlossarySkipWhenFew(t *testing.T) {
	fake := &autoFake{
		glossResp: func(user string) string { return "X=Y\n" },
	}
	withAutoFake(t, fake)
	log := &eventLog{}
	p := &pipe{cfg: resolvedConfig{targetLang: "简体中文"}, stats: newRunStats(), send: log.emit}
	p.provider = fake
	doc := &core.Document{Units: []core.Unit{{ID: "m/0", Kind: "message", Text: "こんにちは"}}}
	p.resolveAutoGlossary(context.Background(), doc)
	fake.mu.Lock()
	glossCalls := fake.glossCalls
	fake.mu.Unlock()
	if glossCalls != 0 {
		t.Fatalf("glossary calls = %d, want 0", glossCalls)
	}
	skipped := false
	for _, e := range log.byType("log") {
		if strings.Contains(e.Message, "跳过模型确认") {
			skipped = true
		}
	}
	if !skipped {
		t.Fatalf("missing skip log: %+v", log.byType("log"))
	}
}

func bdtSamplesDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("KZ_SAMPLES")
	if dir == "" {
		dir = "/root/code/Bishop/示例"
	}
	base := filepath.Join(dir, "Bdt")
	st, err := os.Stat(base)
	if err != nil || !st.IsDir() {
		t.Skip("missing samples: " + base)
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Skip("cannot read samples: " + err.Error())
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".bdt") {
			return filepath.Join(base, e.Name())
		}
	}
	t.Skip("no bdt samples in " + base)
	return ""
}

func TestAutoGlossaryEndToEndRealSample(t *testing.T) {
	sample := bdtSamplesDir(t)
	raw, err := os.ReadFile(sample)
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "real.bdt")
	dst := filepath.Join(dir, "real.out.bdt")
	if err := os.WriteFile(src, raw, 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	before, err := bdtCodec(t).Decode(raw, "real.bdt")
	if err != nil {
		t.Fatalf("decode sample: %v", err)
	}
	fake := &autoFake{
		glossResp: func(user string) string {
			var sb strings.Builder
			for _, line := range strings.Split(user, "\n") {
				s := strings.TrimSpace(line)
				if s == "" || strings.HasPrefix(s, "【") || strings.Contains(s, "术语") || strings.Contains(s, "=") {
					continue
				}
				sb.WriteString(s + "=" + s + "译\n")
			}
			return sb.String()
		},
	}
	withAutoFake(t, fake)
	log := &eventLog{}
	req := baseRequest(src, dst)
	req.Params.Glossary = ""
	req.Params.ChunkSize = 30
	if err := (Runner{}).Run(context.Background(), req, log.emit); err != nil {
		t.Fatalf("run: %v", err)
	}
	out, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	after, err := bdtCodec(t).Decode(out, "real.out.bdt")
	if err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	changed := 0
	byBefore := map[string]string{}
	for _, u := range before.Units {
		byBefore[u.ID] = u.Text
	}
	for _, u := range after.Units {
		if old, ok := byBefore[u.ID]; ok && old != u.Text {
			changed++
		}
	}
	if changed == 0 {
		t.Fatalf("no unit text changed after translation")
	}
	dones := log.byType("done")
	if len(dones) != 1 || dones[0].OK == nil || !*dones[0].OK {
		t.Fatalf("done=%+v", dones)
	}
	fake.mu.Lock()
	prompts := append([]string{}, fake.chunkPrompts...)
	glossCalls := fake.glossCalls
	fake.mu.Unlock()
	hits := checkChunkHits(t, prompts)
	t.Logf("sample=%s changed=%d glossaryTerms=%d glossCalls=%d chunkHits=%d", sample, changed, dones[0].GlossaryTerms, glossCalls, hits)
	if dones[0].GlossaryTerms > 0 {
		if glossCalls != 1 {
			t.Fatalf("glossary calls = %d, want 1", glossCalls)
		}
		if hits == 0 {
			t.Fatalf("glossary non-empty but no chunk hits")
		}
	}
}
