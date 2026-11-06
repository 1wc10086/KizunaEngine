package gothic

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"kizuna/engine/core"
)

const gothicMaxInt32 = int64(1<<31 - 1)

const gothicMinInt32 = int64(-1 << 31)

var gothicOperators = []string{"<>", "<=", ">=", "==", "!=", "=", "<", ">", "!"}

func GothicEvaluate(text string, variables *GothicVariableTable) bool {
	result, ok := GothicTryEvaluate(text, variables)
	if !ok {
		core.Logf("gothic: bad condition " + text)
		return false
	}
	return result
}

func GothicTryEvaluate(text string, variables *GothicVariableTable) (bool, bool) {
	if variables == nil || len(strings.TrimSpace(text)) == 0 {
		return false, false
	}
	normalized := strings.ReplaceAll(text, "&", " and ")
	for _, alternative := range splitKeyword(normalized, "or") {
		alternativeValue := true
		for _, part := range splitKeyword(alternative, "and") {
			if len(strings.TrimSpace(part)) == 0 {
				return false, false
			}
			atom, ok := tryEvaluateAtom(part, variables)
			if !ok {
				return false, false
			}
			if !atom {
				alternativeValue = false
				break
			}
		}
		if alternativeValue {
			return true, true
		}
	}
	return false, true
}

func GothicEvaluateValue(text string, variables *GothicVariableTable) int32 {
	value, _ := tryEvaluateArithmetic(strings.TrimSpace(text), variables)
	return value
}

func GothicParseInt(text string) int32 {
	i := 0
	for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	sign := int64(1)
	if i < len(text) && (text[i] == '+' || text[i] == '-') {
		if text[i] == '-' {
			sign = -1
		}
		i++
	}
	value := int64(0)
	any := false
	for i < len(text) && text[i] >= '0' && text[i] <= '9' {
		any = true
		value = value*10 + int64(text[i]-'0')
		i++
	}
	if !any {
		return 0
	}
	signed := sign * value
	if signed > gothicMaxInt32 {
		return int32(gothicMaxInt32)
	}
	if signed < gothicMinInt32 {
		return int32(gothicMinInt32)
	}
	return int32(signed)
}

func GothicTryParseStrict(text string) (int32, bool) {
	view := strings.TrimSpace(text)
	if len(view) == 0 || !isIntegerText(view) {
		return 0, false
	}
	return GothicParseInt(view), true
}

func tryEvaluateAtom(text string, variables *GothicVariableTable) (bool, bool) {
	atom := strings.TrimSpace(text)
	for len(atom) >= 2 && atom[0] == '(' && atom[len(atom)-1] == ')' {
		atom = strings.TrimSpace(atom[1 : len(atom)-1])
	}
	if len(atom) == 0 || strings.Contains(atom, "|") {
		return false, false
	}
	at, symbol := findOperator(atom)
	if at < 0 {
		return false, false
	}
	leftText := strings.TrimSpace(atom[:at])
	rightText := strings.TrimSpace(atom[at+len(symbol):])
	left, ok := tryEvaluateArithmetic(leftText, variables)
	if !ok {
		return false, false
	}
	right, ok := tryEvaluateArithmetic(rightText, variables)
	if !ok {
		return false, false
	}
	switch symbol {
	case "=", "==":
		return left == right, true
	case "<>", "!=", "!":
		return left != right, true
	case "<":
		return left < right, true
	case ">":
		return left > right, true
	case "<=":
		return left <= right, true
	case ">=":
		return left >= right, true
	}
	return false, true
}

func findOperator(atom string) (int, string) {
	for _, op := range gothicOperators {
		if at := strings.Index(atom, op); at >= 0 {
			return at, op
		}
	}
	return -1, ""
}

func tryEvaluateArithmetic(text string, variables *GothicVariableTable) (int32, bool) {
	view := strings.TrimSpace(text)
	if len(view) == 0 {
		return 0, false
	}
	i := 0
	total, ok := tryParseSum(view, &i, variables)
	if !ok {
		return 0, false
	}
	skipGothicSpaces(view, &i)
	if i != len(view) {
		return 0, false
	}
	if total > gothicMaxInt32 {
		return int32(gothicMaxInt32), true
	}
	if total < gothicMinInt32 {
		return int32(gothicMinInt32), true
	}
	return int32(total), true
}

func tryParseSum(view string, i *int, variables *GothicVariableTable) (int64, bool) {
	first, ok := tryParseProduct(view, i, variables)
	if !ok {
		return 0, false
	}
	total := first
	for {
		skipGothicSpaces(view, i)
		if *i >= len(view) {
			return total, true
		}
		op := view[*i]
		if op != '+' && op != '-' {
			return total, true
		}
		*i++
		next, ok := tryParseProduct(view, i, variables)
		if !ok {
			return 0, false
		}
		if op == '+' {
			total = total + next
		} else {
			total = total - next
		}
	}
}

func tryParseProduct(view string, i *int, variables *GothicVariableTable) (int64, bool) {
	first, ok := tryParseFactor(view, i, variables)
	if !ok {
		return 0, false
	}
	product := first
	for {
		skipGothicSpaces(view, i)
		if *i >= len(view) || view[*i] != '*' {
			return product, true
		}
		*i++
		next, ok := tryParseFactor(view, i, variables)
		if !ok {
			return 0, false
		}
		product = clampToIntRange(product * next)
	}
}

func tryParseFactor(view string, i *int, variables *GothicVariableTable) (int64, bool) {
	skipGothicSpaces(view, i)
	if *i >= len(view) {
		return 0, false
	}
	if view[*i] == '+' || view[*i] == '-' {
		negate := view[*i] == '-'
		*i++
		inner, ok := tryParseFactor(view, i, variables)
		if !ok {
			return 0, false
		}
		if negate {
			return -inner, true
		}
		return inner, true
	}
	if view[*i] == '(' {
		*i++
		inner, ok := tryParseSum(view, i, variables)
		if !ok {
			return 0, false
		}
		skipGothicSpaces(view, i)
		if *i >= len(view) || view[*i] != ')' {
			return 0, false
		}
		*i++
		return inner, true
	}
	return tryParseOperand(view, i, variables)
}

func tryParseOperand(view string, i *int, variables *GothicVariableTable) (int64, bool) {
	skipGothicSpaces(view, i)
	if *i >= len(view) {
		return 0, false
	}
	if view[*i] == '$' {
		start := *i
		*i++
		parsed := int64(0)
		for *i < len(view) && isHexDigit(view[*i]) {
			if parsed <= gothicMaxInt32 {
				parsed = parsed*16 + int64(hexValue(view[*i]))
			}
			*i++
		}
		if *i == start+1 {
			return 0, false
		}
		if *i < len(view) {
			if r := decodeRuneAt(view, *i); r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
				return 0, false
			}
		}
		if parsed > gothicMaxInt32 {
			return 0, false
		}
		return parsed, true
	}
	if view[*i] >= '0' && view[*i] <= '9' {
		parsed := int64(0)
		for *i < len(view) && view[*i] >= '0' && view[*i] <= '9' {
			if parsed <= gothicMaxInt32 {
				parsed = parsed*10 + int64(view[*i]-'0')
			}
			*i++
		}
		if *i < len(view) {
			if r := decodeRuneAt(view, *i); unicode.IsLetter(r) || r == '_' {
				return 0, false
			}
		}
		if parsed > gothicMaxInt32 {
			return gothicMaxInt32, true
		}
		return parsed, true
	}
	if r := decodeRuneAt(view, *i); r == '_' || unicode.IsLetter(r) {
		start := *i
		*i += runeLenAt(view, *i)
		for *i < len(view) {
			r := decodeRuneAt(view, *i)
			if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				break
			}
			*i += runeLenAt(view, *i)
		}
		name := view[start:*i]
		if strings.EqualFold(name, "cgall") {
			return 1, true
		}
		if variables == nil {
			return 0, true
		}
		return int64(variables.Get(name)), true
	}
	return 0, false
}

func skipGothicSpaces(view string, i *int) {
	for *i < len(view) && (view[*i] == ' ' || view[*i] == '\t') {
		*i++
	}
}

func clampToIntRange(value int64) int64 {
	if value > gothicMaxInt32 {
		return gothicMaxInt32
	}
	if value < gothicMinInt32 {
		return gothicMinInt32
	}
	return value
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func hexValue(c byte) int {
	if c >= '0' && c <= '9' {
		return int(c - '0')
	}
	if c >= 'a' && c <= 'f' {
		return int(c-'a') + 10
	}
	return int(c-'A') + 10
}

func decodeRuneAt(s string, i int) rune {
	r, _ := utf8.DecodeRuneInString(s[i:])
	return r
}

func runeLenAt(s string, i int) int {
	_, n := utf8.DecodeRuneInString(s[i:])
	if n <= 0 {
		return 1
	}
	return n
}

func splitKeyword(text, keyword string) []string {
	var parts []string
	start := 0
	i := 0
	for i < len(text) {
		if isKeywordAt(text, i, keyword) {
			parts = append(parts, text[start:i])
			i += len(keyword)
			start = i
		} else {
			i++
		}
	}
	parts = append(parts, text[start:])
	return parts
}

func isKeywordAt(text string, index int, keyword string) bool {
	if index+len(keyword) > len(text) {
		return false
	}
	for k := 0; k < len(keyword); k++ {
		if lowerASCII(text[index+k]) != keyword[k] {
			return false
		}
	}
	before := index == 0 || isWordDelimiter(text[index-1])
	afterIndex := index + len(keyword)
	after := afterIndex >= len(text) || isWordDelimiter(text[afterIndex])
	return before && after
}

func lowerASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

func isWordDelimiter(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '(' || ch == ')' || ch == '&'
}

func isIntegerText(text string) bool {
	i := 0
	if text[0] == '+' || text[0] == '-' {
		if len(text) == 1 {
			return false
		}
		i = 1
	}
	r, n := utf8.DecodeRuneInString(text[i:])
	if !unicode.IsDigit(r) {
		return false
	}
	i += n
	for i < len(text) {
		r, n := utf8.DecodeRuneInString(text[i:])
		if !unicode.IsDigit(r) {
			return false
		}
		i += n
	}
	return true
}
