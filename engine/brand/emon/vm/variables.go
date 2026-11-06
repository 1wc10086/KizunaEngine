package vm

import (
	"strconv"
	"strings"

	"kizuna/engine/brand/emon/script"
	"kizuna/engine/core"
	"kizuna/engine/core/strutil"
	"unicode/utf16"
)

func (m *Machine) onSetNumber(statement *script.EmonStatement) EmonStep {
	for i := int32(0); i < statement.ArgumentCount(); i++ {
		m.applyAssignments(m.raw(statement, i), m.SetVariable)
	}
	return EmonStepContinue
}

func (m *Machine) onNumber(statement *script.EmonStatement) EmonStep {
	for i := int32(0); i < statement.ArgumentCount(); i++ {
		m.applyAssignments(m.raw(statement, i), m.SetVariable)
	}
	return EmonStepContinue
}

func (m *Machine) onIfNumber(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() == 0 {
		return EmonStepContinue
	}
	if !m.conditionsFrom(statement, 1) {
		return EmonStepContinue
	}
	m.applyAssignments(m.raw(statement, 0), m.SetVariable)
	return EmonStepContinue
}

func (m *Machine) onRandomNumber(statement *script.EmonStatement) EmonStep {
	destination := m.raw(statement, 0)
	if destination == "" {
		core.Logf("emon: random number needs a destination")
		return EmonStepContinue
	}
	if !m.isReplaying() {
		m.SetVariable(destination, m.RandomValue(m.EvalInt(statement, 1, 0)))
	}
	return EmonStepContinue
}

func (m *Machine) onSetSysNumber(statement *script.EmonStatement) EmonStep {
	for i := int32(0); i < statement.ArgumentCount(); i++ {
		m.applyAssignments(m.raw(statement, i), m.DeclareSystemVariable)
	}
	return EmonStepContinue
}

func (m *Machine) onSetMacroNumber(statement *script.EmonStatement) EmonStep {
	for i := int32(0); i < statement.ArgumentCount(); i++ {
		m.applyAssignments(m.raw(statement, i), m.setMacroDefault)
	}
	return EmonStepContinue
}

func (m *Machine) onSetSymbol(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() >= 3 {
		m.formatSymbol(statement)
		return EmonStepContinue
	}
	for i := int32(0); i < statement.ArgumentCount(); i++ {
		m.applyAssignments(m.raw(statement, i), m.SetSymbol)
	}
	return EmonStepContinue
}

func (m *Machine) onGetStrLen(statement *script.EmonStatement) EmonStep {
	destination := m.raw(statement, 0)
	if destination == "" {
		core.Logf("emon: get string length needs a destination")
		return EmonStepContinue
	}
	m.SetVariable(destination, script.EmonValueFromInt(int32(len(utf16.Encode([]rune(m.value(statement, 1).AsString()))))))
	return EmonStepContinue
}

func (m *Machine) onString(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() == 0 {
		return EmonStepContinue
	}
	if statement.ArgumentCount() == 1 {
		return EmonStepContinue
	}
	destination := m.raw(statement, 0)
	if destination == "" {
		return EmonStepContinue
	}
	m.SetVariable(destination, script.EmonValueFromString(m.value(statement, 1).AsString()))
	return EmonStepContinue
}

func (m *Machine) onSetOnListStr(statement *script.EmonStatement) EmonStep {
	destination := m.raw(statement, 0)
	if destination == "" || statement.ArgumentCount() < 2 {
		core.Logf("emon: set on list string needs a destination and an index")
		return EmonStepContinue
	}
	index := m.EvalInt(statement, 1, 0)
	slot := index + 2
	if slot < 2 || int32(slot) >= statement.ArgumentCount() {
		core.Logf("emon: set on list string index %d is out of range", index)
		return EmonStepContinue
	}
	m.SetVariable(destination, script.EmonValueFromString(statement.Argument(int32(slot)).Raw))
	return EmonStepContinue
}

func (m *Machine) onSetOnCalListNumber(statement *script.EmonStatement) EmonStep {
	destination := m.raw(statement, 0)
	if destination == "" || statement.ArgumentCount() < 3 {
		core.Logf("emon: set on calculated list number needs a destination, a key and a template")
		return EmonStepContinue
	}
	key := m.raw(statement, 1)
	template := m.raw(statement, 2)
	for i := int32(3); i < statement.ArgumentCount(); i++ {
		probe := substituteListItem(template, key, statement.Argument(i).Raw)
		if m.condition(probe) {
			m.SetVariable(destination, script.EmonValueFromInt(int32(i-3)))
			return EmonStepContinue
		}
	}
	m.SetVariable(destination, script.EmonValueFromInt(-1))
	return EmonStepContinue
}

func substituteListItem(template, key, item string) string {
	if key == "" {
		return template
	}
	literal := item
	if !isASCIIDigits(item) {
		literal = "\"" + item + "\""
	}
	templateRunes := []rune(template)
	keyRunes := []rune(key)
	var builder strings.Builder
	builder.Grow(len(template) + len(literal))
	var quote rune
	index := 0
	for index < len(templateRunes) {
		character := templateRunes[index]
		insideQuotes := quote != 0
		if index+len(keyRunes) <= len(templateRunes) && equalRunes(templateRunes[index:index+len(keyRunes)], keyRunes) {
			if insideQuotes {
				builder.WriteString(item)
			} else {
				builder.WriteString(literal)
			}
			index += len(keyRunes)
			continue
		}
		if character == quote {
			quote = 0
			builder.WriteRune(character)
			index++
			continue
		}
		if !insideQuotes && (character == '"' || character == '＂') {
			quote = character
			builder.WriteRune(character)
			index++
			continue
		}
		builder.WriteRune(character)
		index++
	}
	return builder.String()
}

func equalRunes(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (m *Machine) setMacroDefault(name string, value script.EmonValue) {
	if name == "" {
		return
	}
	if args := m.topArguments(); args != nil {
		if ordinal, ok := tryMacroArgumentIndex(name); ok {
			canonical := macroArgumentName(ordinal)
			current := args.Lookup(canonical)
			if current.IsInteger() || current.AsString() != canonical {
				return
			}
			args.Set(canonical, value)
			args.Set(macroArgumentAlias(ordinal), value)
			return
		}
		local := args.Lookup(name)
		if local.IsInteger() || local.AsString() != name {
			return
		}
		args.Set(name, value)
		return
	}
	m.context.Variables.Set(name, value)
}

func (m *Machine) formatSymbol(statement *script.EmonStatement) {
	destination := m.raw(statement, 0)
	if destination == "" {
		return
	}
	format := m.text(statement, 1)
	values := []script.EmonValue{}
	for i := int32(2); i < statement.ArgumentCount(); i++ {
		values = append(values, m.value(statement, i))
	}
	m.SetSymbol(destination, script.EmonValueFromString(formatPrintf(format, values)))
}

func formatPrintf(format string, values []script.EmonValue) string {
	var output strings.Builder
	totalConversions := countPercentConversions(format)
	conversionOrdinal := 0
	index := 0
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			output.WriteByte(format[i])
			continue
		}
		j := i + 1
		zeroPad := 0
		for j < len(format) && format[j] >= '0' && format[j] <= '9' {
			zeroPad = zeroPad*10 + int(format[j]-'0')
			j++
		}
		if j >= len(format) {
			output.WriteString(format[i:])
			break
		}
		code := format[j]
		if code != 'd' && code != 's' && code != 'x' && code != 'X' {
			output.WriteByte(format[i])
			continue
		}
		conversionOrdinal++
		value := script.EmonValueFromInt(0)
		if index < len(values) {
			value = values[index]
		}
		index++
		var rendered string
		switch code {
		case 's':
			rendered = value.AsString()
		case 'd':
			rendered = strconv.FormatInt(int64(value.AsInt()), 10)
		case 'x':
			rendered = strconv.FormatUint(uint64(uint32(value.AsInt())), 16)
		default:
			rendered = strings.ToUpper(strconv.FormatUint(uint64(uint32(value.AsInt())), 16))
		}
		if zeroPad > len(rendered) && code != 's' {
			rendered = strings.Repeat("0", zeroPad-len(rendered)) + rendered
		}
		if code == 's' && conversionOrdinal == totalConversions && index < len(values) && values[index].IsInteger() {
			maxLen := int(values[index].AsInt())
			index++
			if maxLen >= 0 {
				rendered = truncateUTF16Units(rendered, maxLen)
			}
		}
		output.WriteString(rendered)
		i = j
	}
	return output.String()
}

func truncateUTF16Units(text string, maxLen int) string {
	units := utf16.Encode([]rune(text))
	if len(units) > maxLen {
		units = units[:maxLen]
		if len(units) > 0 && units[len(units)-1] >= 0xD800 && units[len(units)-1] <= 0xDBFF {
			units = units[:len(units)-1]
		}
	}
	return string(utf16.Decode(units))
}

func countPercentConversions(format string) int {
	count := 0
	i := 0
	for i < len(format) {
		if format[i] != '%' || i+1 >= len(format) {
			i++
			continue
		}
		j := i + 1
		for j < len(format) && format[j] >= '0' && format[j] <= '9' {
			j++
		}
		if j >= len(format) {
			break
		}
		if format[j] == 'd' || format[j] == 's' || format[j] == 'x' || format[j] == 'X' {
			count++
		}
		i = j + 1
	}
	return count
}

func isFractionalLiteral(text string) bool {
	trimmed := strings.TrimSpace(text)
	start := 0
	if strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "-") {
		start = 1
	}
	dot := false
	digits := 0
	for i := start; i < len(trimmed); i++ {
		if trimmed[i] == '.' {
			if dot {
				return false
			}
			dot = true
			continue
		}
		if trimmed[i] < '0' || trimmed[i] > '9' {
			return false
		}
		digits++
	}
	return dot && digits > 0
}

func (m *Machine) applyAssignments(text string, set func(string, script.EmonValue)) {
	for _, part := range splitAssignments(text) {
		assignment := strings.TrimSpace(part)
		if assignment == "" {
			continue
		}
		for _, alternative := range splitAlternatives(assignment) {
			m.applySingleAssignment(strings.TrimSpace(alternative), set)
		}
	}
}

func (m *Machine) applySingleAssignment(text string, set func(string, script.EmonValue)) {
	stripped := stripOuterParentheses(strings.TrimSpace(text))
	name, operation, expression, ok := trySplitAssignment(stripped)
	if !ok {
		return
	}
	current := m.Lookup(name)
	var value script.EmonValue
	if expression == "" {
		value = script.EmonValueFromString("")
	} else if isFractionalLiteral(expression) {
		value = script.EmonValueFromString(strings.TrimSpace(expression))
	} else {
		value = script.Evaluate(expression, m.resolver)
	}
	switch operation {
	case "=":
		set(name, value)
	case "+=":
		set(name, current.Add(value))
	case "-=":
		set(name, current.Subtract(value))
	case "*=":
		set(name, current.Multiply(value))
	case "/=":
		set(name, current.Divide(value))
	case "%=":
		set(name, current.Modulo(value))
	case "|=":
		set(name, script.EmonValueFromInt(current.AsInt()|value.AsInt()))
	case "&=":
		set(name, script.EmonValueFromInt(current.AsInt()&value.AsInt()))
	case "^=":
		set(name, script.EmonValueFromInt(current.AsInt()^value.AsInt()))
	case "<<=":
		set(name, script.EmonValueFromInt(current.AsInt()<<(uint32(value.AsInt())&31)))
	case ">>=":
		set(name, script.EmonValueFromInt(current.AsInt()>>(uint32(value.AsInt())&31)))
	}
}

func trySplitAssignment(text string) (string, string, string, bool) {
	runes := []rune(text)
	offs := runeOffsets(text, len(runes))
	depth := 0
	var quote rune
	for i := 0; i < len(runes); i++ {
		character := runes[i]
		if quote != 0 {
			if character == quote {
				quote = 0
			}
			continue
		}
		if character == '"' || character == '＂' {
			quote = character
			continue
		}
		if character == '(' || character == '（' || character == '[' || character == '［' || character == '{' || character == '｛' {
			depth++
			continue
		}
		if character == ')' || character == '）' || character == ']' || character == '］' || character == '}' || character == '｝' {
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth != 0 || character != '=' {
			continue
		}
		var previous rune
		if i > 0 {
			previous = runes[i-1]
		}
		var next rune
		if i+1 < len(runes) {
			next = runes[i+1]
		}
		if next == '=' || previous == '=' || previous == '!' {
			continue
		}
		doubled := (previous == '<' || previous == '>') && i >= 2 && runes[i-2] == previous
		if (previous == '<' || previous == '>') && !doubled {
			continue
		}
		rawName := text[:offs[i]]
		expression := strings.Trim(text[offs[i+1]:], " \t\r\n")
		operation := "="
		if doubled {
			operation = string([]rune{previous, previous}) + "="
			rawName = text[:offs[i-2]]
		} else if previous == '+' || previous == '-' || previous == '*' || previous == '/' || previous == '%' || previous == '|' || previous == '&' || previous == '^' {
			operation = string(previous) + "="
			rawName = text[:offs[i-1]]
		}
		name := strings.Trim(rawName, " \t\r\n")
		if name == "" {
			return "", "", "", false
		}
		return name, operation, expression, true
	}
	return "", "", "", false
}

func splitAssignments(text string) []string {
	return strutil.SplitTopLevel(text, ',')
}

func splitAlternatives(text string) []string {
	runes := []rune(text)
	offs := runeOffsets(text, len(runes))
	result := []string{}
	depth := 0
	var quote rune
	start := 0
	for i := 0; i < len(runes); i++ {
		character := runes[i]
		if quote != 0 {
			if character == quote {
				quote = 0
			}
			continue
		}
		if character == '"' || character == '＂' {
			quote = character
			continue
		}
		if character == '(' || character == '（' || character == '[' || character == '［' || character == '{' || character == '｛' {
			depth++
		} else if character == ')' || character == '）' || character == ']' || character == '］' || character == '}' || character == '｝' {
			if depth > 0 {
				depth--
			}
		} else if character == '|' && depth == 0 && i+1 < len(runes) && runes[i+1] == '|' {
			result = append(result, text[start:offs[i]])
			start = offs[i+2]
			i++
		}
	}
	result = append(result, text[start:])
	return rejoinExpressions(result)
}

func rejoinExpressions(parts []string) []string {
	rejoined := []string{}
	for _, part := range parts {
		if len(rejoined) != 0 && !containsAssignment(part) {
			rejoined[len(rejoined)-1] = rejoined[len(rejoined)-1] + "||" + part
		} else {
			rejoined = append(rejoined, part)
		}
	}
	return rejoined
}

func containsAssignment(text string) bool {
	_, _, _, ok := trySplitAssignment(stripOuterParentheses(strings.TrimSpace(text)))
	return ok
}

func runeOffsets(text string, count int) []int {
	offs := make([]int, 0, count+1)
	for i := range text {
		offs = append(offs, i)
	}
	return append(offs, len(text))
}

func stripOuterParentheses(text string) string {
	current := text
	for len(current) >= 2 && current[0] == '(' && current[len(current)-1] == ')' {
		depth := 0
		balanced := true
		for i := 0; i < len(current); i++ {
			if current[i] == '(' {
				depth++
			} else if current[i] == ')' {
				depth--
				if depth == 0 && i != len(current)-1 {
					balanced = false
					break
				}
			}
		}
		if !balanced || depth != 0 {
			break
		}
		current = strings.TrimSpace(current[1 : len(current)-1])
	}
	return current
}

func parseIntInvariant(text string) (int32, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || strings.Contains(trimmed, "_") {
		return 0, false
	}
	value, err := strconv.ParseInt(trimmed, 10, 32)
	if err != nil {
		return 0, false
	}
	return int32(value), true
}

func tryParseInt(text string) (int, bool) {
	value, ok := parseIntInvariant(text)
	return int(value), ok
}
