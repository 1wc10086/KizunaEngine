package slg

import (
	"strconv"
	"strings"
)

// 特別授業3SLG 的 SELECT_TAIL / SELECT frame[1] 字符串是"宿主命令 + 表达式"的
// 混合体：`#saki_BT = ( $#ID_BP[0,'BT値'] == 100 ) && ( $@ID_TERCOR['状態'] & 0x1 ) == 0`。
// 原引擎用一套和剧本同源的求值器处理，所以这里也照它的优先级实现：
// `&&` 低于所有比较、`||` 更低，`&`/`|` 只高于比较。不能套 C 的同级左结合——
// `X == 100 && Y & 0x1 == 0` 必须读成 `X==100 && ((Y&0x1)==0)`。

type tokenKind int

const (
	tokenEOF tokenKind = iota
	tokenNumber
	tokenName
	tokenDB
	tokenOperator
)

// dbRef 是 `$#表[角色,'参数']` / `$#表[角色,数字]` / `$@表['行名']` 的解析结果。
//
// 三种写法在同一份剧本里都有：
//
//	$#ID_BP[0,'BT値']      角色 + 参数名
//	$#ID_TC[1,37]          角色 + 行为编号（数字列）
//	$@ID_HASOBJS['重り']    只有行名
//
// 保留形状，求值时再翻译（见 dbView）。
type dbRef struct {
	table   string
	index   int32
	column  string
	rowName string
}

func (r dbRef) oneDimensional() bool { return r.rowName != "" }

type token struct {
	kind  tokenKind
	text  string
	value int32
	ref   dbRef
}

// isVariable 判断记号是不是剧本变量（#name / @name）。
func (t token) isVariable() bool {
	return t.kind == tokenName && len(t.text) > 0 && (t.text[0] == '#' || t.text[0] == '@')
}

// splitStatements 按分号切分一条宿主命令尾串。
//
// 分号也可能出现在引号内（列名、行为名都可能带），所以只能按引号外的分号切；
// 末尾多余的分号不产生空语句。
func splitStatements(text string) []string {
	var out []string
	start := 0
	var quote byte
	for index := 0; index < len(text); index++ {
		switch character := text[index]; {
		case quote != 0:
			if character == quote {
				quote = 0
			}
		case character == '\'' || character == '"':
			quote = character
		case character == ';':
			if statement := strings.TrimSpace(text[start:index]); statement != "" {
				out = append(out, statement)
			}
			start = index + 1
		}
	}
	if statement := strings.TrimSpace(text[start:]); statement != "" {
		out = append(out, statement)
	}
	return out
}

type lexer struct {
	text   string
	pos    int
	peeked *token
}

func (l *lexer) next() token {
	if l.peeked != nil {
		value := *l.peeked
		l.peeked = nil
		return value
	}
	for l.pos < len(l.text) && (l.text[l.pos] == ' ' || l.text[l.pos] == '\t') {
		l.pos++
	}
	if l.pos >= len(l.text) {
		return token{kind: tokenEOF}
	}
	character := l.text[l.pos]
	if character >= '0' && character <= '9' {
		start := l.pos
		for l.pos < len(l.text) && isNumberByte(l.text[l.pos]) {
			l.pos++
		}
		value, _ := parseInteger(l.text[start:l.pos])
		return token{kind: tokenNumber, text: l.text[start:l.pos], value: value}
	}
	// $ 前缀是"变量数据库"取值；后面必须紧跟表的写法（#ID_BP / @ID_TERCOR）。
	if character == '$' {
		if value, ok := l.scanDB(); ok {
			return value
		}
		l.pos++
		return token{kind: tokenOperator, text: "$"}
	}
	if character == '#' || character == '@' || isNameByte(character) {
		start := l.pos
		l.pos++
		// 前缀 #/@ 本身不是名字字符，必须先跨过去再扫描，
		// 否则 `#saki_energy` 会读出一个空名字并卡住词法位置。
		for l.pos < len(l.text) && isNameByte(l.text[l.pos]) {
			l.pos++
		}
		return token{kind: tokenName, text: l.text[start:l.pos]}
	}
	if l.pos+1 < len(l.text) {
		two := l.text[l.pos : l.pos+2]
		if two == "&&" || two == "||" || two == "==" || two == "!=" || two == "<=" || two == ">=" ||
			two == "+=" || two == "-=" || two == "*=" || two == "/=" || two == "|=" || two == "&=" {
			l.pos += 2
			return token{kind: tokenOperator, text: two}
		}
	}
	l.pos++
	return token{kind: tokenOperator, text: string(character)}
}

// peek 只看下一个记号，供"是赋值还是表达式"的判定使用。
// 注意要先在副本上取，再把副本的位置写回：否则 peek 之后 next 会把
// 同一个记号再读一遍，`a - b` 会变成"读两次 '-'"。
func (l *lexer) peek() token {
	if l.peeked == nil {
		saved := *l
		value := saved.next()
		*l = saved
		l.peeked = &value
	}
	return *l.peeked
}

// scanDB 读取 `$#表名[角色,'列']`、`$#表名[角色,数字]` 或 `$@表名['行名']`。
// 列名也可以不带引号（剧本里混用 `$#ID_LOG[0,膣挿入回数]`），
// 所以引号只用来界定"到哪结束"，不强制。
func (l *lexer) scanDB() (token, bool) {
	start := l.pos
	index := start + 1
	if index >= len(l.text) || (l.text[index] != '#' && l.text[index] != '@') {
		return token{}, false
	}
	index++
	tableStart := index
	for index < len(l.text) && isNameByte(l.text[index]) {
		index++
	}
	if index == tableStart {
		return token{}, false
	}
	table := l.text[tableStart:index]
	for index < len(l.text) && l.text[index] == ' ' {
		index++
	}
	if index >= len(l.text) || l.text[index] != '[' {
		return token{}, false
	}
	index++
	for index < len(l.text) && l.text[index] == ' ' {
		index++
	}
	if index < len(l.text) && (l.text[index] == '\'' || l.text[index] == '"') {
		// 一维写法：`$@ID_HASOBJS['重り']`，引号里是行名。
		rowName, after, ok := scanQuotedName(l.text, index)
		if !ok {
			return token{}, false
		}
		for after < len(l.text) && l.text[after] == ' ' {
			after++
		}
		if after >= len(l.text) || l.text[after] != ']' {
			return token{}, false
		}
		l.pos = after + 1
		return token{kind: tokenDB, text: l.text[start:l.pos], ref: dbRef{table: table, rowName: rowName}}, true
	}
	indexStart := index
	for index < len(l.text) && l.text[index] != ',' && l.text[index] != ']' {
		index++
	}
	row, ok := parseInteger(strings.TrimSpace(l.text[indexStart:index]))
	if !ok || index >= len(l.text) || l.text[index] != ',' {
		return token{}, false
	}
	index++
	column, after, ok := scanQuotedName(l.text, index)
	if !ok {
		return token{}, false
	}
	for after < len(l.text) && l.text[after] == ' ' {
		after++
	}
	if after >= len(l.text) || l.text[after] != ']' {
		return token{}, false
	}
	l.pos = after + 1
	return token{kind: tokenDB, text: l.text[start:l.pos], ref: dbRef{table: table, index: row, column: column}}, true
}

// scanQuotedName 读取列名：带引号（' 或 "）时读到配对的引号，
// 否则读到逗号或右括号。两边都可能带空白，必须一起裁掉：
// `[0, 'BT値']` 和 `[0,'BT値']` 在剧本里同时存在。
func scanQuotedName(text string, index int) (string, int, bool) {
	for index < len(text) && text[index] == ' ' {
		index++
	}
	if index < len(text) && (text[index] == '\'' || text[index] == '"') {
		quote := text[index]
		start := index + 1
		for index = start; index < len(text); index++ {
			if text[index] == quote {
				return strings.TrimSpace(text[start:index]), index + 1, true
			}
		}
		return "", 0, false
	}
	start := index
	for index < len(text) && text[index] != ',' && text[index] != ']' {
		index++
	}
	return strings.TrimSpace(text[start:index]), index, true
}

func isNumberByte(character byte) bool {
	return character >= '0' && character <= '9' ||
		character >= 'a' && character <= 'f' ||
		character >= 'A' && character <= 'F' ||
		character == 'x' || character == 'X'
}

// isNameByte 覆盖汉字、假名与符号名：名字里出现什么都按名字处理，
// 只有 ASCII 运算符与空白是分隔符。`+`/`-`/`*`/`/` 等必须是分隔符，
// 否则 `#a - 1` 会被并成一个名字。
func isNameByte(character byte) bool {
	return character >= 0x80 || character >= 'A' && character <= 'Z' ||
		character >= 'a' && character <= 'z' ||
		character >= '0' && character <= '9' ||
		character == '_' || character == '.'
}

// parseInteger 读十进制/十六进制；`-0` 这种写法在原脚本里出现过。
func parseInteger(text string) (int32, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}
	negative := false
	if text[0] == '-' {
		negative = true
		text = text[1:]
	} else if text[0] == '+' {
		text = text[1:]
	}
	value, err := strconv.ParseInt(text, 0, 32)
	if err != nil {
		if value, err = strconv.ParseInt(text, 10, 32); err != nil {
			return 0, false
		}
	}
	result := int32(value)
	if negative {
		result = -result
	}
	return result, true
}

// ------------------------------------------------------------------ 语法节点

type node interface {
	eval(v *evaluator) int32
}

type numberNode struct{ value int32 }

func (n numberNode) eval(*evaluator) int32 { return n.value }

type variableNode struct{ name string }

// eval 里未知变量按 0 处理：剧本会读很多"本作没有"的变量（移植过来的
// 脚本往往保留全套），报错会让分支整段跑不下去。
func (n variableNode) eval(v *evaluator) int32 {
	if v.host == nil {
		return 0
	}
	return v.host.Property(n.name)
}

type dbNode struct{ ref dbRef }

func (n dbNode) eval(v *evaluator) int32 {
	view := v.view()
	if view == nil {
		return 0
	}
	return view.get(n.ref)
}

type unaryNode struct {
	op   string
	body node
}

func (n unaryNode) eval(v *evaluator) int32 {
	value := n.body.eval(v)
	switch n.op {
	case "-":
		return -value
	case "!":
		return boolValue(value == 0)
	case "~":
		return ^value
	}
	return value
}

type binaryNode struct {
	op          string
	left, right node
}

func (n binaryNode) eval(v *evaluator) int32 {
	left := n.left.eval(v)
	// && / || 先短路再求右值：右值可能是除法或 DB 读，
	// 短路可以避免为不可能执行的分支建行/建列。
	switch n.op {
	case "&&":
		if left == 0 {
			return 0
		}
		return boolValue(n.right.eval(v) != 0)
	case "||":
		if left != 0 {
			return 1
		}
		return boolValue(n.right.eval(v) != 0)
	}
	right := n.right.eval(v)
	switch n.op {
	case "+":
		return left + right
	case "-":
		return left - right
	case "*":
		return left * right
	case "/":
		// 除零返回 0 而不是崩溃：脚本里的 `/10` 系数在气力/好感为 0 时会出现。
		if right == 0 {
			return 0
		}
		return left / right
	case "%":
		if right == 0 {
			return 0
		}
		return left % right
	case "&":
		return left & right
	case "|":
		return left | right
	case "==":
		return boolValue(left == right)
	case "!=":
		return boolValue(left != right)
	case "<":
		return boolValue(left < right)
	case "<=":
		return boolValue(left <= right)
	case ">":
		return boolValue(left > right)
	case ">=":
		return boolValue(left >= right)
	}
	return 0
}

func boolValue(value bool) int32 {
	if value {
		return 1
	}
	return 0
}

// ------------------------------------------------------------------ 解析

type parser struct {
	lexer *lexer
	err   error
}

// parseExpression 解析完整表达式并确保没有剩余记号。
//
// 优先级从低到高：|| < && < 比较 < & | < + - < * / % < 一元 < 原子。
// `&`/`|` 放在比较之上是这份脚本的实测结果（见文件头的注释），
// 也符合 C 语言里 `&` 高于 `==` 的规则。
func parseExpression(text string) (node, error) {
	p := &parser{lexer: &lexer{text: strings.TrimSpace(text)}}
	result := p.parseOr()
	if p.err != nil {
		return nil, p.err
	}
	if rest := p.lexer.peek(); rest.kind != tokenEOF {
		return nil, &parseError{text: text, remaining: rest.text}
	}
	return result, nil
}

type parseError struct {
	text      string
	remaining string
}

func (e *parseError) Error() string {
	return "slg: unexpected token '" + e.remaining + "' in expression '" + e.text + "'"
}

func (p *parser) fail(text string) {
	if p.err == nil {
		p.err = &parseError{text: text, remaining: text}
	}
}

func (p *parser) parseOr() node {
	left := p.parseAnd()
	for p.err == nil {
		if next := p.lexer.peek(); next.kind == tokenOperator && next.text == "||" {
			p.lexer.next()
			left = binaryNode{op: "||", left: left, right: p.parseAnd()}
			continue
		}
		break
	}
	return left
}

func (p *parser) parseAnd() node {
	left := p.parseCompare()
	for p.err == nil {
		if next := p.lexer.peek(); next.kind == tokenOperator && next.text == "&&" {
			p.lexer.next()
			left = binaryNode{op: "&&", left: left, right: p.parseCompare()}
			continue
		}
		break
	}
	return left
}

func (p *parser) parseCompare() node {
	left := p.parseBitwise()
	for p.err == nil {
		next := p.lexer.peek()
		if next.kind != tokenOperator || !isCompareOperator(next.text) {
			break
		}
		p.lexer.next()
		left = binaryNode{op: next.text, left: left, right: p.parseBitwise()}
	}
	return left
}

func isCompareOperator(operator string) bool {
	switch operator {
	case "==", "!=", "<", "<=", ">", ">=":
		return true
	}
	return false
}

func (p *parser) parseBitwise() node {
	left := p.parseAdditive()
	for p.err == nil {
		next := p.lexer.peek()
		if next.kind != tokenOperator || (next.text != "&" && next.text != "|") {
			break
		}
		p.lexer.next()
		left = binaryNode{op: next.text, left: left, right: p.parseAdditive()}
	}
	return left
}

func (p *parser) parseAdditive() node {
	left := p.parseMultiplicative()
	for p.err == nil {
		next := p.lexer.peek()
		if next.kind != tokenOperator || (next.text != "+" && next.text != "-") {
			break
		}
		p.lexer.next()
		left = binaryNode{op: next.text, left: left, right: p.parseMultiplicative()}
	}
	return left
}

func (p *parser) parseMultiplicative() node {
	left := p.parseUnary()
	for p.err == nil {
		next := p.lexer.peek()
		if next.kind != tokenOperator || (next.text != "*" && next.text != "/" && next.text != "%") {
			break
		}
		p.lexer.next()
		left = binaryNode{op: next.text, left: left, right: p.parseUnary()}
	}
	return left
}

func (p *parser) parseUnary() node {
	next := p.lexer.next()
	if next.kind == tokenOperator {
		switch next.text {
		case "-", "!", "~":
			return unaryNode{op: next.text, body: p.parseUnary()}
		case "+":
			return p.parseUnary()
		case "(":
			body := p.parseOr()
			if p.err != nil {
				return body
			}
			if closing := p.lexer.next(); closing.kind != tokenOperator || closing.text != ")" {
				p.fail(closing.text)
			}
			return body
		}
		p.fail(next.text)
		return numberNode{}
	}
	return p.parseAtom(next)
}

func (p *parser) parseAtom(next token) node {
	switch next.kind {
	case tokenNumber:
		return numberNode{value: next.value}
	case tokenName:
		if next.isVariable() {
			return variableNode{name: next.text}
		}
		p.fail(next.text)
		return numberNode{}
	case tokenDB:
		return dbNode{ref: next.ref}
	}
	p.fail(next.text)
	return numberNode{}
}
