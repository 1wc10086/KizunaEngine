package data

import (
	"sort"
	"strings"
)

// Achievement 是 define.bsa:achievement.db 里的一条成就。
//
// 文件不是严格 JSON：值是未加引号的 TRUE/FALSE，条目间可能有空行和尾逗号。
// 它在作品之间只有 ID/名称/说明/是否隐藏四项，所以用一个专用解析器比引入
// JSON 宽松模式更可控——解析失败时整张表为空，不会让状态画面崩掉。
type Achievement struct {
	ID     string
	Name   string
	Notes  string
	Secret bool
}

// AchievementTable 按文件中出现的顺序保存成就，并提供 ID 查询。
type AchievementTable struct {
	entries []Achievement
	index   map[string]int
}

func ParseAchievementTable(text []byte) *AchievementTable {
	table := &AchievementTable{index: make(map[string]int)}
	content := string(text)
	position := 0
	for {
		keyStart := strings.IndexByte(content[position:], '"')
		if keyStart < 0 {
			break
		}
		keyStart += position
		id, afterKey, ok := readQuoted(content, keyStart)
		if !ok {
			break
		}
		position = afterKey
		if !strings.HasPrefix(id, "ID") {
			continue
		}
		colon := strings.IndexByte(content[position:], ':')
		if colon < 0 {
			break
		}
		open := strings.IndexByte(content[position+colon:], '{')
		if open < 0 {
			break
		}
		open += position + colon
		close := strings.IndexByte(content[open:], '}')
		if close < 0 {
			break
		}
		close += open
		entry := Achievement{ID: id}
		body := content[open+1 : close]
		entry.Secret = parseAchievementBool(fieldValue(body, "secret"))
		entry.Name = fieldValue(body, "name")
		entry.Notes = fieldValue(body, "notes")
		if entry.Name == "" && entry.Notes == "" {
			position = close + 1
			continue
		}
		table.index[id] = len(table.entries)
		table.entries = append(table.entries, entry)
		position = close + 1
	}
	return table
}

// Entries 返回按 ID 排序的副本：文件里作者常按角色分组，状态画面按 ID
// 顺序排列才和原引擎的编号列一致。
func (t *AchievementTable) Entries() []Achievement {
	if t == nil {
		return nil
	}
	out := make([]Achievement, len(t.entries))
	copy(out, t.entries)
	sort.SliceStable(out, func(left, right int) bool {
		return out[left].ID < out[right].ID
	})
	return out
}

func (t *AchievementTable) Get(id string) (Achievement, bool) {
	if t == nil {
		return Achievement{}, false
	}
	index, ok := t.index[id]
	if !ok {
		return Achievement{}, false
	}
	return t.entries[index], true
}

func (t *AchievementTable) Len() int {
	if t == nil {
		return 0
	}
	return len(t.entries)
}

func readQuoted(text string, start int) (string, int, bool) {
	if start >= len(text) || text[start] != '"' {
		return "", start, false
	}
	var builder strings.Builder
	for index := start + 1; index < len(text); index++ {
		switch text[index] {
		case '\\':
			if index+1 < len(text) {
				index++
				builder.WriteByte(text[index])
			}
		case '"':
			return builder.String(), index + 1, true
		default:
			builder.WriteByte(text[index])
		}
	}
	return "", len(text), false
}

// fieldValue 从一条成就的花括号体内取出字段值；字段名必须位于引号内，
// 避免把说明文本里的 "name" 之类的词误当成键。
func fieldValue(body, field string) string {
	needle := `"` + field + `"`
	index := strings.Index(body, needle)
	if index < 0 {
		return ""
	}
	rest := body[index+len(needle):]
	colon := strings.IndexByte(rest, ':')
	if colon < 0 {
		return ""
	}
	rest = strings.TrimSpace(rest[colon+1:])
	if rest == "" {
		return ""
	}
	if rest[0] == '"' {
		value, _, _ := readQuoted(rest, 0)
		return value
	}
	end := strings.IndexAny(rest, ",}\r\n")
	if end < 0 {
		end = len(rest)
	}
	return strings.TrimSpace(rest[:end])
}

func parseAchievementBool(text string) bool {
	return strings.EqualFold(strings.TrimSpace(text), "true")
}
