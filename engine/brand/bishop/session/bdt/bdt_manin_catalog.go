package bdt

import (
	"strconv"
	"strings"

	"kizuna/engine/core/textcodec"
)

// bdt_manin_catalog.go 解析 atktbl.bdt（責め言葉表）并实现词语库存/反应计算。
//
// 这个文件和 Session 无关：脚本、言葉責め、整理画面都只依赖这里的数据结构与
// 纯函数，方便单独测试，也避免把「一条台词值多少興奮」散落在 UI 代码里。

const maninWordTableName = "atktbl"

// loadManinWordCatalog 读取 atktbl.bdt。文件整份按 0xFF 异或加密（和
// scene00.bdt 相同），明文是 CP932 表：
//
//	[ 1]a01,B,B,C,E,E,D,言葉...
//
// 六个字母依次对应六名女主角（X29=1..6），不是六种反应。
func loadManinWordCatalog(data []byte) *maninWordCatalog {
	catalog := &maninWordCatalog{byID: map[int]int{}}
	if len(data) == 0 {
		return catalog
	}
	decrypted := make([]byte, len(data))
	for i, value := range data {
		decrypted[i] = ^value
	}
	text := textcodec.DecodeCP932(decrypted)
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") {
			continue
		}
		closeBracket := strings.IndexByte(line, ']')
		if closeBracket < 0 {
			continue
		}
		parts := strings.SplitN(line[closeBracket+1:], ",", 8)
		if len(parts) < 8 {
			continue
		}
		idText := strings.TrimSpace(parts[0])
		if len(idText) < 2 || (idText[0] != 'a' && idText[0] != 'A') {
			continue
		}
		id, err := strconv.Atoi(idText[1:])
		if err != nil || id <= 0 {
			continue
		}
		word := maninWord{ID: id, Text: strings.TrimSpace(parts[7])}
		for i := 0; i < 6; i++ {
			grade := byte('C')
			if len(parts[1+i]) > 0 {
				grade = parts[1+i][0]
			}
			word.Grades[i] = grade
		}
		word.Rank = maninRankOf(word.Grades)
		catalog.byID[id] = len(catalog.Words)
		catalog.Words = append(catalog.Words, word)
	}
	return catalog
}

// maninRankOf 把六人反应折成显示等级 II..V。
// 原作只说「V 平均好反应、II 平均差」，没有单独等级表；A=5..E=1 的平均值
// 是最直接且不会和实际反应脱节的算法。
func maninRankOf(grades [6]byte) int {
	total := 0
	for _, grade := range grades {
		switch grade {
		case 'A':
			total += 5
		case 'B':
			total += 4
		case 'C':
			total += 3
		case 'D':
			total += 2
		default:
			total++
		}
	}
	average := float64(total) / 6.0
	switch {
	case average >= 4.5:
		return 5
	case average >= 3.5:
		return 4
	case average >= 2.5:
		return 3
	default:
		return 2
	}
}

func (c *maninWordCatalog) word(id int) (maninWord, bool) {
	if c == nil {
		return maninWord{}, false
	}
	index, ok := c.byID[id]
	if !ok || index < 0 || index >= len(c.Words) {
		return maninWord{}, false
	}
	return c.Words[index], true
}

// expandManinWords 把剧本里的 <nn> 标记替换成責め言葉全文，并返回本条台词
// 收集到的词 ID。原作用粉色显示这些台词；颜色由调用方根据 hasWord 决定。
func expandManinWords(text string, catalog *maninWordCatalog) (string, []int) {
	if len(text) == 0 || !strings.ContainsRune(text, '<') {
		return text, nil
	}
	var out strings.Builder
	var ids []int
	for i := 0; i < len(text); {
		if text[i] != '<' {
			out.WriteByte(text[i])
			i++
			continue
		}
		end := strings.IndexByte(text[i+1:], '>')
		if end < 0 {
			out.WriteString(text[i:])
			break
		}
		end += i + 1
		id, err := strconv.Atoi(text[i+1 : end])
		if err != nil {
			out.WriteString(text[i : end+1])
			i = end + 1
			continue
		}
		word, ok := catalog.word(id)
		if !ok {
			out.WriteString(text[i : end+1])
			i = end + 1
			continue
		}
		out.WriteString(word.Text)
		ids = append(ids, id)
		i = end + 1
	}
	return out.String(), ids
}

func (u *maninRuntime) wordText(id int) string {
	if u == nil {
		return ""
	}
	word, ok := u.catalog.word(id)
	if !ok {
		return ""
	}
	return word.Text
}

func (u *maninRuntime) wordRank(id int) int {
	if u == nil {
		return 2
	}
	word, ok := u.catalog.word(id)
	if !ok {
		return 2
	}
	return word.Rank
}

// collectWord 把台词里读到的词放进临时记忆。满 21 条时覆盖最旧的一条；
// 原作就是这样限制「メモリー欄」，逼玩家回家整理。
func (u *maninRuntime) collectWord(id int) {
	if u == nil || id <= 0 {
		return
	}
	if _, ok := u.catalog.word(id); !ok {
		return
	}
	for _, existing := range u.memory {
		if existing == id {
			return
		}
	}
	if len(u.memory) >= maninMemoryCap {
		u.memory = append(u.memory[:0], u.memory[1:]...)
	}
	u.memory = append(u.memory, id)
}

func (u *maninRuntime) inBank(id int) bool {
	for _, existing := range u.words {
		if existing == id {
			return true
		}
	}
	return false
}

func (u *maninRuntime) removeMemory(id int) {
	kept := u.memory[:0]
	for _, existing := range u.memory {
		if existing != id {
			kept = append(kept, existing)
		}
	}
	u.memory = kept
}

// consumeWord 在台词真正说出口后从库存移除。原作「ネタ切れ」是失败条件，
// 所以词不能被无限重复使用；整理画面里保存的词也一样消耗。
func (u *maninRuntime) consumeWord(id int) {
	if u == nil {
		return
	}
	kept := u.words[:0]
	for _, existing := range u.words {
		if existing != id {
			kept = append(kept, existing)
		}
	}
	u.words = kept
	u.removeMemory(id)
}

// saveWord 把临时记忆里的词写进データ欄。返回 false 表示需要覆盖模式。
func (u *maninRuntime) saveWord(id int) bool {
	if u == nil || id <= 0 || u.inBank(id) {
		return true
	}
	if len(u.words) >= maninWordBankCap {
		return false
	}
	u.words = append(u.words, id)
	return true
}

// attackPool 是言葉責め可选的词：保存栏优先，临时记忆补在后面。原作允许
// 从「記憶・保存された一覧」里选，重复的词只显示一次。
func (u *maninRuntime) attackPool() []int {
	if u == nil {
		return nil
	}
	seen := map[int]bool{}
	out := make([]int, 0, maninWordBankCap+maninMemoryCap)
	for _, id := range u.words {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	for _, id := range u.memory {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// applyReaction 按原作五档返回兴奋/压力增量：
// A 最高 +6 兴奋，B 優良 +4 兴奋，C 普通 +2/+2，
// D 嫌悪 +2/+4，E 最悪 +6 压力。target 为 1..6。
func (u *maninRuntime) applyReaction(id, target int) (int, int) {
	if u == nil || target < 1 || target > 6 {
		return 0, 0
	}
	word, ok := u.catalog.word(id)
	if !ok {
		return 0, 0
	}
	switch word.Grades[target-1] {
	case 'A':
		return 6, 0
	case 'B':
		return 4, 0
	case 'C':
		return 2, 2
	case 'D':
		return 2, 4
	default:
		return 0, 6
	}
}
