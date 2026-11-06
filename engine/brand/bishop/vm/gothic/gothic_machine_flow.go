package gothic

import (
	"strings"

	"kizuna/engine/core"
)

var gothicSystemVariables = []string{
	"cleargame",
	"h1", "h2", "h3", "h4", "h5", "h6", "h7", "h8", "h9", "h10",
	"h11", "h12", "h13", "h14", "h15", "h16", "h17", "h18", "h19", "h20", "h21",
	"clearinit", "systemmode", "smode",
}

var gothicKnownWords = map[string]struct{}{
	"if": {}, "elseif": {}, "else": {}, "endif": {}, "goto": {}, "gosub": {}, "return": {},
	"menu": {}, "endmenu": {}, "item": {}, "itemif": {}, "break": {}, "init": {}, "int": {}, "end": {},
	"restart": {}, "pause": {}, "g": {}, "graphic": {}, "t": {}, "text": {}, "music": {}, "game": {},
	"opening": {}, "logo": {}, "startmenu": {}, "staffroll": {}, "cls": {}, "mode": {}, "exec": {},
	"macro": {}, "endm": {}, "call": {}, "print": {}, "printcr": {}, "key": {}, "case": {}, "caseif": {},
	"default": {}, "resume": {}, "lwait": {}, "frame": {}, "memo": {}, "wait": {}, "load": {}, "on": {},
	"off": {}, "effect": {}, "draw": {}, "layer": {}, "scroll": {}, "copy": {}, "page": {}, "map": {},
	"clear": {}, "level": {}, "w": {}, "wave": {}, "tilt": {}, "ani": {}, "rmap": {}, "smap": {}, "dmap": {},
	"set": {}, "inputname": {}, "play": {}, "chain": {}, "open": {}, "map1": {}, "map2": {}, "map3": {},
	"frameu": {}, "framed": {}, "date1": {},
}

func (m *GothicMachine) execute(statement *GothicStatement) *GothicAction {
	word := commandWord(statement)
	if len(word) == 0 {
		core.Logf("gothic: unrecognized line at %d", statement.Index)
		m.pc++
		return nil
	}
	switch strings.ToLower(word) {
	case "end":
		m.pc++
		return m.finishNow()
	case "goto":
		m.gotoLabel(firstArg(statement), statement.Index)
		return nil
	case "gosub":
		m.gosubLabel(firstArg(statement), statement.Index)
		return nil
	case "return":
		m.returnStep()
		return nil
	case "if":
		return m.executeIf(statement)
	case "elseif":
		return m.executeElseIf(statement)
	case "else":
		return m.executeElse(statement)
	case "endif":
		return m.executeEndIf(statement)
	case "break":
		return m.executeBreak(statement)
	case "init":
		for _, name := range gothicSystemVariables {
			if !m.variables.IsDeclared(name) {
				m.variables.Declare(name, 0)
			}
		}
		m.pc++
		return nil
	case "int":
		name := unquoteGothic(firstArg(statement))
		if len(name) > 0 && !m.variables.IsDeclared(name) {
			m.variables.Declare(name, 0)
		}
		m.pc++
		return nil
	case "item", "itemif", "endmenu":
		m.pc++
		return nil
	case "menu":
		return m.executeMenu(statement)
	case "pause":
		m.pc++
		millis := int32(18) * GothicParseInt(firstArg(statement))
		if millis < 0 {
			millis = 0
		}
		return NewGothicAction(GothicActionPause, statement, millis, statement.Raw, nil)
	case "t", "text":
		return m.executeTextSub(statement)
	case "w":
		return m.executeWaveVoice(statement)
	case "wave":
		return m.executeWave(statement)
	case "tilt":
		m.pc++
		return NewGothicAction(GothicActionTilt, statement, 0, statement.Raw, nil)
	case "ani":
		return m.executeAni(statement)
	case "rmap":
		m.pc++
		return NewGothicAction(GothicActionRmap, statement, 0, statement.Raw, nil)
	case "smap":
		m.pc++
		return NewGothicAction(GothicActionSmap, statement, 0, statement.Raw, nil)
	case "dmap":
		return m.executeDmap(statement)
	case "set":
		m.pc++
		return NewGothicAction(GothicActionSetControl, statement, 0, statement.Raw, nil)
	case "inputname":
		m.pc++
		return NewGothicAction(GothicActionInputName, statement, 0, statement.Raw, nil)
	default:
		if m.tryRecoverAssignment(statement) {
			return nil
		}
		if _, ok := gothicKnownWords[strings.ToLower(word)]; !ok {
			core.Logf("gothic: unknown command "+word+" at %d", statement.Index)
		}
		m.pc++
		return NewGothicAction(GothicActionCommand, statement, 0, statement.Raw, nil)
	}
}

func (m *GothicMachine) executeIf(statement *GothicStatement) *GothicAction {
	tail := readInlineTail(joinArgs(statement.Args))
	if tail.isInline {
		if !GothicEvaluate(tail.condition, m.variables) {
			m.pc++
			return nil
		}
		return m.executeInline(tail, statement)
	}
	value := GothicEvaluate(joinArgs(statement.Args), m.variables)
	m.branches = append(m.branches, value)
	if value {
		m.pc++
		return nil
	}
	m.skipBranch()
	return nil
}

func (m *GothicMachine) executeElseIf(statement *GothicStatement) *GothicAction {
	if len(m.branches) == 0 {
		core.Logf("gothic: elseif without if at %d", statement.Index)
		m.pc++
		return nil
	}
	if m.branches[len(m.branches)-1] {
		m.skipToEndIf()
		return nil
	}
	value := GothicEvaluate(joinArgs(statement.Args), m.variables)
	if value {
		m.branches[len(m.branches)-1] = true
		m.pc++
		return nil
	}
	m.skipBranch()
	return nil
}

func (m *GothicMachine) executeElse(statement *GothicStatement) *GothicAction {
	if len(m.branches) == 0 {
		core.Logf("gothic: else without if at %d", statement.Index)
		m.pc++
		return nil
	}
	if m.branches[len(m.branches)-1] {
		m.skipToEndIf()
		return nil
	}
	m.branches[len(m.branches)-1] = true
	m.pc++
	return nil
}

func (m *GothicMachine) executeEndIf(statement *GothicStatement) *GothicAction {
	if len(m.branches) == 0 {
		core.Logf("gothic: endif without if at %d", statement.Index)
		m.pc++
		return nil
	}
	m.branches = m.branches[:len(m.branches)-1]
	m.pc++
	return nil
}

func (m *GothicMachine) executeBreak(statement *GothicStatement) *GothicAction {
	if !strings.EqualFold(firstArg(statement), "menu") {
		core.Logf("gothic: bad break at %d", statement.Index)
		m.pc++
		return nil
	}
	target := int32(-1)
	best := int32(-1)
	for _, menu := range m.program.Menus() {
		if menu != nil && menu.StatementIndex <= m.pc && m.pc <= menu.EndStatementIndex && menu.StatementIndex > best {
			best = menu.StatementIndex
			target = menu.EndStatementIndex + 1
		}
	}
	if target < 0 {
		core.Logf("gothic: break outside menu at %d", statement.Index)
		m.pc++
		return nil
	}
	if len(m.branches) > 0 {
		m.branches = m.branches[:len(m.branches)-1]
	}
	m.pc = target
	return nil
}

func (m *GothicMachine) executeMenu(statement *GothicStatement) *GothicAction {
	id := unquoteGothic(firstArg(statement))
	var found *GothicMenu
	if len(id) > 0 {
		found = m.program.FindMenu(id)
	}
	if found == nil {
		core.Logf("gothic: menu not found "+id+" at %d", statement.Index)
		m.pc++
		return nil
	}
	m.menu = found
	m.pc++
	return NewGothicAction(GothicActionMenu, statement, 0, statement.Raw, found)
}

func (m *GothicMachine) executeTextSub(statement *GothicStatement) *GothicAction {
	m.pc++
	args := statement.Args
	if args == nil {
		args = []string{}
	}
	if len(args) >= 2 && strings.EqualFold(args[0], "wait") {
		if count, ok := GothicTryParseStrict(args[1]); ok {
			return NewGothicAction(GothicActionWait, statement, int32(18)*count, statement.Raw, nil)
		}
	}
	return NewGothicAction(GothicActionCommand, statement, 0, statement.Raw, nil)
}

func (m *GothicMachine) executeWaveVoice(statement *GothicStatement) *GothicAction {
	m.pc++
	sub := strings.TrimSpace(statement.Sub)
	if len(sub) == 0 {
		sub = firstArg(statement)
	}
	args := statement.Args
	if args == nil {
		args = []string{}
	}
	id := int32(0)
	if len(args) > 1 {
		id = GothicParseInt(strings.TrimSpace(args[1]))
	}
	if strings.EqualFold(sub, "play") {
		return NewGothicAction(GothicActionWavePlay, statement, id, statement.Raw, nil)
	}
	if strings.EqualFold(sub, "chain") {
		return NewGothicAction(GothicActionWaveChain, statement, id, statement.Raw, nil)
	}
	core.Logf("gothic: bad w command at %d", statement.Index)
	return NewGothicAction(GothicActionCommand, statement, 0, statement.Raw, nil)
}

func (m *GothicMachine) executeWave(statement *GothicStatement) *GothicAction {
	m.pc++
	sub := strings.TrimSpace(statement.Sub)
	if len(sub) == 0 {
		sub = firstArg(statement)
	}
	if strings.EqualFold(sub, "open") {
		return NewGothicAction(GothicActionWaveOpen, statement, 0, statement.Raw, nil)
	}
	if strings.EqualFold(sub, "wait") {
		return NewGothicAction(GothicActionWaveWait, statement, 0, statement.Raw, nil)
	}
	core.Logf("gothic: bad wave command at %d", statement.Index)
	return NewGothicAction(GothicActionCommand, statement, 0, statement.Raw, nil)
}

func (m *GothicMachine) executeAni(statement *GothicStatement) *GothicAction {
	m.pc++
	mode := firstArg(statement)
	if strings.EqualFold(mode, "on") {
		return NewGothicAction(GothicActionAniToggle, statement, 1, statement.Raw, nil)
	}
	if strings.EqualFold(mode, "off") {
		return NewGothicAction(GothicActionAniToggle, statement, 0, statement.Raw, nil)
	}
	core.Logf("gothic: bad ani command at %d", statement.Index)
	return NewGothicAction(GothicActionCommand, statement, 0, statement.Raw, nil)
}

func (m *GothicMachine) executeDmap(statement *GothicStatement) *GothicAction {
	m.pc++
	args := statement.Args
	if args == nil {
		args = []string{}
	}
	slot := int32(0)
	if len(args) > 0 {
		slot = GothicParseInt(strings.TrimSpace(args[0]))
	}
	return NewGothicAction(GothicActionDmap, statement, slot, statement.Raw, nil)
}

func (m *GothicMachine) executeInline(tail *gothicInlineTail, statement *GothicStatement) *GothicAction {
	if strings.EqualFold(tail.kind, "goto") {
		m.gotoLabel(tail.label, statement.Index)
		return nil
	}
	if strings.EqualFold(tail.kind, "gosub") {
		m.gosubLabel(tail.label, statement.Index)
		return nil
	}
	m.returnStep()
	return nil
}

func (m *GothicMachine) gotoLabel(label string, index int32) {
	name := unquoteGothic(label)
	target := int32(-1)
	if len(name) > 0 {
		target = m.program.FindLabel(name)
	}
	if target < 0 || int(target) >= m.program.Count() {
		core.Logf("gothic: goto missing label "+name+" at %d", index)
		m.pc++
		return
	}
	m.branches = nil
	m.pc = target
}

func (m *GothicMachine) gosubLabel(label string, index int32) {
	name := unquoteGothic(label)
	target := int32(-1)
	if len(name) > 0 {
		target = m.program.FindLabel(name)
	}
	if target < 0 || int(target) >= m.program.Count() {
		core.Logf("gothic: gosub missing label "+name+" at %d", index)
		m.pc++
		return
	}
	m.calls = append(m.calls, m.pc+1)
	m.pc = target
}

func (m *GothicMachine) returnStep() {
	if len(m.calls) == 0 {
		m.pc++
		m.finishNow()
		return
	}
	m.pc = m.calls[len(m.calls)-1]
	m.calls = m.calls[:len(m.calls)-1]
}

func (m *GothicMachine) skipBranch() {
	depth := 0
	for i := int(m.pc) + 1; i < m.program.Count(); i++ {
		candidate := m.program.Statements()[i]
		if candidate.Kind != GothicStatementCommand && candidate.Kind != GothicStatementEmpty {
			continue
		}
		word := strings.ToLower(commandWord(candidate))
		if word == "if" {
			if isBlockIf(candidate) {
				depth++
			}
			continue
		}
		if word == "endif" {
			if depth == 0 {
				if len(m.branches) > 0 {
					m.branches = m.branches[:len(m.branches)-1]
				}
				m.pc = int32(i + 1)
				return
			}
			depth--
			continue
		}
		if depth == 0 && word == "elseif" {
			if GothicEvaluate(joinArgs(candidate.Args), m.variables) {
				if len(m.branches) > 0 {
					m.branches[len(m.branches)-1] = true
				}
				m.pc = int32(i + 1)
				return
			}
			continue
		}
		if depth == 0 && word == "else" {
			if len(m.branches) > 0 && m.branches[len(m.branches)-1] {
				m.skipToEndIfFrom(int32(i))
				return
			}
			if len(m.branches) > 0 {
				m.branches[len(m.branches)-1] = true
			}
			m.pc = int32(i + 1)
			return
		}
	}
	core.Logf("gothic: branch search fell off end at %d", m.pc)
	m.pc = int32(m.program.Count())
}

func (m *GothicMachine) skipToEndIf() {
	m.skipToEndIfFrom(m.pc)
}

func (m *GothicMachine) skipToEndIfFrom(from int32) {
	depth := 0
	for i := int(from) + 1; i < m.program.Count(); i++ {
		candidate := m.program.Statements()[i]
		if candidate.Kind != GothicStatementCommand && candidate.Kind != GothicStatementEmpty {
			continue
		}
		word := strings.ToLower(commandWord(candidate))
		if word == "if" && isBlockIf(candidate) {
			depth++
			continue
		}
		if word == "endif" {
			if depth == 0 {
				if len(m.branches) > 0 {
					m.branches = m.branches[:len(m.branches)-1]
				}
				m.pc = int32(i + 1)
				return
			}
			depth--
		}
	}
	core.Logf("gothic: endif search fell off end at %d", m.pc)
	m.pc = int32(m.program.Count())
}

func (m *GothicMachine) tryRecoverAssignment(statement *GothicStatement) bool {
	raw := strings.TrimLeft(statement.Raw, " \t")
	if strings.HasPrefix(raw, "#") {
		raw = strings.TrimLeft(raw[1:], " \t")
	}
	at := indexOfPlainEquals(raw)
	if at < 0 {
		return false
	}
	left := strings.TrimSpace(raw[:at])
	if len(left) == 0 || strings.Contains(left, " ") || strings.Contains(left, "\t") {
		return false
	}
	if _, ok := gothicKnownWords[strings.ToLower(left)]; ok {
		return false
	}
	if strings.EqualFold(left, "int") {
		return false
	}
	m.variables.Set(left, GothicEvaluateValue(strings.TrimSpace(raw[at+1:]), m.variables))
	m.pc++
	return true
}

func indexOfPlainEquals(text string) int {
	quoted := false
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == '"' {
			quoted = !quoted
			continue
		}
		if quoted || c != '=' {
			continue
		}
		prevOp := i > 0 && (text[i-1] == '<' || text[i-1] == '>' || text[i-1] == '!' || text[i-1] == '=')
		nextEq := i+1 < len(text) && text[i+1] == '='
		if prevOp || nextEq {
			continue
		}
		return i
	}
	return -1
}

func commandWord(statement *GothicStatement) string {
	name := strings.TrimSpace(statement.Name)
	if len(name) > 0 {
		return name
	}
	raw := strings.TrimLeft(statement.Raw, " \t")
	if strings.HasPrefix(raw, "#") {
		raw = strings.TrimLeft(raw[1:], " \t")
	}
	if strings.HasPrefix(raw, ":") {
		return ""
	}
	end := 0
	for end < len(raw) && raw[end] != ' ' && raw[end] != '\t' {
		end++
	}
	word := strings.TrimRight(raw[:end], ",")
	if eq := strings.Index(word, "="); eq >= 0 {
		word = word[:eq]
	}
	return word
}

func firstArg(statement *GothicStatement) string {
	args := statement.Args
	if args == nil {
		args = []string{}
	}
	if len(args) > 0 {
		return strings.TrimRight(strings.TrimSpace(args[0]), ",")
	}
	return strings.TrimSpace(statement.Sub)
}

func joinArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return strings.Join(args, " ")
}

func assignmentExpression(statement *GothicStatement) string {
	args := statement.Args
	if args == nil {
		args = []string{}
	}
	if len(args) >= 2 && strings.TrimSpace(args[0]) == "=" {
		return strings.Join(args[1:], " ")
	}
	sub := strings.TrimSpace(statement.Sub)
	if len(sub) > 0 {
		return sub
	}
	return joinArgs(args)
}

func unquoteGothic(text string) string {
	view := strings.TrimSpace(text)
	if len(view) >= 2 && view[0] == '"' && view[len(view)-1] == '"' {
		return view[1 : len(view)-1]
	}
	return view
}

func startsCommand(raw string) bool {
	return strings.HasPrefix(strings.TrimLeft(raw, " \t"), "#")
}

func pageBreakValue(statement *GothicStatement) int32 {
	if strings.TrimSpace(statement.Name) == "%%" {
		return 2
	}
	if strings.HasPrefix(strings.TrimLeft(statement.Raw, " \t"), "%%") {
		return 2
	}
	return 1
}

func isBlockIf(statement *GothicStatement) bool {
	if !strings.EqualFold(commandWord(statement), "if") {
		return false
	}
	return !readInlineTail(joinArgs(statement.Args)).isInline
}

type gothicInlineTail struct {
	isInline  bool
	kind      string
	label     string
	condition string
}

func readInlineTail(rest string) *gothicInlineTail {
	tail := &gothicInlineTail{condition: rest}
	var tokens []string
	start := -1
	flush := func(end int) {
		if start >= 0 {
			tokens = append(tokens, strings.TrimRight(rest[start:end], ","))
			start = -1
		}
	}
	for i := 0; i < len(rest); i++ {
		if rest[i] == ' ' || rest[i] == '\t' {
			flush(i)
		} else if start < 0 {
			start = i
		}
	}
	flush(len(rest))
	if len(tokens) >= 2 && strings.EqualFold(tokens[len(tokens)-2], "goto") {
		tail.isInline = true
		tail.kind = "goto"
		tail.label = unquoteGothic(tokens[len(tokens)-1])
		tail.condition = strings.Join(tokens[:len(tokens)-2], " ")
	} else if len(tokens) >= 2 && strings.EqualFold(tokens[len(tokens)-2], "gosub") {
		tail.isInline = true
		tail.kind = "gosub"
		tail.label = unquoteGothic(tokens[len(tokens)-1])
		tail.condition = strings.Join(tokens[:len(tokens)-2], " ")
	} else if len(tokens) >= 1 && strings.EqualFold(tokens[len(tokens)-1], "return") {
		tail.isInline = true
		tail.kind = "return"
		tail.condition = strings.Join(tokens[:len(tokens)-1], " ")
	}
	return tail
}
