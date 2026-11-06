package translator

import (
	"fmt"
	"strconv"
	"strings"
)

const chunkMarker = "待翻译条数"
const singleMarker = "只翻译下面这一条"
const reviewMarker = "最终复核"

func buildSystemPrompt(targetLang string) string {
	return "你是一位资深的日文→" + targetLang + "游戏剧本译者，译文要能直接放进游戏发布：忠实、通顺、有文学味，杜绝机翻腔。\n\n" +
		"【输出协议（最高优先级，必须严格遵守）】\n" +
		"1. 输入是若干条日文，每条以 [i] 开头（i 为行号）。你必须对每个行号各输出一行，格式为 [i] 译文，行号的数量、数字与顺序必须和输入完全一致：不合并、不拆分、不遗漏、不新增。\n" +
		"2. 每条译文必须写在同一行内（行内不得出现换行符）；一行就是一条，条与条之间只用一个换行分隔。\n" +
		"3. 只输出译文行：不要解释、不要备注、不要罗马字注音、不要 Markdown 代码块、不要前言后语。\n" +
		"4. 输入行首的 [i] 标记必须原样保留；原文中除行首标记外的其它标记（如 <0>、<1>、{name}、%s 这类占位符）必须原样保留在译文对应位置，不得改动、不得删除、不得新增。\n" +
		"5. 每条的台词引号、命令前缀由系统另行处理，你只翻译纯日文句子本身，不要自行添加引号或前缀。\n\n" +
		"【翻译要求】\n" +
		"6. 逐条翻译，忠实完整：不漏译、不增译、不概括；原文很短时译文也要同样短促。\n" +
		"7. 人名、地名、组织、招式等专有名词按通行中文写法统一，有术语表时必须采用术语表译法。\n" +
		"8. 贴合角色语气与口癖：第一人称、敬语、粗话、孩童腔、书面腔各归其位；人物之间的称呼前后一致。\n" +
		"9. 拟声拟态词、笑声、惊叫按中文游戏习惯译出，译文里不应残留任何假名。\n" +
		"10. 句式按中文语序自然重组，用" + targetLang + "，避免翻译腔。\n\n" +
		"【译文纯净度（务必执行）】\n" +
		"11. 严禁残留日语句末语气与口癖（ね、よ、な、さ、ぞ、ぜ、わ、かな、ってば等）：必须按中文语感吸收或删掉，不得音译成「呢哦呀啊」之类硬凑；反例「そうだね」不要译成「是啊呢」，要译成「是啊」或「没错」。\n" +
		"12. 严禁残留日语人称与称谓的日式说法（僕、俺、私、君、お前、さん、くん、ちゃん、様、先辈等）：一律转成中文习惯（我、老子、你、小X、阿X、前辈等），不要保留「桑、酱、君」。\n" +
		"13. 严禁把日文感叹词照搬（ええ→嗯，えっ→咦，はあ→唉，あら→哎呀，ふふ→呵呵，うん→嗯）；严禁逐字直译日语惯用表达（〜てしまった、〜ので、〜わけではない等按中文叙述习惯自然落地）。\n\n" +
		"【成人与暴力内容】\n" +
		"14. 原作含性、情色、暴力的露骨描写时照译不误：不删减、不淡化、不跳过、不用省略号含糊带过；粗话、辱骂照原强度译出，不要净化。\n"
}

func buildChunkUserPrompt(items []promptItem, terms []glossaryEntry, targetLang string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "【%s：%d，请翻译成%s】\n", chunkMarker, len(items), targetLang)
	if len(terms) > 0 {
		sb.WriteString("【术语表（必须严格采用下列译法）】\n")
		sb.WriteString(formatGlossary(terms))
		sb.WriteString("\n")
	}
	for i, it := range items {
		fmt.Fprintf(&sb, "[%d] %s\n", i+1, it.text)
	}
	fmt.Fprintf(&sb, "【最后提醒】只输出 %d 行，每行以 [行号] 开头，行号依次是", len(items))
	for i := range items {
		if i > 0 {
			sb.WriteString("、")
		}
		sb.WriteString(strconv.Itoa(i + 1))
	}
	sb.WriteString("。除此之外不得有任何字符。")
	return sb.String()
}

func buildSingleUserPrompt(text string, terms []glossaryEntry, targetLang string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "【%s，翻译成%s】\n", singleMarker, targetLang)
	if len(terms) > 0 {
		sb.WriteString("【术语表（必须严格采用下列译法）】\n")
		sb.WriteString(formatGlossary(terms))
		sb.WriteString("\n")
	}
	sb.WriteString("[1] " + text + "\n")
	sb.WriteString("【最后提醒】只输出 1 行，以 [1] 开头。除此之外不得有任何字符。")
	return sb.String()
}

func buildReviewUserPrompt(orig string, current string, prev []string, next []string, targetLang string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "【%s：下面这条%s译文可能残留了日文，请复核】\n", reviewMarker, targetLang)
	sb.WriteString("原始日文：\n" + orig + "\n")
	sb.WriteString("当前译文：\n" + current + "\n")
	if len(prev) > 0 {
		sb.WriteString("上文（仅供理解语境，不要翻译）：\n" + strings.Join(prev, "\n") + "\n")
	}
	if len(next) > 0 {
		sb.WriteString("下文（仅供理解语境，不要翻译）：\n" + strings.Join(next, "\n") + "\n")
	}
	sb.WriteString("【要求】只输出最终译文一行（不要行号、不要解释、不要残留任何假名）；如果当前译文已经没有问题，原样输出它；如果确实无需改动，输出 KEEP。")
	return sb.String()
}

type promptItem struct {
	text string
}

func parseChunkResponse(resp string, n int) (map[int]string, []int, []int) {
	got := make(map[int]string)
	order := make([]int, 0, n)
	last := 0
	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		idx, body, ok := parseNumberedLine(line)
		if !ok {
			if last > 0 {
				got[last] = got[last] + strings.TrimSpace(line)
			}
			continue
		}
		if _, dup := got[idx]; !dup {
			order = append(order, idx)
		}
		if prev, dup := got[idx]; dup {
			got[idx] = prev + body
		} else {
			got[idx] = body
		}
		last = idx
	}
	_ = order
	var missing []int
	for i := 1; i <= n; i++ {
		if _, ok := got[i]; !ok {
			missing = append(missing, i)
		}
	}
	var extra []int
	for idx := range got {
		if idx < 1 || idx > n {
			extra = append(extra, idx)
		}
	}
	for _, idx := range extra {
		delete(got, idx)
	}
	return got, missing, extra
}

func parseNumberedLine(line string) (int, string, bool) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "[") {
		return 0, "", false
	}
	end := strings.Index(t, "]")
	if end <= 1 {
		return 0, "", false
	}
	num, err := strconv.Atoi(strings.TrimSpace(t[1:end]))
	if err != nil || num <= 0 {
		return 0, "", false
	}
	body := strings.TrimSpace(t[end+1:])
	return num, body, true
}

func stripNumberPrefix(s string) string {
	if idx, body, ok := parseNumberedLine(s); ok && idx >= 0 {
		return body
	}
	return s
}

func isKana(r rune) bool {
	return (r >= 0x3040 && r <= 0x309F) || (r >= 0x30A0 && r <= 0x30FF)
}

func looksJapanese(s string) bool {
	for _, r := range s {
		if isKana(r) {
			return true
		}
	}
	return false
}

func containsJapanese(s string) bool {
	return looksJapanese(s)
}
