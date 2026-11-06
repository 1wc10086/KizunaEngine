package translator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kizuna/kernel/core"
	"kizuna/kernel/core/textcodec"
	"kizuna/kernel/translator/providers"
)

type transJob struct {
	unitIdx int
	id      string
	orig    string
	prefix  string
	suffix  string
}

type pipe struct {
	cfg           resolvedConfig
	provider      providers.Provider
	stats         *runStats
	system        string
	send          func(Event)
	glossaryTerms int
}

func (Runner) Run(ctx context.Context, req Request, emit func(Event)) error {
	var emitMu sync.Mutex
	send := func(ev Event) {
		emitMu.Lock()
		defer emitMu.Unlock()
		emit(ev)
	}
	cfg, err := normalizeRequest(req)
	if err != nil {
		send(Event{Type: "error", Message: err.Error()})
		ok := false
		send(Event{Type: "done", OK: &ok, ElapsedMs: 0})
		return err
	}
	p := &pipe{cfg: cfg, stats: newRunStats(), send: send}
	p.system = buildSystemPrompt(cfg.targetLang)
	return p.run(ctx, req)
}

func (p *pipe) fail(err error) error {
	p.send(Event{Type: "error", Message: err.Error()})
	ok := false
	_, _, _, _, doneUnits := p.stats.snapshot()
	p.send(Event{Type: "done", OK: &ok, Translated: doneUnits, ElapsedMs: time.Since(p.stats.start).Milliseconds()})
	return err
}

func (p *pipe) cancel() error {
	p.send(Event{Type: "log", Level: "warn", Message: "已取消"})
	ok := false
	_, _, _, _, doneUnits := p.stats.snapshot()
	p.send(Event{Type: "done", OK: &ok, Translated: doneUnits, ElapsedMs: time.Since(p.stats.start).Milliseconds()})
	return nil
}

func (p *pipe) run(ctx context.Context, req Request) error {
	if err := ctx.Err(); err != nil {
		return p.cancel()
	}
	f, ok := core.Find(req.Feature)
	if !ok {
		return p.fail(fmt.Errorf("translator: unknown feature %q", req.Feature))
	}
	codec, ok := f.(core.Codec)
	if !ok {
		return p.fail(fmt.Errorf("translator: feature %q is not a codec", req.Feature))
	}
	data, err := os.ReadFile(req.Source)
	if err != nil {
		return p.fail(fmt.Errorf("translator: read source: %w", err))
	}
	doc, err := codec.Decode(data, filepath.Base(req.Source))
	if err != nil {
		return p.fail(fmt.Errorf("translator: decode: %w", err))
	}
	sourceEnc := p.cfg.sourceEnc
	if sourceEnc == "" || sourceEnc == "auto" {
		sourceEnc = doc.Encoding
	}
	p.send(Event{Type: "log", Level: "info", Message: fmt.Sprintf("解码完成：%s（feature=%s，编码=%s），共 %d 个单元", req.Source, req.Feature, sourceEnc, len(doc.Units))})
	jobs := collectJobs(doc)
	p.send(Event{Type: "log", Level: "info", Message: fmt.Sprintf("可翻译条目 %d/%d（Kind 非 raw 且含日文假名）", len(jobs), len(doc.Units))})
	if len(jobs) == 0 {
		p.send(Event{Type: "log", Level: "info", Message: "无可翻译条目，直接按目标编码重新输出"})
		if err := p.writeBack(codec, doc, req.Target); err != nil {
			return p.fail(err)
		}
		doneOK := true
		p.send(Event{Type: "done", OK: &doneOK, OutputPath: req.Target, Translated: 0, Skipped: len(doc.Units), ElapsedMs: time.Since(p.stats.start).Milliseconds(), GlossaryTerms: len(p.cfg.glossary)})
		return nil
	}
	provider, err := ProviderFactory(p.cfg.provider)
	if err != nil {
		return p.fail(fmt.Errorf("translator: provider: %w", err))
	}
	p.provider = provider
	p.resolveAutoGlossary(ctx, doc)
	chunks := chunkIndexes(len(jobs), p.cfg.chunkSize)
	p.send(Event{Type: "log", Level: "info", Message: fmt.Sprintf("开始翻译：%d 条，分 %d 块（每块 %d 条），并发 %d", len(jobs), len(chunks), p.cfg.chunkSize, p.cfg.concurrency)})
	stopTick := p.startStatsTicker()
	defer stopTick()
	results := make([]string, len(jobs))
	skipped := make([]bool, len(jobs))
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	ch := make(chan int)
	var firstErr error
	var errMu sync.Mutex
	for w := 0; w < p.cfg.concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ci := range ch {
				if runCtx.Err() != nil {
					return
				}
				if err := p.translateChunk(runCtx, chunks[ci], jobs, results, skipped); err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
					cancel()
					return
				}
				_, _, _, _, done := p.stats.snapshot()
				total := len(jobs)
				var percent float64
				if total > 0 {
					percent = float64(done) * 100 / float64(total)
				}
				p.send(Event{Type: "progress", Current: done, Total: total, Percent: percent})
				p.send(Event{Type: "log", Level: "info", Message: fmt.Sprintf("[%d/%d] 块完成：累计 %d/%d 条", ci+1, len(chunks), done, total)})
			}
		}()
	}
	go func() {
		defer close(ch)
		for ci := range chunks {
			select {
			case <-runCtx.Done():
				return
			case ch <- ci:
			}
		}
	}()
	wg.Wait()
	if ctx.Err() != nil {
		stopTick()
		return p.cancel()
	}
	if firstErr != nil {
		stopTick()
		return p.fail(firstErr)
	}
	if err := p.finalReview(runCtx, jobs, results, skipped); err != nil {
		stopTick()
		return p.fail(err)
	}
	translated := 0
	for ji, jb := range jobs {
		if skipped[ji] {
			continue
		}
		t := results[ji]
		if strings.TrimSpace(t) == "" {
			skipped[ji] = true
			continue
		}
		codec.SetUnitText(doc, jb.id, jb.prefix+t+jb.suffix)
		translated++
	}
	if err := p.writeBack(codec, doc, req.Target); err != nil {
		stopTick()
		return p.fail(err)
	}
	stopTick()
	p.send(p.stats.event())
	doneOK := true
	p.send(Event{Type: "done", OK: &doneOK, OutputPath: req.Target, Translated: translated, Skipped: len(doc.Units) - translated, ElapsedMs: time.Since(p.stats.start).Milliseconds(), GlossaryTerms: p.glossaryTerms})
	p.send(Event{Type: "log", Level: "info", Message: fmt.Sprintf("翻译完成：已译 %d 条，跳过 %d 条，输出 %s", translated, len(doc.Units)-translated, req.Target)})
	return nil
}

func collectJobs(doc *core.Document) []transJob {
	var jobs []transJob
	if doc == nil {
		return nil
	}
	for i, u := range doc.Units {
		if u.Kind == "raw" {
			continue
		}
		if !containsJapanese(u.Text) {
			continue
		}
		jobs = append(jobs, transJob{unitIdx: i, id: u.ID, orig: u.Text, prefix: u.Prefix, suffix: u.Suffix})
	}
	return jobs
}

func (p *pipe) chat(ctx context.Context, user string, maxTokens int) (providers.Response, error) {
	resp, err := p.provider.Chat(ctx, providers.Request{
		System:      p.system,
		User:        user,
		Temperature: p.cfg.temperature,
		MaxTokens:   maxTokens,
	})
	if err != nil {
		p.stats.addFailed()
		return providers.Response{}, err
	}
	p.stats.addRequest(resp.PromptTokens, resp.CompletionTokens)
	return resp, nil
}

func (p *pipe) singleTranslate(ctx context.Context, orig string) (string, error) {
	terms := matchGlossary(p.cfg.glossary, []string{orig})
	resp, err := p.chat(ctx, buildSingleUserPrompt(orig, terms, p.cfg.targetLang), p.cfg.singleTokens)
	if err != nil {
		return "", err
	}
	got, missing, _ := parseChunkResponse(resp.Text, 1)
	if len(missing) > 0 {
		return "", fmt.Errorf("translator: single retry missing line")
	}
	return strings.TrimSpace(got[1]), nil
}

func (p *pipe) translateChunk(ctx context.Context, chunk []int, jobs []transJob, results []string, skipped []bool) error {
	items := make([]promptItem, 0, len(chunk))
	texts := make([]string, 0, len(chunk))
	for _, ji := range chunk {
		items = append(items, promptItem{text: jobs[ji].orig})
		texts = append(texts, jobs[ji].orig)
	}
	terms := matchGlossary(p.cfg.glossary, texts)
	resp, err := p.chat(ctx, buildChunkUserPrompt(items, terms, p.cfg.targetLang), p.cfg.chunkTokens)
	if err != nil {
		return err
	}
	got, missing, _ := parseChunkResponse(resp.Text, len(chunk))
	for _, m := range missing {
		ji := chunk[m-1]
		retry, rerr := p.singleTranslate(ctx, jobs[ji].orig)
		if rerr != nil || strings.TrimSpace(retry) == "" {
			skipped[ji] = true
			results[ji] = jobs[ji].orig
			p.send(Event{Type: "log", Level: "warn", Message: fmt.Sprintf("行号 %d 缺失且单独重试失败，保留原文（unit=%s）", m, jobs[ji].id)})
			continue
		}
		got[m] = retry
	}
	for pos, ji := range chunk {
		t, ok := got[pos+1]
		if !ok {
			continue
		}
		t = strings.TrimSpace(t)
		for tries := 0; looksJapanese(t) && tries < p.cfg.retryLimit; tries++ {
			retry, rerr := p.singleTranslate(ctx, jobs[ji].orig)
			if rerr != nil {
				return rerr
			}
			if strings.TrimSpace(retry) != "" {
				t = strings.TrimSpace(retry)
			}
		}
		results[ji] = t
	}
	p.stats.addDoneUnits(len(chunk))
	return nil
}

func (p *pipe) finalReview(ctx context.Context, jobs []transJob, results []string, skipped []bool) error {
	var residual []int
	for ji := range jobs {
		if skipped[ji] {
			continue
		}
		if looksJapanese(results[ji]) {
			residual = append(residual, ji)
		}
	}
	if len(residual) == 0 {
		return nil
	}
	p.send(Event{Type: "residual", Count: len(residual), Round: 1})
	p.send(Event{Type: "log", Level: "warn", Message: fmt.Sprintf("残留日文 %d 条，进入最终通检", len(residual))})
	for _, ji := range residual {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var prev, next []string
		for k := ji - 1; k >= 0 && len(prev) < p.cfg.contextLines; k-- {
			prev = append([]string{jobs[k].orig}, prev...)
		}
		for k := ji + 1; k < len(jobs) && len(next) < p.cfg.contextLines; k++ {
			next = append(next, jobs[k].orig)
		}
		resp, err := p.chat(ctx, buildReviewUserPrompt(jobs[ji].orig, results[ji], prev, next, p.cfg.targetLang), p.cfg.reviewTokens)
		if err != nil {
			p.send(Event{Type: "log", Level: "warn", Message: fmt.Sprintf("最终通检失败（unit=%s）：%v，保留当前译文", jobs[ji].id, err)})
			continue
		}
		out := firstLine(strings.TrimSpace(resp.Text))
		if out == "" || out == reviewMarker {
			continue
		}
		if out == "KEEP" {
			continue
		}
		results[ji] = stripNumberPrefix(out)
	}
	still := 0
	for _, ji := range residual {
		if !skipped[ji] && looksJapanese(results[ji]) {
			still++
		}
	}
	if still > 0 {
		p.send(Event{Type: "residual", Count: still, Round: 2})
		p.send(Event{Type: "log", Level: "warn", Message: fmt.Sprintf("最终通检后仍残留日文 %d 条，保留译文不阻塞完成", still)})
	}
	return nil
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if t != "" {
			return t
		}
	}
	return ""
}

func (p *pipe) writeBack(codec core.Codec, doc *core.Document, target string) error {
	te := p.cfg.targetEnc
	switch te {
	case "", "auto":
	default:
		doc.Encoding = normalizeTargetEncoding(te)
		p.send(Event{Type: "log", Level: "info", Message: fmt.Sprintf("目标编码：%s（源编码 %s）", doc.Encoding, p.cfg.sourceEnc)})
	}
	out, err := codec.Encode(doc)
	if err != nil {
		return fmt.Errorf("translator: encode: %w", err)
	}
	if err := core.AtomicWrite(target, out); err != nil {
		return fmt.Errorf("translator: write target: %w", err)
	}
	return nil
}

func normalizeTargetEncoding(te string) string {
	switch te {
	case "utf8", "utf-8", "utf16", "utf16le", "utf-16", "utf-16le", "cp932", "shift_jis", "shift-jis", "sjis", "gbk", "gb2312", "gb18030":
		return textcodec.CanonicalName(te)
	default:
		return te
	}
}

func (p *pipe) startStatsTicker() func() {
	done := make(chan struct{})
	var once sync.Once
	stop := func() {
		once.Do(func() { close(done) })
	}
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				p.send(p.stats.event())
			}
		}
	}()
	return stop
}
