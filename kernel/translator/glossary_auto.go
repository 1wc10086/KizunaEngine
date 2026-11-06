package translator

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"kizuna/kernel/core"
)

const maxAutoGlossaryTerms = 120

const glossaryMarker = "术语表定译"

const glossarySystemPrompt = "你是游戏剧本术语表助手，只输出术语译名行，不做任何解释。"

var kataRunPattern = regexp.MustCompile("[\u30A1-\u30F6\u30FC\u30FB\u00B7\uFF65]{2,12}")

var kanjiRunPattern = regexp.MustCompile("[\u4E00-\u9FFF\u3005]+")

var honorificPattern = regexp.MustCompile("([\u4E00-\u9FFF\u30A1-\u30F6][\u4E00-\u9FFF\u30A1-\u30F6\u3005\u30FC\u30FB]{0,11})(\u3055\u3093|\u3061\u3083\u3093|\u304F\u3093|\u541B|\u5148\u8F08|\u5148\u751F|\u69D8|\u6BBF)")

type autoCandidate struct {
	term   string
	rank   int
	freq   int
	origin string
	note   string
}

func extractAutoCandidates(doc *core.Document, user []glossaryEntry) []autoCandidate {
	userSet := map[string]bool{}
	for _, e := range user {
		userSet[e.src] = true
	}
	byTerm := map[string]*autoCandidate{}
	roleFreq := map[string]int{}
	var corpus []string
	if doc != nil {
		for _, u := range doc.Units {
			if u.Kind == "raw" {
				continue
			}
			t := strings.TrimSpace(u.Text)
			if t == "" {
				continue
			}
			corpus = append(corpus, t)
			if u.Kind == "character" || u.Kind == "name" {
				roleFreq[t]++
			}
		}
	}
	for term, n := range roleFreq {
		upsertCandidate(byTerm, term, 0, n, "role", "")
	}
	for _, text := range corpus {
		for base, full := range honorificHits(text) {
			upsertCandidate(byTerm, base, 1, 1, "role", full)
		}
	}
	kataFreq := map[string]int{}
	for _, text := range corpus {
		for _, m := range kataRunPattern.FindAllString(text, -1) {
			w := strings.Trim(m, "\u30FB\u00B7\uFF65\u30FC-")
			if utf8.RuneCountInString(w) < 2 || utf8.RuneCountInString(w) > 12 {
				continue
			}
			kataFreq[w]++
		}
	}
	for w, n := range kataFreq {
		if n < 2 {
			continue
		}
		upsertCandidate(byTerm, w, 2, n, "kata", "")
	}
	kanjiFreq := map[string]int{}
	for _, text := range corpus {
		for _, m := range kanjiRunPattern.FindAllString(text, -1) {
			n := utf8.RuneCountInString(m)
			if n < 2 || n > 6 {
				continue
			}
			kanjiFreq[m]++
		}
	}
	for w, n := range kanjiFreq {
		if n < 3 {
			continue
		}
		upsertCandidate(byTerm, w, 2, n, "kanji", "")
	}
	var out []autoCandidate
	for _, c := range byTerm {
		if userSet[c.term] {
			continue
		}
		if !keepCandidateTerm(c.term) {
			continue
		}
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].rank != out[j].rank {
			return out[i].rank < out[j].rank
		}
		if out[i].freq != out[j].freq {
			return out[i].freq > out[j].freq
		}
		ni := utf8.RuneCountInString(out[i].term)
		nj := utf8.RuneCountInString(out[j].term)
		if ni != nj {
			return ni > nj
		}
		return out[i].term < out[j].term
	})
	if len(out) > maxAutoGlossaryTerms {
		out = out[:maxAutoGlossaryTerms]
	}
	return out
}

func upsertCandidate(byTerm map[string]*autoCandidate, term string, rank int, n int, origin string, note string) {
	c := byTerm[term]
	if c == nil {
		byTerm[term] = &autoCandidate{term: term, rank: rank, freq: n, origin: origin, note: note}
		return
	}
	c.freq += n
	if rank < c.rank {
		c.rank = rank
		c.origin = origin
	}
	if c.note == "" {
		c.note = note
	}
}

func honorificHits(text string) map[string]string {
	out := map[string]string{}
	for _, m := range honorificPattern.FindAllStringSubmatch(text, -1) {
		if len(m) != 3 {
			continue
		}
		base := strings.Trim(m[1], "\u30FB\u00B7\uFF65\u30FC- \u3000")
		if base == "" {
			continue
		}
		if _, dup := out[base]; !dup {
			out[base] = m[0]
		}
	}
	return out
}

func keepCandidateTerm(term string) bool {
	t := strings.TrimSpace(term)
	if t == "" {
		return false
	}
	if strings.ContainsAny(t, "=\uFF1D\n\r") {
		return false
	}
	allASCII := true
	hasLetter := false
	for _, r := range t {
		if r >= 128 {
			allASCII = false
		}
		if unicode.IsLetter(r) {
			hasLetter = true
		}
	}
	if allASCII || !hasLetter {
		return false
	}
	return true
}

func countCandidateOrigins(cands []autoCandidate) (role int, kata int, kanji int) {
	for _, c := range cands {
		switch {
		case c.rank < 2:
			role++
		case c.origin == "kanji":
			kanji++
		default:
			kata++
		}
	}
	return role, kata, kanji
}

func buildAutoGlossaryPrompt(cands []autoCandidate) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "【%s：游戏剧本术语表共 %d 条，请给出每个术语的简体中文译名】\n", glossaryMarker, len(cands))
	sb.WriteString("这是游戏剧本术语表，请给出简体中文译名，同一术语只出现一次，格式严格为 `原文=译名`，每行一条，不要解释、不要编号、不要罗马字。\n")
	sb.WriteString("术语：\n")
	for _, c := range cands {
		sb.WriteString(c.term + "\n")
	}
	sb.WriteString("【最后提醒】只输出译名行，每行一条 `原文=译名`。除此之外不得有任何字符。")
	return sb.String()
}

func parseAutoGlossary(text string, cands []autoCandidate) []glossaryEntry {
	candSet := map[string]bool{}
	for _, c := range cands {
		candSet[c.term] = true
	}
	seen := map[string]bool{}
	var out []glossaryEntry
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			continue
		}
		line = stripNumberPrefix(line)
		src, dst := splitAutoGlossaryLine(line)
		src = strings.TrimSpace(src)
		dst = strings.TrimSpace(dst)
		if src == "" || dst == "" {
			continue
		}
		if !candSet[src] || seen[src] {
			continue
		}
		if dst == src || strings.ContainsAny(dst, "=\uFF1D") {
			continue
		}
		if utf8.RuneCountInString(dst) > 32 {
			continue
		}
		seen[src] = true
		out = append(out, glossaryEntry{src: src, dst: dst})
	}
	return out
}

func splitAutoGlossaryLine(line string) (string, string) {
	for _, sep := range []string{"=", "\uFF1D"} {
		if idx := strings.Index(line, sep); idx >= 0 {
			return line[:idx], line[idx+len(sep):]
		}
	}
	return "", ""
}

func mergeAutoGlossary(autoEntries []glossaryEntry, user []glossaryEntry) []glossaryEntry {
	merged := map[string]string{}
	var order []string
	for _, e := range autoEntries {
		if _, dup := merged[e.src]; !dup {
			order = append(order, e.src)
		}
		merged[e.src] = e.dst
	}
	for _, e := range user {
		if _, dup := merged[e.src]; !dup {
			order = append(order, e.src)
		}
		merged[e.src] = e.dst
	}
	out := make([]glossaryEntry, 0, len(order))
	for _, src := range order {
		out = append(out, glossaryEntry{src: src, dst: merged[src]})
	}
	return out
}

func previewAutoGlossary(entries []glossaryEntry, notes map[string]string, limitRunes int) string {
	var parts []string
	for _, e := range entries {
		s := e.src + "=" + e.dst
		if notes != nil {
			if full, ok := notes[e.src]; ok && full != "" && full != e.src {
				s = e.src + "（" + full + "）=" + e.dst
			}
		}
		parts = append(parts, s)
	}
	joined := strings.Join(parts, "；")
	rs := []rune(joined)
	if len(rs) > limitRunes {
		joined = string(rs[:limitRunes]) + "…"
	}
	return joined
}

func (p *pipe) resolveAutoGlossary(ctx context.Context, doc *core.Document) {
	userEntries := p.cfg.glossary
	cands := extractAutoCandidates(doc, userEntries)
	role, kata, kanji := countCandidateOrigins(cands)
	p.send(Event{Type: "log", Level: "info", Message: fmt.Sprintf("自动术语表：候选 %d 条（角色名 %d / 片假名 %d / 汉字词 %d）", len(cands), role, kata, kanji)})
	if len(cands) < 2 {
		p.send(Event{Type: "log", Level: "info", Message: fmt.Sprintf("自动术语表：候选不足（%d 条），跳过模型确认", len(cands))})
		p.glossaryTerms = len(userEntries)
		return
	}
	maxTokens := len(cands)*32 + 512
	if maxTokens < 512 {
		maxTokens = 512
	}
	if maxTokens > 4096 {
		maxTokens = 4096
	}
	system := p.system
	p.system = glossarySystemPrompt
	resp, err := p.chat(ctx, buildAutoGlossaryPrompt(cands), maxTokens)
	p.system = system
	if err != nil {
		p.send(Event{Type: "log", Level: "warn", Message: fmt.Sprintf("自动术语表：模型确认失败（%v），仅使用用户术语", err)})
		p.glossaryTerms = len(userEntries)
		p.send(Event{Type: "log", Level: "info", Message: fmt.Sprintf("自动术语表：模型确认 0 条，合并用户术语 %d 条，最终 %d 条", len(userEntries), len(userEntries))})
		return
	}
	autoEntries := parseAutoGlossary(resp.Text, cands)
	merged := mergeAutoGlossary(autoEntries, userEntries)
	p.cfg.glossary = merged
	p.glossaryTerms = len(merged)
	p.send(Event{Type: "log", Level: "info", Message: fmt.Sprintf("自动术语表：模型确认 %d 条，合并用户术语 %d 条，最终 %d 条", len(autoEntries), len(userEntries), len(merged))})
	if len(merged) > 0 {
		notes := map[string]string{}
		for _, c := range cands {
			if c.note != "" {
				notes[c.term] = c.note
			}
		}
		head := merged
		if len(head) > 8 {
			head = head[:8]
		}
		p.send(Event{Type: "log", Level: "info", Message: "自动术语表预览：" + previewAutoGlossary(head, notes, 200)})
	}
}
