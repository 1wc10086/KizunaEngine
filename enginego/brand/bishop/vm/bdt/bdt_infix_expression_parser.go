package bdt

const bdtInfixMaxDepth = 16

func ParseBdtInfixExpression(text string) BdtInfixExpression {
	if expression, ok := TryParseBdtInfixExpression(text); ok {
		return expression
	}
	return invalidBdtInfixExpression()
}

func TryParseBdtInfixExpression(text string) (BdtInfixExpression, bool) {
	if len(text) == 0 {
		return invalidBdtInfixExpression(), false
	}
	reader := &bdtInfixReader{text: text}
	root := reader.parseExpression(0)
	reader.skipSpaces()
	if root == nil || !reader.isAtEnd() {
		return invalidBdtInfixExpression(), false
	}
	return newValidBdtInfixExpression(root), true
}

func EvaluateBdtInfixExpression(text string, resolve func(byte, int32) int32) int32 {
	if resolve == nil {
		return -1
	}
	if expression, ok := TryParseBdtInfixExpression(text); ok {
		return expression.Evaluate(resolve)
	}
	return -1
}

type bdtInfixReader struct {
	text     string
	position int
}

func (r *bdtInfixReader) isAtEnd() bool {
	return r.position >= len(r.text)
}

func (r *bdtInfixReader) skipSpaces() {
	for r.position < len(r.text) && (r.text[r.position] == ' ' || r.text[r.position] == '\t') {
		r.position++
	}
}

func (r *bdtInfixReader) parseExpression(depth int) bdtInfixNode {
	if depth >= bdtInfixMaxDepth {
		return nil
	}
	left := r.parseTerm(depth)
	if left == nil {
		return nil
	}
	for {
		r.skipSpaces()
		if r.position >= len(r.text) || (r.text[r.position] != '&' && r.text[r.position] != '|') {
			return left
		}
		op := r.text[r.position]
		r.position++
		right := r.parseTerm(depth)
		if right == nil {
			return nil
		}
		left = &bdtInfixLogicNode{operator: op, left: left, right: right}
	}
}

func (r *bdtInfixReader) parseTerm(depth int) bdtInfixNode {
	if depth >= bdtInfixMaxDepth {
		return nil
	}
	r.skipSpaces()
	if r.position >= len(r.text) {
		return nil
	}
	if r.text[r.position] == '(' {
		r.position++
		inner := r.parseExpression(depth + 1)
		if inner == nil {
			return nil
		}
		r.skipSpaces()
		if r.position >= len(r.text) || r.text[r.position] != ')' {
			return nil
		}
		r.position++
		return inner
	}
	return r.parseComparison()
}

func (r *bdtInfixReader) parseComparison() bdtInfixNode {
	left := r.parseAtom()
	if left == nil {
		return nil
	}
	r.skipSpaces()
	if r.position >= len(r.text) || !isBdtComparison(r.text[r.position]) {
		return nil
	}
	op := r.text[r.position]
	r.position++
	right := r.parseAtom()
	if right == nil {
		return nil
	}
	return &bdtInfixComparisonNode{operator: op, left: left, right: right}
}

func (r *bdtInfixReader) parseAtom() bdtInfixNode {
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
		return &bdtInfixVariableNode{bank: bdtToLower(current), index: index}
	}
	if current >= '0' && current <= '9' {
		value := r.parseDigits()
		if value < 0 {
			return nil
		}
		return &bdtInfixConstantNode{value: value}
	}
	return nil
}

func (r *bdtInfixReader) parseDigits() int32 {
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

func isBdtComparison(value byte) bool {
	return value == '=' || value == '<' || value == '>' || value == '!'
}
