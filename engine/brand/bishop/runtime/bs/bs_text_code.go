package bs

import "strings"

const (
	textCodeEscape           = '\\'
	textCodeIdeographicSpace = '\u3000'
	textCodeSpace            = ' '
	textCodeClosingMarks     = "。、，．」』！？!?…"
	textCodeSeparators       = " \u3000\u30fb"
)

func Speaker(value string) string {
	if len(value) == 0 {
		return ""
	}
	text := value
	if strings.IndexByte(value, textCodeEscape) >= 0 {
		text = stripTextCode(value)
	}
	return trimTextSeparators(text)
}

func Message(value string) string {
	if len(value) == 0 {
		return ""
	}
	text := value
	if strings.IndexByte(value, textCodeEscape) >= 0 {
		text = stripTextCode(value)
	}
	return collapseTextSpaces(text)
}

// revHeartCode 是旧式 BSScript 文本里的爱心占位符。
//
// KD 的 32000 条消息里有 7000 多处 "$0"，位置都在语气词末尾
// （「えへ$0」「チュ$0」「あ～ん$0」）。原引擎在渲染前把它替换成
// 字体里的爱心字形；直接显示会在对白里留下 "$0" 字样。
const revHeartCode = "$0"

// LegacyMessage 是 BSScript 旧作的对白清洗：先按通用规则剥控制码/压缩空格，
// 再把爱心占位符换成可显示的 ♥。现代 BSXScript 走另一条消息路径，不受影响。
func LegacyMessage(value string) string {
	return strings.ReplaceAll(Message(value), revHeartCode, "♥")
}

func stripTextCode(value string) string {
	source := []rune(value)
	buffer := make([]rune, 0, len(source))
	for index := 0; index < len(source); index++ {
		if source[index] == textCodeEscape {
			index++
			continue
		}
		buffer = append(buffer, source[index])
	}
	return string(buffer)
}

func collapseTextSpaces(value string) string {
	source := []rune(value)
	buffer := make([]rune, 0, len(source))
	pendingSpace := false
	for _, character := range source {
		if character == textCodeSpace || character == textCodeIdeographicSpace {
			pendingSpace = true
			continue
		}
		if pendingSpace {
			pendingSpace = false
			if len(buffer) > 0 && strings.IndexRune(textCodeClosingMarks, character) < 0 {
				buffer = append(buffer, textCodeIdeographicSpace)
			}
		}
		buffer = append(buffer, character)
	}
	return string(buffer)
}

func trimTextSeparators(value string) string {
	return strings.Trim(value, textCodeSeparators)
}
