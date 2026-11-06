package config

import (
	"fmt"
	"strconv"
	"strings"
)

// config_layout.go 解析 define.bsa 里的 system_config_ex.layout（系统设置画面）。
//
// 这个文件不是标准 JSON：键可以不写引号，键值对之间没有逗号而是靠换行分隔，
// 数组元素间的逗号可有可无，';' 开始行注释，数字还能写成 "317+30*0" 这样的
// 算式。因此分两步处理：先切成 token，再递归下降解析。
//
// 各作品的功能组差别很大（而且内部顺序常被原引擎当作语义），所以
// ConfigPage.Functions 保留原始解析树，只把界面骨架（分类、按钮、分页）
// 转成强类型，避免换作品时丢配置。

// ConfigScreen 是 system_config_ex.layout 的顶层结构。
type ConfigScreen struct {
	Base       string
	Font       ConfigFont
	Categories []ConfigCategory
	Common     []ConfigCommon
	Pages      map[string]ConfigPage
}

// ConfigFont 是 "description font" 段；Align 为空表示左对齐。
type ConfigFont struct {
	Size         int
	Weight       int
	FontFace     string
	EudcFontFace string
	Color        uint32
	Align        string
}

// ConfigCategory 是分类切换按钮及其说明。
type ConfigCategory struct {
	Layout      string
	Button      ConfigButton
	Description ConfigDescription
}

// ConfigCommon 是所有分类共用的按钮（no 是原引擎的功能编号）。
type ConfigCommon struct {
	No          int
	Button      ConfigButton
	Description ConfigDescription
}

// ConfigButton 是一张多状态按钮图的第一个状态坐标。
type ConfigButton struct {
	Name     string
	MaxState int
	X        int
	Y        int
}

// ConfigDescription 是鼠标悬停时的说明：文字和图片二选一，HasImage 用来
// 区分"没配说明"和"配了图片说明"。
type ConfigDescription struct {
	Text     string
	Image    string
	X        int
	Y        int
	HasImage bool
}

// ConfigPage 是一个分类页的底图和功能组。Functions 保留原始树。
type ConfigPage struct {
	Name      string
	Base      string
	Functions map[string]any
}

// ParseLayoutDocument 暴露同一套布局解析器，供 BGV/立ち絵等其它 layout 文件复用。
// 它们和设置画面使用同一种"无逗号、可算式、分号注释"的方言。
func ParseLayoutDocument(data []byte) (map[string]any, error) {
	return parseLayoutDocument(data)
}

// LoadConfigScreen 解析整个布局文档。缺失的可选段会得到零值，
// 只有语法错误才返回 error。
func LoadConfigScreen(data []byte) (*ConfigScreen, error) {
	root, err := parseLayoutDocument(data)
	if err != nil {
		return nil, err
	}

	screen := &ConfigScreen{
		Pages: make(map[string]ConfigPage),
	}
	screen.Base, _ = Text(root["base"])
	screen.Font = parseConfigFont(root["description font"])

	if categories, ok := Array(root["category button"]); ok {
		screen.Categories = make([]ConfigCategory, 0, len(categories))
		for _, item := range categories {
			entry, ok := Object(item)
			if !ok {
				continue
			}

			category := ConfigCategory{}
			category.Layout, _ = Text(entry["layout"])
			// "category button" 把按钮放在 image 子节点里。
			if button, ok := ParseButton(entry["image"]); ok {
				category.Button = button
			}
			category.Description = parseConfigDescription(entry["description"])
			screen.Categories = append(screen.Categories, category)
		}
	}

	if common, ok := Array(root["common button"]); ok {
		screen.Common = make([]ConfigCommon, 0, len(common))
		for _, item := range common {
			entry, ok := Object(item)
			if !ok {
				continue
			}

			button := ConfigCommon{}
			button.No, _ = Number(entry["no"])
			if image, ok := ParseButton(entry["image"]); ok {
				button.Button = image
			}
			button.Description = parseConfigDescription(entry["description"])
			screen.Common = append(screen.Common, button)
		}
	}

	if layouts, ok := Object(root["layout"]); ok {
		for name, value := range layouts {
			page := ConfigPage{
				Name:      name,
				Functions: make(map[string]any),
			}
			if entry, ok := Object(value); ok {
				page.Base, _ = Text(entry["base"])
				if functions, ok := Object(entry["function"]); ok {
					page.Functions = functions
				}
			}
			screen.Pages[name] = page
		}
	}

	return screen, nil
}

// Object 取出解析树中的对象节点。
func Object(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	return object, ok
}

// Array 取出解析树中的数组节点。
func Array(value any) ([]any, bool) {
	array, ok := value.([]any)
	return array, ok
}

// Number 取出整数节点。解析器只产生 int，这里兼容其他整数类型，
// 方便调用方处理后续填充的树。
func Number(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		if int64(int(number)) != number {
			return 0, false
		}
		return int(number), true
	case uint64:
		if number > uint64(int(^uint(0)>>1)) {
			return 0, false
		}
		return int(number), true
	default:
		return 0, false
	}
}

// Text 取出字符串节点。
func Text(value any) (string, bool) {
	text, ok := value.(string)
	return text, ok
}

// ParseButton 把树里的按钮或仪表节点转成 ConfigButton。
//
// 同一份数据有两种写法：功能组里直接写 {name:..., pos:...}，
// 而分类/通用按钮把按钮放在 image 子节点里。两种都接受，
// 调用方不用关心自己拿到的是哪一层。
func ParseButton(value any) (ConfigButton, bool) {
	object, ok := Object(value)
	if !ok {
		return ConfigButton{}, false
	}
	if _, exists := object["name"]; !exists {
		if image, ok := Object(object["image"]); ok {
			object = image
		}
	}

	name, _ := Text(object["name"])
	if len(name) == 0 {
		return ConfigButton{}, false
	}

	button := ConfigButton{Name: name}
	button.MaxState, _ = Number(object["max_state"])
	if pos, ok := Array(object["pos"]); ok {
		button.X, button.Y = layoutPosition(pos)
	}
	return button, true
}

func parseConfigFont(value any) ConfigFont {
	font := ConfigFont{}
	object, ok := Object(value)
	if !ok {
		return font
	}

	font.Size, _ = Number(object["size"])
	font.Weight, _ = Number(object["weight"])
	font.FontFace, _ = Text(object["fontface"])
	font.EudcFontFace, _ = Text(object["eudcfontface"])
	if color, ok := Number(object["color"]); ok {
		font.Color = uint32(color)
	}
	font.Align, _ = Text(object["align"])
	return font
}

func parseConfigDescription(value any) ConfigDescription {
	description := ConfigDescription{}
	object, ok := Object(value)
	if !ok {
		return description
	}

	description.Text, _ = Text(object["text"])
	description.Image, _ = Text(object["image"])
	description.HasImage = len(description.Image) > 0
	if pos, ok := Array(object["pos"]); ok {
		description.X, description.Y = layoutPosition(pos)
	}
	return description
}

func layoutPosition(pos []any) (int, int) {
	var x, y int
	if len(pos) > 0 {
		x, _ = Number(pos[0])
	}
	if len(pos) > 1 {
		y, _ = Number(pos[1])
	}
	return x, y
}

type layoutTokenKind uint8

const (
	layoutTokenEOF layoutTokenKind = iota
	layoutTokenLBrace
	layoutTokenRBrace
	layoutTokenLBracket
	layoutTokenRBracket
	layoutTokenColon
	layoutTokenComma
	layoutTokenString
	layoutTokenNumber
	layoutTokenWord
	layoutTokenPlus
	layoutTokenMinus
	layoutTokenStar
	layoutTokenSlash
	layoutTokenPercent
	layoutTokenLParen
	layoutTokenRParen
)

func (kind layoutTokenKind) String() string {
	switch kind {
	case layoutTokenEOF:
		return "end of file"
	case layoutTokenLBrace:
		return "'{'"
	case layoutTokenRBrace:
		return "'}'"
	case layoutTokenLBracket:
		return "'['"
	case layoutTokenRBracket:
		return "']'"
	case layoutTokenColon:
		return "':'"
	case layoutTokenComma:
		return "','"
	case layoutTokenString:
		return "string"
	case layoutTokenNumber:
		return "number"
	case layoutTokenWord:
		return "word"
	case layoutTokenPlus:
		return "'+'"
	case layoutTokenMinus:
		return "'-'"
	case layoutTokenStar:
		return "'*'"
	case layoutTokenSlash:
		return "'/'"
	case layoutTokenPercent:
		return "'%'"
	case layoutTokenLParen:
		return "'('"
	case layoutTokenRParen:
		return "')'"
	default:
		return "unknown token"
	}
}

type layoutToken struct {
	kind layoutTokenKind
	text string
	line int
}

// tokenizeLayout 丢掉空白和 ';' 注释，只保留语法 token。
// 换行本身没有语义，键值对靠 "key:" 的形态分隔。
func tokenizeLayout(source string) ([]layoutToken, error) {
	tokens := make([]layoutToken, 0, len(source)/8)
	line := 1

	for index := 0; index < len(source); {
		character := source[index]

		switch {
		case character == '\n':
			line++
			index++
		case character == '\r' || character == ' ' || character == '\t' ||
			character == '\f' || character == '\v':
			index++
		case character == ';':
			for index < len(source) && source[index] != '\n' {
				index++
			}
		case character == '{':
			tokens = append(tokens, layoutToken{kind: layoutTokenLBrace, line: line})
			index++
		case character == '}':
			tokens = append(tokens, layoutToken{kind: layoutTokenRBrace, line: line})
			index++
		case character == '[':
			tokens = append(tokens, layoutToken{kind: layoutTokenLBracket, line: line})
			index++
		case character == ']':
			tokens = append(tokens, layoutToken{kind: layoutTokenRBracket, line: line})
			index++
		case character == ':':
			tokens = append(tokens, layoutToken{kind: layoutTokenColon, line: line})
			index++
		case character == ',':
			tokens = append(tokens, layoutToken{kind: layoutTokenComma, line: line})
			index++
		case character == '+':
			tokens = append(tokens, layoutToken{kind: layoutTokenPlus, line: line})
			index++
		case character == '-':
			tokens = append(tokens, layoutToken{kind: layoutTokenMinus, line: line})
			index++
		case character == '*':
			tokens = append(tokens, layoutToken{kind: layoutTokenStar, line: line})
			index++
		case character == '/':
			tokens = append(tokens, layoutToken{kind: layoutTokenSlash, line: line})
			index++
		case character == '%':
			tokens = append(tokens, layoutToken{kind: layoutTokenPercent, line: line})
			index++
		case character == '(':
			tokens = append(tokens, layoutToken{kind: layoutTokenLParen, line: line})
			index++
		case character == ')':
			tokens = append(tokens, layoutToken{kind: layoutTokenRParen, line: line})
			index++
		case character == '"':
			text, next, err := scanLayoutString(source, index, line)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, layoutToken{kind: layoutTokenString, text: text, line: line})
			index = next
		case character >= '0' && character <= '9':
			start := index
			index++
			if character == '0' && index < len(source) &&
				(source[index] == 'x' || source[index] == 'X') {
				index++
				digits := index
				for index < len(source) && isLayoutHexDigit(source[index]) {
					index++
				}
				if index == digits {
					return nil, layoutError(line, "hex number without digits")
				}
			} else {
				for index < len(source) && source[index] >= '0' && source[index] <= '9' {
					index++
				}
				// 小数（museum_buex_layout 里的 size: .5 / 0.5）。
				// 布局解析只需要跳过这些值，不参与坐标计算。
				if index < len(source) && source[index] == '.' &&
					index+1 < len(source) && source[index+1] >= '0' && source[index+1] <= '9' {
					index++
					for index < len(source) && source[index] >= '0' && source[index] <= '9' {
						index++
					}
				}
			}
			tokens = append(tokens, layoutToken{
				kind: layoutTokenNumber,
				text: source[start:index],
				line: line,
			})
		case character == '.' && index+1 < len(source) &&
			source[index+1] >= '0' && source[index+1] <= '9':
			start := index
			index++
			for index < len(source) && source[index] >= '0' && source[index] <= '9' {
				index++
			}
			tokens = append(tokens, layoutToken{
				kind: layoutTokenNumber,
				text: source[start:index],
				line: line,
			})
		case isLayoutWordStart(character):
			start := index
			for index < len(source) && isLayoutWordByte(source[index]) {
				index++
			}
			tokens = append(tokens, layoutToken{
				kind: layoutTokenWord,
				text: source[start:index],
				line: line,
			})
		default:
			return nil, layoutError(line, "unexpected character %q", character)
		}
	}

	tokens = append(tokens, layoutToken{kind: layoutTokenEOF, line: line})
	return tokens, nil
}

func scanLayoutString(source string, index, line int) (string, int, error) {
	var builder strings.Builder
	index++ // 跳过起始引号

	for index < len(source) {
		character := source[index]
		switch character {
		case '"':
			return builder.String(), index + 1, nil
		case '\\':
			index++
			if index >= len(source) {
				return "", index, layoutError(line, "unterminated string")
			}
			escaped := source[index]
			switch escaped {
			case 'n':
				builder.WriteByte('\n')
			case 'r':
				builder.WriteByte('\r')
			case 't':
				builder.WriteByte('\t')
			case '"':
				builder.WriteByte('"')
			case '\\':
				builder.WriteByte('\\')
			default:
				// 未知转义保留原样，避免把说明文字里的反斜杠吃掉。
				builder.WriteByte('\\')
				builder.WriteByte(escaped)
			}
			index++
		default:
			builder.WriteByte(character)
			index++
		}
	}

	return "", index, layoutError(line, "unterminated string")
}

func isLayoutHexDigit(character byte) bool {
	return character >= '0' && character <= '9' ||
		character >= 'a' && character <= 'f' ||
		character >= 'A' && character <= 'F'
}

func isLayoutWordStart(character byte) bool {
	return character == '_' ||
		character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= 0x80
}

func isLayoutWordByte(character byte) bool {
	return isLayoutWordStart(character) ||
		character >= '0' && character <= '9' ||
		character == '-' || character == '.'
}

func layoutError(line int, format string, args ...any) error {
	return fmt.Errorf("layout: line %d: %s", line, fmt.Sprintf(format, args...))
}

type layoutParser struct {
	tokens []layoutToken
	index  int
}

func parseLayoutDocument(data []byte) (map[string]any, error) {
	source := strings.TrimPrefix(string(data), "\ufeff")
	tokens, err := tokenizeLayout(source)
	if err != nil {
		return nil, err
	}

	parser := &layoutParser{tokens: tokens}
	if token := parser.peek(); token.kind != layoutTokenLBrace {
		return nil, layoutError(token.line, "expected '{' at the start, got %s", token.kind)
	}

	root, err := parser.parseObject()
	if err != nil {
		return nil, err
	}

	parser.skipSeparators()
	if token := parser.peek(); token.kind != layoutTokenEOF {
		return nil, layoutError(token.line, "unexpected %s after the document", token.kind)
	}

	return root, nil
}

func (p *layoutParser) peek() layoutToken {
	return p.tokens[p.index]
}

func (p *layoutParser) advance() layoutToken {
	token := p.tokens[p.index]
	if p.index < len(p.tokens)-1 {
		p.index++
	}
	return token
}

// skipSeparators 跳过可选的逗号；换行和注释在切词阶段已经消失。
func (p *layoutParser) skipSeparators() {
	for p.peek().kind == layoutTokenComma {
		p.advance()
	}
}

func (p *layoutParser) parseObject() (map[string]any, error) {
	open := p.advance()
	if open.kind != layoutTokenLBrace {
		return nil, layoutError(open.line, "expected '{', got %s", open.kind)
	}

	object := make(map[string]any)

	for {
		p.skipSeparators()
		token := p.peek()
		switch token.kind {
		case layoutTokenRBrace:
			p.advance()
			return object, nil
		case layoutTokenEOF:
			return nil, layoutError(token.line, "unterminated object")
		case layoutTokenString, layoutTokenWord:
			p.advance()
		default:
			return nil, layoutError(token.line, "expected a key, got %s", token.kind)
		}

		if colon := p.advance(); colon.kind != layoutTokenColon {
			return nil, layoutError(colon.line, "expected ':' after key %q, got %s",
				token.text, colon.kind)
		}

		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		object[token.text] = value
	}
}

func (p *layoutParser) parseArray() ([]any, error) {
	open := p.advance()
	if open.kind != layoutTokenLBracket {
		return nil, layoutError(open.line, "expected '[', got %s", open.kind)
	}

	var array []any
	for {
		p.skipSeparators()
		token := p.peek()
		switch token.kind {
		case layoutTokenRBracket:
			p.advance()
			return array, nil
		case layoutTokenEOF:
			return nil, layoutError(token.line, "unterminated array")
		}

		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		array = append(array, value)
	}
}

func (p *layoutParser) parseValue() (any, error) {
	token := p.peek()

	switch token.kind {
	case layoutTokenLBrace:
		return p.parseObject()
	case layoutTokenLBracket:
		return p.parseArray()
	case layoutTokenString:
		p.advance()
		return token.text, nil
	case layoutTokenWord:
		p.advance()
		return token.text, nil
	case layoutTokenNumber, layoutTokenPlus, layoutTokenMinus, layoutTokenLParen:
		return p.parseExpression()
	default:
		return nil, layoutError(token.line, "expected a value, got %s", token.kind)
	}
}

func (p *layoutParser) parseExpression() (int, error) {
	return p.parseAdditive()
}

func (p *layoutParser) parseAdditive() (int, error) {
	value, err := p.parseMultiplicative()
	if err != nil {
		return 0, err
	}

	for {
		switch p.peek().kind {
		case layoutTokenPlus:
			p.advance()
			right, err := p.parseMultiplicative()
			if err != nil {
				return 0, err
			}
			value += right
		case layoutTokenMinus:
			p.advance()
			right, err := p.parseMultiplicative()
			if err != nil {
				return 0, err
			}
			value -= right
		default:
			return value, nil
		}
	}
}

func (p *layoutParser) parseMultiplicative() (int, error) {
	value, err := p.parseUnary()
	if err != nil {
		return 0, err
	}

	for {
		operator := p.peek()
		switch operator.kind {
		case layoutTokenStar:
			p.advance()
			right, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			value *= right
		case layoutTokenSlash:
			p.advance()
			right, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, layoutError(operator.line, "division by zero")
			}
			value /= right
		case layoutTokenPercent:
			p.advance()
			right, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, layoutError(operator.line, "division by zero")
			}
			value %= right
		default:
			return value, nil
		}
	}
}

func (p *layoutParser) parseUnary() (int, error) {
	switch p.peek().kind {
	case layoutTokenPlus:
		p.advance()
		return p.parseUnary()
	case layoutTokenMinus:
		p.advance()
		value, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		return -value, nil
	default:
		return p.parsePrimary()
	}
}

func (p *layoutParser) parsePrimary() (int, error) {
	token := p.peek()

	switch token.kind {
	case layoutTokenNumber:
		p.advance()
		return parseLayoutInteger(token)
	case layoutTokenLParen:
		p.advance()
		value, err := p.parseExpression()
		if err != nil {
			return 0, err
		}
		if close := p.advance(); close.kind != layoutTokenRParen {
			return 0, layoutError(close.line, "expected ')', got %s", close.kind)
		}
		return value, nil
	default:
		return 0, layoutError(token.line, "expected a number, got %s", token.kind)
	}
}

func parseLayoutInteger(token layoutToken) (int, error) {
	text := token.text

	if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") {
		value, err := strconv.ParseUint(text[2:], 16, 64)
		if err != nil {
			return 0, layoutError(token.line, "invalid number %q", text)
		}
		if value > uint64(int(^uint(0)>>1)) {
			// 颜色写成 0xffffffff。32 位平台上 int 放不下，但按 uint32
			// 截断后 ConfigFont.Color 仍能还原原值，不要因此拒绝整个文件。
			if value <= uint64(^uint32(0)) {
				return int(uint32(value)), nil
			}
			return 0, layoutError(token.line, "number %q overflows int", text)
		}
		return int(value), nil
	}

	if strings.Contains(text, ".") {
		// 小数只出现在显示尺寸等非坐标字段；截断成整数，保证整份布局仍可解析。
		value, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return 0, layoutError(token.line, "invalid number %q", text)
		}
		return int(value), nil
	}

	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil || int64(int(value)) != value {
		return 0, layoutError(token.line, "invalid number %q", text)
	}
	return int(value), nil
}
