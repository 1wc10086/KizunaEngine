package bdt

const bdtExpressionMaxDepth = 16

func ParseBdtExpression(text string) BdtExpression {
	if expression, ok := TryParseBdtExpression(text); ok {
		return expression
	}
	return invalidBdtExpression()
}

func TryParseBdtExpression(text string) (BdtExpression, bool) {
	if len(text) == 0 {
		return invalidBdtExpression(), false
	}
	reader := &bdtExpressionReader{text: text}
	root := reader.parseExpression(0)
	reader.skipSpaces()
	if root == nil || !reader.isAtEnd() {
		return invalidBdtExpression(), false
	}
	return newValidBdtExpression(root), true
}

func EvaluateBdtExpression(text string, resolve func(byte, int32) int32) int32 {
	if resolve == nil {
		return -1
	}
	return ParseBdtExpression(text).Evaluate(resolve)
}

type bdtExpressionReader struct {
	text     string
	position int
}

func (r *bdtExpressionReader) isAtEnd() bool {
	return r.position >= len(r.text)
}

func (r *bdtExpressionReader) skipSpaces() {
	for r.position < len(r.text) && (r.text[r.position] == ' ' || r.text[r.position] == '\t') {
		r.position++
	}
}

func (r *bdtExpressionReader) parseExpression(depth int) bdtExpressionNode {
	r.skipSpaces()
	if r.position >= len(r.text) {
		return nil
	}
	current := r.text[r.position]
	if current == '(' {
		return r.parseGroup(depth)
	}
	if current == '&' || current == '|' || current == '=' || current == '<' || current == '>' || current == '!' {
		r.position++
		r.skipSpaces()
		if r.position >= len(r.text) || r.text[r.position] != '(' {
			return nil
		}
		arguments := r.parseGroupArguments(depth)
		if arguments == nil {
			return nil
		}
		return &bdtApplyNode{operator: current, arguments: arguments}
	}
	return r.parseAtom()
}

func (r *bdtExpressionReader) parseGroupArguments(depth int) []bdtExpressionNode {
	if depth >= bdtExpressionMaxDepth {
		return nil
	}
	r.position++
	arguments := []bdtExpressionNode{}
	first := r.parseExpression(depth + 1)
	if first == nil {
		return nil
	}
	arguments = append(arguments, first)
	for {
		r.skipSpaces()
		if r.position >= len(r.text) {
			return nil
		}
		if r.text[r.position] != ':' {
			break
		}
		r.position++
		next := r.parseExpression(depth + 1)
		if next == nil {
			return nil
		}
		arguments = append(arguments, next)
	}
	r.skipSpaces()
	if r.position >= len(r.text) || r.text[r.position] != ')' {
		return nil
	}
	r.position++
	return arguments
}

func (r *bdtExpressionReader) parseGroup(depth int) bdtExpressionNode {
	arguments := r.parseGroupArguments(depth)
	if arguments == nil {
		return nil
	}
	if len(arguments) == 1 {
		return arguments[0]
	}
	return &bdtGroupNode{items: arguments}
}

func (r *bdtExpressionReader) parseAtom() bdtExpressionNode {
	r.skipSpaces()
	if r.position >= len(r.text) {
		return nil
	}
	current := r.text[r.position]
	if isBdtBankLetter(current) {
		r.position++
		index := r.parseDigits()
		if index < 0 {
			return nil
		}
		return &bdtVariableNode{bank: bdtToLower(current), index: index}
	}
	if current >= '0' && current <= '9' {
		value := r.parseDigits()
		if value < 0 {
			return nil
		}
		return &bdtConstantNode{value: value}
	}
	return nil
}

func (r *bdtExpressionReader) parseDigits() int32 {
	start := r.position
	var value int64
	for r.position < len(r.text) && r.text[r.position] >= '0' && r.text[r.position] <= '9' {
		value = value*10 + int64(r.text[r.position]-'0')
		if value > 2147483647 {
			return 2147483647
		}
		r.position++
	}
	if r.position == start {
		return -1
	}
	return int32(value)
}

func isBdtBankLetter(value byte) bool {
	switch bdtToLower(value) {
	case 'x', 'z', 'l', 't', 'h', 'c', 'g':
		return true
	default:
		return false
	}
}

func bdtToLower(value byte) byte {
	if value >= 'A' && value <= 'Z' {
		return value + ('a' - 'A')
	}
	return value
}
