package script

import (
	"strings"
	"unicode"
)

func ParseBytes(name string, scriptBytes []byte) *EmonProgram {
	return Parse(name, DecodeScript(scriptBytes))
}

func Parse(name string, scriptText string) *EmonProgram {
	scriptText = applyDefines(scriptText)
	statements := make([]*EmonStatement, 0, 64)
	lines := SplitLines(scriptText)
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		number := int32(i + 1)
		runes := []rune(line)
		start := 0
		for start < len(runes) && unicode.IsSpace(runes[start]) {
			start++
		}

		if start >= len(runes) {
			continue
		}

		if runes[start] != '#' {
			// 原引擎的脚本行允许"正文 + 内联命令 + 正文"混排：
			// BONYUMANAMI 的 BMzengi.txt 有 1500 多行形如
			//   本文#IfJump(割り込み選択,SelectNumber!=0)#Page#IfJump(...)
			// ZUBUNURE 的 lv1d01.txt 则是
			//   正文#立ち消去#StopSound#Sound(KS04.ogg)后续正文
			// 之前的实现把整行当正文，命令文本会原样画到对话框里。
			segments := splitTextLine(line)
			// seenCommand 记录"本行已经出现过命令"：之后再出现的正文属于
			// 同一段的后半句（ZUBUNURE 的 `閉めた。#立ち消去#…ご丁寧に…`），
			// 追加时不能插换行，否则原版一行会被拆成两行。
			seenCommand := false
			for index, segment := range segments {
				if segment.command != "" {
					statements = append(statements, parseCommand("#"+segment.command, line, number))
					seenCommand = true
					continue
				}
				// 只裁掉整行首尾的空白，中间段的缩进（如
				// 「#return　　　残っていて下さい。」的缩排）属于正文，要保留。
				text := segment.text
				if index == 0 {
					text = strings.TrimLeft(text, " \t　")
				}
				if index == len(segments)-1 {
					text = strings.TrimRight(text, " \t　")
				}
				if text == "" {
					continue
				}
				statements = append(statements, &EmonStatement{
					Raw:    line,
					Line:   number,
					IsText: true,
					Inline: seenCommand,
					Text:   text,
				})
			}
			continue
		}

		for _, invocation := range splitInvocations(runes, start) {
			statements = append(statements, parseCommand(invocation, line, number))
		}
	}

	return NewEmonProgram(name, statements)
}

type textSegment struct {
	text    string
	command string
}

// inlineTextCommands 是允许出现在正文中间、由正文求值阶段处理的两条命令：
// #String(变量) 做变量插值，#Return(注音)/#Return『注音』 做行内换行与注音。
// 其它 `#名字` 在原引擎里都是独立语句，必须切出来执行。
var inlineTextCommands = map[string]bool{
	"STRING": true,
	"RETURN": true,
}

// splitTextLine 按原引擎的语义切分"以正文开头"的行：
// 遇到 `#` 且后面不是 String/Return 时，`#` 起就是一条独立语句，
// 语句在括号配对结束（或下一个 `#` 处）为止；剩下的仍然是正文。
func splitTextLine(line string) []textSegment {
	segments := []textSegment{}
	runes := []rune(line)
	var text strings.Builder
	flushText := func() {
		if text.Len() > 0 {
			segments = append(segments, textSegment{text: text.String()})
			text.Reset()
		}
	}
	for i := 0; i < len(runes); i++ {
		if runes[i] != '#' {
			text.WriteRune(runes[i])
			continue
		}
		name, nameEnd := commandNameAt(runes, i+1)
		upper := strings.ToUpper(name)
		// #String(变量) 做插值、#Return(注音)/#Return『注音』 做行内注音，
		// 这两种必须留在正文里交给 expandInline；裸 #Return 是"软换行"语句，
		// 要切出来交给 onReturn —— 否则同一行后面的正文会被并进同一个 Text
		// 语句，丢失原来的分行结构。
		if name == "" || (inlineTextCommands[upper] && (upper != "RETURN" || returnHasArgument(runes, nameEnd))) {
			text.WriteRune(runes[i])
			continue
		}
		flushText()
		end := nameEnd
		if end < len(runes) && (runes[end] == '(' || runes[end] == '（') {
			if close := findClose(runes, end); close >= 0 {
				end = close + 1
			}
		}
		segments = append(segments, textSegment{command: string(runes[i+1 : end])})
		i = end - 1
	}
	flushText()
	return segments
}

// returnHasArgument 判断 #Return 后面是否带注音参数（半角或全角括号、『』）。
func returnHasArgument(runes []rune, end int) bool {
	for end < len(runes) && unicode.IsSpace(runes[end]) {
		end++
	}
	if end >= len(runes) {
		return false
	}
	r := runes[end]
	return r == '(' || r == '（' || r == '『'
}

// commandNameAt 读取 `#` 之后紧跟的命令名，返回名字与结束下标。
// ASCII 命令名必须在遇到非标识符字符处停下（`#return一体何の用…` 里
// Return 之后是日文正文，命令名只能是 "return"）；日文命令（ZUBUNURE 的
// #立ち消去）则必须后面紧跟 `#`、`(` 或行尾，避免把正文一起吞掉。
func commandNameAt(runes []rune, start int) (string, int) {
	if start >= len(runes) {
		return "", start
	}
	if isASCIIIdentifierRune(runes[start]) {
		end := start
		for end < len(runes) && isASCIIIdentifierRune(runes[end]) {
			end++
		}
		return string(runes[start:end]), end
	}
	// 日文命令：吃到命令边界为止（`#`、`(`、行尾），但必须真的停在边界上，
	// 否则 `#return一体何の用…` 这种行内标记会把正文吞掉。
	end := start
	for end < len(runes) {
		r := runes[end]
		if unicode.IsSpace(r) || r == '#' || r == '(' || r == ')' || r == '（' || r == '）' || isQuoteRune(r) {
			break
		}
		end++
	}
	if end == start {
		return "", start
	}
	if end < len(runes) && runes[end] != '#' && runes[end] != '(' {
		return "", start
	}
	return string(runes[start:end]), end
}

func isASCIIIdentifierRune(r rune) bool {
	return r == '_' || (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

func splitInvocations(runes []rune, start int) []string {
	invocations := make([]string, 0, 2)
	segmentStart := start
	inQuotes := false
	for i := start; i < len(runes); i++ {
		value := runes[i]
		if isQuoteRune(value) {
			inQuotes = !inQuotes
		} else if !inQuotes {
			if value == '(' {
				close := findClose(runes, i)
				if close >= 0 {
					i = close
				}
			} else if value == '#' && i > segmentStart {
				invocations = append(invocations, string(runes[segmentStart:i]))
				segmentStart = i
			}
		}
	}

	invocations = append(invocations, string(runes[segmentStart:]))
	return invocations
}

func parseCommand(invocation string, raw string, line int32) *EmonStatement {
	body := []rune(strings.TrimSpace(invocation))
	open := -1
	inQuotes := false
	for i := 1; i < len(body); i++ {
		if isQuoteRune(body[i]) {
			inQuotes = !inQuotes
		} else if body[i] == '(' && !inQuotes {
			open = i
			break
		}
	}

	if open < 0 {
		return &EmonStatement{
			Command: trimCommandName(body),
			Raw:     raw,
			Line:    line,
		}
	}

	close := findClose(body, open)
	var argumentsText string
	if close < 0 {
		argumentsText = string(body[open+1:])
	} else {
		argumentsText = string(body[open+1 : close])
	}

	return &EmonStatement{
		Command:   strings.TrimSpace(string(body[1:open])),
		Raw:       raw,
		Line:      line,
		Arguments: splitArguments(argumentsText),
	}
}

func trimCommandName(body []rune) string {
	if len(body) > 1 {
		return strings.TrimSpace(string(body[1:]))
	}

	return ""
}

func findClose(body []rune, open int) int {
	depth := 1
	inQuotes := false
	for i := open + 1; i < len(body); i++ {
		if isQuoteRune(body[i]) {
			inQuotes = !inQuotes
		} else if !inQuotes {
			if body[i] == '(' {
				depth++
			} else if body[i] == ')' {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
	}

	return -1
}

func splitArguments(argumentsText string) []EmonArgument {
	if strings.TrimSpace(argumentsText) == "" {
		return nil
	}

	runes := []rune(argumentsText)
	arguments := make([]EmonArgument, 0, 4)
	depth := 0
	inQuotes := false
	start := 0
	for i := 0; i < len(runes); i++ {
		value := runes[i]
		if value == '"' {
			inQuotes = !inQuotes
		} else if !inQuotes {
			if value == '(' || value == '[' || value == '{' {
				depth++
			} else if value == ')' || value == ']' || value == '}' {
				if depth > 0 {
					depth--
				}
			} else if value == ',' && depth == 0 {
				arguments = append(arguments, parseArgument(string(runes[start:i])))
				start = i + 1
			}
		}
	}

	arguments = append(arguments, parseArgument(string(runes[start:])))
	return arguments
}

func parseArgument(part string) EmonArgument {
	trimmed := strings.TrimSpace(part)
	runes := []rune(trimmed)
	if len(runes) >= 2 && runes[0] == '"' && runes[len(runes)-1] == '"' {
		return EmonArgument{Raw: string(runes[1 : len(runes)-1]), WasQuoted: true}
	}

	return EmonArgument{Raw: trimmed}
}

// 原引擎的 #Define 是纯文本替换（sub_417200 在加载期登记，sub_417040 按
// 脚本顺序把名字原样换成值，替换结果只看文本，不经过变量表）：
// `#Define(名,11)` 之后所有出现「名」的地方都会变成 11，包括其他命令
// 的参数内嵌的 #Define（先擦除、再替换）。
func applyDefines(text string) string {
	type define struct {
		name  string
		value string
	}

	defines := []define{}
	blanked := []rune(text)
	runes := []rune(text)
	search := 0
	for search < len(runes)-len("#define") {
		idx := indexDefine(runes, search)
		if idx < 0 {
			break
		}

		open := idx + len("#define")
		if open >= len(runes) || runes[open] != '(' {
			search = idx + 1
			continue
		}

		close := findClose(runes, open)
		if close < 0 {
			search = idx + 1
			continue
		}

		arguments := splitArguments(string(runes[open+1 : close]))
		if len(arguments) >= 2 {
			name := strings.TrimSpace(arguments[0].Raw)
			if name != "" {
				defines = append(defines, define{name: name, value: arguments[1].Raw})
			}
		}

		for i := idx; i <= close; i++ {
			blanked[i] = ' '
		}
		search = close + 1
	}

	if len(defines) == 0 {
		return text
	}

	result := string(blanked)
	// 原引擎按登记的逆序做替换（后 Define 的先展开）。
	for i := len(defines) - 1; i >= 0; i-- {
		result = strings.ReplaceAll(result, defines[i].name, defines[i].value)
	}

	return result
}

// 大小写不敏感地找 "#Define"（原引擎用 _strnicmp），避免在全角文本里
// 误匹配时逐字节回退的开销。
func indexDefine(runes []rune, from int) int {
	const marker = "#define"
	for i := from; i+len(marker) <= len(runes); i++ {
		if !strings.EqualFold(string(runes[i:i+len(marker)]), marker) {
			continue
		}

		return i
	}

	return -1
}
