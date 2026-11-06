package script

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"

	"kizuna/engine/core"
)

const maxCachedExpressions = 1024

const maxLoggedExpressions = 1024

type EmonVariableSource interface {
	Lookup(name string) EmonValue
}

var (
	tokenCacheGate = &sync.Mutex{}
	tokenCache     = map[string][]token{}

	logGate           = &sync.Mutex{}
	loggedExpressions = map[string]struct{}{}
)

func Evaluate(expression string, variables EmonVariableSource) EmonValue {
	if expression == "" {
		return EmonValueFromInt(0)
	}

	return evaluateGuarded(expression, variables)
}

func EvaluateCondition(expression string, variables EmonVariableSource) bool {
	return isTruthy(Evaluate(expression, variables))
}

func evaluateGuarded(expression string, variables EmonVariableSource) (value EmonValue) {
	defer func() {
		if r := recover(); r != nil {
			if exception, ok := r.(*expressionException); ok {
				logFailureOnce(expression, describeSyntaxFailure(expression, exception))
				value = EmonValueFromString(expression)
				return
			}

			logFailureOnce(expression, panicMessage(r))
			value = EmonValueFromString(expression)
		}
	}()

	return newEvaluator(getTokens(expression), variables).run()
}

func isTruthy(value EmonValue) bool {
	if value.IsIdentifier() {
		return false
	}

	if value.IsInteger() {
		return value.AsInt() != 0
	}

	return len(value.AsString()) != 0
}

func getTokens(expression string) []token {
	tokenCacheGate.Lock()
	if cached, exists := tokenCache[expression]; exists {
		tokenCacheGate.Unlock()
		return cached
	}
	tokenCacheGate.Unlock()

	tokens := tokenize(expression)

	tokenCacheGate.Lock()
	if len(tokenCache) >= maxCachedExpressions {
		tokenCache = map[string][]token{}
	}
	tokenCache[expression] = tokens
	tokenCacheGate.Unlock()

	return tokens
}

func logFailureOnce(expression string, reason string) {
	logGate.Lock()
	_, logged := loggedExpressions[expression]
	first := !logged
	if first {
		loggedExpressions[expression] = struct{}{}
		if len(loggedExpressions) > maxLoggedExpressions {
			loggedExpressions = map[string]struct{}{expression: {}}
		}
	}
	logGate.Unlock()

	if first {
		core.Logf("emon: failed to evaluate '%s': %s; raw text kept", expression, reason)
	}
}

func describeSyntaxFailure(expression string, exception *expressionException) string {
	reason := exception.reason
	if hasDuplicatedQuotes(expression) {
		reason += "; an empty string literal abuts the next value (duplicated quotes)"
	}

	return "invalid syntax: " + reason
}

func hasDuplicatedQuotes(expression string) bool {
	runes := []rune(expression)
	for i := 0; i+1 < len(runes); i++ {
		quote := runes[i]
		if quote != '"' && quote != '＂' {
			continue
		}

		if runes[i+1] != quote {
			continue
		}

		after := rune(0)
		if i+2 < len(runes) {
			after = runes[i+2]
		}

		if after != 0 && !isExpressionDelimiter(after) {
			return true
		}

		i++
	}

	return false
}

func isExpressionDelimiter(c rune) bool {
	switch c {
	case ' ', '\t', '\r', '\n', '\u3000', '"', '＂', '(', ')', ',',
		'=', '+', '-', '*', '/', '%', '!', '<', '>', '&', '|', '^', '~':
		return true
	}

	return false
}

func CommonPrefixLength(left string, right string) int {
	leftUnits := utf16.Encode([]rune(left))
	rightUnits := utf16.Encode([]rune(right))
	length := len(leftUnits)
	if len(rightUnits) < length {
		length = len(rightUnits)
	}

	for i := 0; i < length; i++ {
		if leftUnits[i] != rightUnits[i] {
			return i
		}
	}

	return length
}

func compareOrdinal(left string, right string) int {
	leftUnits := utf16.Encode([]rune(left))
	rightUnits := utf16.Encode([]rune(right))
	length := len(leftUnits)
	if len(rightUnits) < length {
		length = len(rightUnits)
	}

	for i := 0; i < length; i++ {
		if leftUnits[i] == rightUnits[i] {
			continue
		}

		if leftUnits[i] < rightUnits[i] {
			return -1
		}

		return 1
	}

	if len(leftUnits) < len(rightUnits) {
		return -1
	}

	if len(leftUnits) > len(rightUnits) {
		return 1
	}

	return 0
}

func panicMessage(recovered any) string {
	if err, ok := recovered.(error); ok {
		return err.Error()
	}

	return fmt.Sprint(recovered)
}

type tokenKind int

const (
	tokenEnd tokenKind = iota
	tokenNumber
	tokenReal
	tokenText
	tokenName
	tokenOperator
	tokenOpen
	tokenClose
)

type token struct {
	kind   tokenKind
	text   string
	number int32
	real   float64
}

type expressionException struct {
	reason string
}

func (e *expressionException) Error() string {
	return e.reason
}

func tokenize(text string) []token {
	runes := []rune(text)
	tokens := make([]token, 0, 16)
	i := 0
	for i < len(runes) {
		c := runes[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\u3000':
			i++
		case c == '"' || c == '＂':
			tokens = append(tokens, readStringToken(runes, &i))
		case c == '(':
			tokens = append(tokens, token{kind: tokenOpen, text: "("})
			i++
		case c == ')':
			tokens = append(tokens, token{kind: tokenClose, text: ")"})
			i++
		case isOperatorStart(c):
			tokens = append(tokens, readOperatorToken(runes, &i))
		default:
			tokens = append(tokens, readBareToken(runes, &i))
		}
	}

	tokens = append(tokens, token{kind: tokenEnd})
	return tokens
}

func isOperatorStart(c rune) bool {
	switch c {
	case ',', '=', '+', '-', '*', '/', '%', '!', '<', '>', '&', '|', '^', '~':
		return true
	}

	return false
}

func readOperatorToken(runes []rune, i *int) token {
	c := runes[*i]
	next := rune(0)
	if *i+1 < len(runes) {
		next = runes[*i+1]
	}

	switch c {
	case '=':
		if next == '=' {
			*i += 2
			return token{kind: tokenOperator, text: "=="}
		}

		*i++
		return token{kind: tokenOperator, text: "="}
	case '!':
		if next == '=' {
			*i += 2
			return token{kind: tokenOperator, text: "!="}
		}

		*i++
		return token{kind: tokenOperator, text: "!"}
	case '<':
		switch next {
		case '=':
			*i += 2
			return token{kind: tokenOperator, text: "<="}
		case '>':
			*i += 2
			return token{kind: tokenOperator, text: "!="}
		case '<':
			*i += 2
			return token{kind: tokenOperator, text: "<<"}
		}

		*i++
		return token{kind: tokenOperator, text: "<"}
	case '>':
		switch next {
		case '=':
			*i += 2
			return token{kind: tokenOperator, text: ">="}
		case '>':
			*i += 2
			return token{kind: tokenOperator, text: ">>"}
		}

		*i++
		return token{kind: tokenOperator, text: ">"}
	case '&':
		if next == '&' {
			*i += 2
			return token{kind: tokenOperator, text: "&&"}
		}

		*i++
		return token{kind: tokenOperator, text: "&"}
	case '|':
		if next == '|' {
			*i += 2
			return token{kind: tokenOperator, text: "||"}
		}

		*i++
		return token{kind: tokenOperator, text: "|"}
	case '^':
		if next == '*' {
			if *i+2 < len(runes) && runes[*i+2] == '=' {
				*i += 3
				return token{kind: tokenOperator, text: "^*="}
			}

			*i += 2
			return token{kind: tokenOperator, text: "^*"}
		}

		if next == '=' {
			*i += 2
			return token{kind: tokenOperator, text: "^="}
		}

		if next == 's' || next == 'c' || next == 't' || next == 'i' || next == 'a' {
			*i += 2
			return token{kind: tokenOperator, text: "^" + string(next)}
		}

		*i++
		return token{kind: tokenOperator, text: "^"}
	case ',':
		panic(&expressionException{reason: "unexpected ',' in expression"})
	}

	*i++
	return token{kind: tokenOperator, text: string(c)}
}

func readStringToken(runes []rune, i *int) token {
	var builder strings.Builder
	*i++
	for {
		if *i >= len(runes) {
			panic(&expressionException{reason: "unterminated string literal"})
		}

		c := runes[*i]
		if c == '"' || c == '＂' {
			*i++
			return token{kind: tokenText, text: builder.String()}
		}

		if c == '\\' && *i+1 < len(runes) {
			*i++
			builder.WriteRune(runes[*i])
			*i++
		} else {
			builder.WriteRune(c)
			*i++
		}
	}
}

func readBareToken(runes []rune, i *int) token {
	start := *i
	for *i < len(runes) && !isExpressionDelimiter(runes[*i]) {
		*i++
	}

	raw := string(runes[start:*i])
	if *i > start && isNumberStart(runes[start]) {
		normalized := normalizeDigits(raw)
		if strings.ContainsRune(normalized, '.') {
			if real, ok := parseFloatInvariant(normalized); ok {
				return token{kind: tokenReal, text: raw, real: real}
			}
		}

		return token{kind: tokenNumber, text: raw, number: parseNumber(raw)}
	}

	return token{kind: tokenName, text: raw}
}

func isNumberStart(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= '０' && c <= '９')
}

func parseNumber(raw string) int32 {
	normalized := normalizeDigits(raw)
	if len(normalized) > 2 && hasHexPrefix(normalized) {
		if hex, ok := parseHexUint32(normalized[2:]); ok {
			return int32(hex)
		}
	}

	if value, ok := parseIntInvariant(normalized); ok {
		return value
	}

	if wide, ok := parseInt64Invariant(normalized); ok {
		return int32(wide)
	}

	return 0
}

func normalizeDigits(raw string) string {
	fullWidth := false
	for _, c := range raw {
		if (c >= '０' && c <= '９') || (c >= 'Ａ' && c <= 'Ｆ') ||
			(c >= 'ａ' && c <= 'ｆ') || c == 'Ｘ' || c == 'ｘ' {
			fullWidth = true
			break
		}
	}

	if !fullWidth {
		return raw
	}

	var builder strings.Builder
	builder.Grow(len(raw))
	for _, c := range raw {
		switch {
		case c >= '０' && c <= '９':
			builder.WriteRune('0' + (c - '０'))
		case c >= 'Ａ' && c <= 'Ｆ':
			builder.WriteRune('A' + (c - 'Ａ'))
		case c >= 'ａ' && c <= 'ｆ':
			builder.WriteRune('a' + (c - 'ａ'))
		case c == 'Ｘ' || c == 'ｘ':
			builder.WriteRune('x')
		default:
			builder.WriteRune(c)
		}
	}

	return builder.String()
}

type evaluator struct {
	tokens    []token
	variables EmonVariableSource
	pos       int
}

func newEvaluator(tokens []token, variables EmonVariableSource) *evaluator {
	return &evaluator{tokens: tokens, variables: variables}
}

func (e *evaluator) run() EmonValue {
	value := e.parseLogicalOr()
	if e.current().kind != tokenEnd {
		panic(e.unexpected("expected the end of the expression"))
	}

	return value.toValue()
}

func (e *evaluator) current() token {
	return e.tokens[e.pos]
}

func (e *evaluator) unexpected(expectation string) *expressionException {
	return &expressionException{
		reason: fmt.Sprintf("%s, but found %s at token %d", expectation, describeToken(e.current()), e.pos),
	}
}

func describeToken(t token) string {
	switch t.kind {
	case tokenText:
		return "string \"" + t.text + "\""
	case tokenNumber:
		return "number " + t.text
	case tokenReal:
		return "number " + t.text
	case tokenName:
		return "name " + t.text
	case tokenOperator:
		return "operator " + t.text
	case tokenOpen:
		return "'('"
	case tokenClose:
		return "')'"
	}

	return "the end of the expression"
}

func (e *evaluator) match(op string) bool {
	if e.current().kind == tokenOperator && e.current().text == op {
		e.pos++
		return true
	}

	return false
}

func equalsNumber(left exprNumber, right exprNumber) bool {
	if left.isIdentifier && right.isIdentifier {
		return left.asString() == right.asString()
	}

	leftInteger, leftOk := left.tryGetInteger()
	rightInteger, rightOk := right.tryGetInteger()
	if leftOk && rightOk {
		return leftInteger == rightInteger
	}

	return left.asString() == right.asString()
}

func compareNumber(left exprNumber, right exprNumber) int {
	if left.isIdentifier && right.isIdentifier {
		return compareOrdinal(left.asString(), right.asString())
	}

	leftInteger, leftOk := left.tryGetInteger()
	rightInteger, rightOk := right.tryGetInteger()
	if leftOk && rightOk {
		if leftInteger < rightInteger {
			return -1
		}
		if leftInteger > rightInteger {
			return 1
		}
		return 0
	}

	return compareOrdinal(left.asString(), right.asString())
}

func (e *evaluator) parseLogicalOr() exprNumber {
	value := e.parseLogicalAnd()
	for e.match("||") {
		right := e.parseLogicalAnd()
		if value.isTruthy() || right.isTruthy() {
			value = fromInt(1)
		} else {
			value = fromInt(0)
		}
	}

	return value
}

func (e *evaluator) parseLogicalAnd() exprNumber {
	value := e.parseEquality()
	for e.match("&&") {
		right := e.parseEquality()
		if value.isTruthy() && right.isTruthy() {
			value = fromInt(1)
		} else {
			value = fromInt(0)
		}
	}

	return value
}

func (e *evaluator) parseBitOr() exprNumber {
	value := e.parseBitXor()
	for e.match("|") {
		right := e.parseBitXor()
		value = fromInt(value.asInteger() | right.asInteger())
	}

	return value
}

func (e *evaluator) parseBitXor() exprNumber {
	value := e.parseBitAnd()
	for e.match("^") {
		right := e.parseBitAnd()
		value = fromInt(value.asInteger() ^ right.asInteger())
	}

	return value
}

func (e *evaluator) parseBitAnd() exprNumber {
	value := e.parseShift()
	for e.match("&") {
		right := e.parseShift()
		leftInteger, leftOk := value.tryGetInteger()
		rightInteger, rightOk := right.tryGetInteger()
		if leftOk && rightOk {
			value = fromInt(leftInteger & rightInteger)
		} else {
			value = fromInt(int32(CommonPrefixLength(value.asString(), right.asString())))
		}
	}

	return value
}

func (e *evaluator) parseEquality() exprNumber {
	value := e.parseRelational()
	for {
		if e.match("==") || e.match("=") {
			right := e.parseRelational()
			if equalsNumber(value, right) {
				value = fromInt(1)
			} else {
				value = fromInt(0)
			}
		} else if e.match("!=") {
			right := e.parseRelational()
			if equalsNumber(value, right) {
				value = fromInt(0)
			} else {
				value = fromInt(1)
			}
		} else {
			return value
		}
	}
}

func (e *evaluator) parseRelational() exprNumber {
	value := e.parseBitOr()
	for {
		if e.match("<=") {
			right := e.parseBitOr()
			if compareNumber(value, right) <= 0 {
				value = fromInt(1)
			} else {
				value = fromInt(0)
			}
		} else if e.match(">=") {
			right := e.parseBitOr()
			if compareNumber(value, right) >= 0 {
				value = fromInt(1)
			} else {
				value = fromInt(0)
			}
		} else if e.match("<") {
			right := e.parseBitOr()
			if compareNumber(value, right) < 0 {
				value = fromInt(1)
			} else {
				value = fromInt(0)
			}
		} else if e.match(">") {
			right := e.parseBitOr()
			if compareNumber(value, right) > 0 {
				value = fromInt(1)
			} else {
				value = fromInt(0)
			}
		} else {
			return value
		}
	}
}

func (e *evaluator) parseShift() exprNumber {
	value := e.parseAdditive()
	for {
		if e.match("<<") {
			right := e.parseAdditive()
			value = fromInt(value.asInteger() << (uint32(right.asInteger()) & 31))
		} else if e.match(">>") {
			right := e.parseAdditive()
			value = fromInt(value.asInteger() >> (uint32(right.asInteger()) & 31))
		} else {
			return value
		}
	}
}

func (e *evaluator) parseAdditive() exprNumber {
	value := e.parseMultiplicative()
	for {
		if e.match("+") {
			right := e.parseMultiplicative()
			value = value.add(right)
		} else if e.match("-") {
			right := e.parseMultiplicative()
			value = value.subtract(right)
		} else {
			return value
		}
	}
}

func (e *evaluator) parseMultiplicative() exprNumber {
	value := e.parsePower()
	for {
		if e.match("*") {
			right := e.parsePower()
			value = value.multiply(right)
		} else if e.match("/") {
			right := e.parsePower()
			value = value.divide(right)
		} else if e.match("%") {
			right := e.parsePower()
			value = value.modulo(right)
		} else {
			return value
		}
	}
}

func (e *evaluator) parsePower() exprNumber {
	value := e.parseUnary()
	for e.match("^*") {
		right := e.parseUnary()
		value = fromReal(math.Pow(value.asReal(), right.asReal()))
	}

	return value
}

func (e *evaluator) parseUnary() exprNumber {
	if e.match("-") {
		return fromInt(-e.parseUnary().asInteger())
	}

	if e.match("+") {
		return e.parseUnary()
	}

	if e.match("~") {
		return fromInt(^e.parseUnary().asInteger())
	}

	if e.match("!") {
		if e.parseUnary().isTruthy() {
			return fromInt(0)
		}

		return fromInt(1)
	}

	if e.match("^s") {
		return fromReal(math.Sin(degrees(e.parseUnary())))
	}

	if e.match("^c") {
		return fromReal(math.Cos(degrees(e.parseUnary())))
	}

	if e.match("^t") {
		return fromReal(math.Tan(degrees(e.parseUnary())))
	}

	if e.match("^i") {
		return fromInt(e.parseUnary().asInteger())
	}

	if e.match("^a") {
		return absolute(e.parseUnary())
	}

	return e.parsePrimary()
}

func degrees(value exprNumber) float64 {
	return value.asReal() * math.Pi / 180
}

func absolute(value exprNumber) exprNumber {
	if value.isReal {
		return fromReal(math.Abs(value.asReal()))
	}

	integer := value.asInteger()
	if integer < 0 {
		return fromInt(-integer)
	}

	return fromInt(integer)
}

func (e *evaluator) parsePrimary() exprNumber {
	current := e.current()
	switch current.kind {
	case tokenNumber:
		e.pos++
		return fromInt(current.number)
	case tokenReal:
		e.pos++
		return fromReal(current.real)
	case tokenText:
		e.pos++
		return fromString(current.text)
	case tokenName:
		e.pos++
		return fromValue(e.variables.Lookup(current.text))
	case tokenOpen:
		e.pos++
		value := e.parseLogicalOr()
		if e.current().kind != tokenClose {
			panic(e.unexpected("expected ')' to close a parenthesized value"))
		}

		e.pos++
		return value
	}

	panic(e.unexpected("expected a value"))
}

type exprNumber struct {
	integer      int32
	real         float64
	text         string
	hasText      bool
	isReal       bool
	isIdentifier bool
}

func fromInt(value int32) exprNumber {
	return exprNumber{integer: value}
}

func fromReal(value float64) exprNumber {
	return exprNumber{real: value, isReal: true}
}

func fromString(value string) exprNumber {
	return exprNumber{text: value, hasText: true}
}

func fromIdentifier(value string) exprNumber {
	return exprNumber{text: value, hasText: true, isIdentifier: true}
}

func fromValue(value EmonValue) exprNumber {
	if value.IsIdentifier() {
		return fromIdentifier(value.AsString())
	}

	if value.IsInteger() {
		return fromInt(value.AsInt())
	}

	return fromString(value.AsString())
}

func (n exprNumber) asInteger() int32 {
	if !n.hasText {
		if n.isReal {
			return truncateReal(n.real)
		}

		return n.integer
	}

	text := strings.TrimSpace(n.text)
	if text == "" {
		return 0
	}

	if len(text) > 2 && hasHexPrefix(text) {
		if hex, ok := parseHexUint32(text[2:]); ok {
			return int32(hex)
		}

		return 0
	}

	if value, ok := parseIntInvariant(text); ok {
		return value
	}

	return 0
}

func (n exprNumber) asReal() float64 {
	if !n.hasText {
		if n.isReal {
			return n.real
		}

		return float64(n.integer)
	}

	return float64(n.asInteger())
}

func (n exprNumber) asString() string {
	if n.hasText {
		return n.text
	}

	if n.isReal {
		return formatReal(n.real)
	}

	return strconv.FormatInt(int64(n.integer), 10)
}

func (n exprNumber) tryGetInteger() (int32, bool) {
	if !n.hasText {
		if n.isReal {
			return truncateReal(n.real), true
		}

		return n.integer, true
	}

	if n.isIdentifier {
		return 0, true
	}

	text := strings.TrimSpace(n.text)
	if text == "" {
		return 0, true
	}

	if len(text) > 2 && hasHexPrefix(text) {
		if hex, ok := parseHexUint32(text[2:]); ok {
			return int32(hex), true
		}
	}

	if value, ok := parseIntInvariant(text); ok {
		return value, true
	}

	return 0, false
}

func (n exprNumber) isTruthy() bool {
	if n.isIdentifier {
		return false
	}

	if n.hasText {
		return len(n.text) != 0
	}

	if n.isReal {
		return n.real != 0
	}

	return n.integer != 0
}

func (n exprNumber) add(other exprNumber) exprNumber {
	if !n.hasText && !other.hasText {
		if n.isReal || other.isReal {
			return fromReal(n.asReal() + other.asReal())
		}

		return fromInt(n.integer + other.integer)
	}

	return fromString(n.asString() + other.asString())
}

func (n exprNumber) subtract(other exprNumber) exprNumber {
	if n.isReal || other.isReal {
		return fromReal(n.asReal() - other.asReal())
	}

	return fromInt(n.asInteger() - other.asInteger())
}

func (n exprNumber) multiply(other exprNumber) exprNumber {
	if n.isReal || other.isReal {
		return fromReal(n.asReal() * other.asReal())
	}

	return fromInt(n.asInteger() * other.asInteger())
}

func (n exprNumber) divide(other exprNumber) exprNumber {
	if n.isReal || other.isReal {
		divisor := other.asReal()
		if divisor == 0 {
			return fromReal(0)
		}

		return fromReal(n.asReal() / divisor)
	}

	divisor := other.asInteger()
	if divisor == 0 {
		return fromInt(0)
	}

	dividend := n.asInteger()
	if divisor == -1 && dividend == minInt32 {
		return fromInt(minInt32)
	}

	return fromInt(dividend / divisor)
}

func (n exprNumber) modulo(other exprNumber) exprNumber {
	if n.isReal || other.isReal {
		divisor := other.asReal()
		if divisor == 0 {
			return fromReal(0)
		}

		return fromReal(math.Mod(n.asReal(), divisor))
	}

	divisor := other.asInteger()
	if divisor == 0 {
		return fromInt(0)
	}

	return fromInt(n.asInteger() % divisor)
}

func (n exprNumber) toValue() EmonValue {
	if n.hasText {
		if n.isIdentifier {
			return EmonValueFromIdentifier(n.text)
		}

		return EmonValueFromString(n.text)
	}

	if n.isReal {
		return EmonValueFromInt(truncateReal(n.real))
	}

	return EmonValueFromInt(n.integer)
}
