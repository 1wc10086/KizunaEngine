package bdt

import (
	"strings"
	"unicode"

	"kizuna/engine/core"
)

func (m *BdtMachine) executeCaption(text string) {
	if strings.HasPrefix(text, "LX") {
		if m.dialect == BdtDialectManin {
			caption := ""
			if len(text) > 2 {
				caption = text[2:]
			}
			m.pc++
			if h, ok := m.host.(ManinHost); ok {
				m.callHost(func() { h.SetCaption(caption) }, "caption")
			}
			return
		}
		m.pc++
		return
	}
	if strings.HasPrefix(text, "LH") || strings.HasPrefix(text, "LK") {
		m.pc++
		return
	}
	m.unknown(text)
}

func (m *BdtMachine) executeGoto(body string) {
	if !m.jumpTo(body) {
		m.pc++
	}
}

func (m *BdtMachine) executeGoSub(body string) {
	resume := m.pc + 1
	if !m.jumpTo(body) {
		m.pc++
		return
	}
	if len(m.calls) >= BdtMaxCallDepth {
		m.calls = append(m.calls[1:], resume)
	} else {
		m.calls = append(m.calls, resume)
	}
}

func (m *BdtMachine) executeReturn() {
	if len(m.calls) == 0 {
		m.halt()
		return
	}
	m.pc = m.calls[len(m.calls)-1]
	m.calls = m.calls[:len(m.calls)-1]
}

func (m *BdtMachine) executeReturnWait() {
	m.pc++
	m.stop = BdtStopInput
}

func (m *BdtMachine) executeChapterEnd() {
	m.calls = m.calls[:0]
	m.pending = m.pending[:0]
	m.choices = m.choices[:0]
	m.waitMilliseconds = 0
	m.callHost(func() { m.host.StopBgm() }, "bgm")
	if !m.jumpTo("999-99") {
		m.halt()
	}
}

func (m *BdtMachine) executeJump(body string) {
	parts := strings.Split(body, ",")
	if len(parts) < 5 {
		core.Logf("bdt: bad jump: %s", body)
		m.pc++
		return
	}
	number := bdtAtoi(parts[0])
	basis := 0
	if len(parts[1]) > 0 {
		basis = bdtBankBasis(parts[1][0])
	}
	limit := bdtAtoi(parts[2])
	op := byte('=')
	if len(parts[3]) > 0 {
		op = parts[3][0]
	}
	value := m.getVariable(basis, number)
	var taken bool
	switch op {
	case '=':
		taken = value == limit
	case '<':
		taken = value < limit
	case '>':
		taken = value > limit
	case 'N', '!':
		taken = value != limit
	default:
		core.Logf("bdt: bad jump operator: %s", body)
		m.pc++
		return
	}
	if taken {
		if !m.jumpTo(parts[4]) {
			m.pc++
		}
	} else {
		m.pc++
	}
}

func (m *BdtMachine) executeQuestion(text string) {
	rest := ""
	if len(text) > 1 {
		rest = text[1:]
	}
	if strings.HasPrefix(rest, "YN") {
		m.executeYesNo(rest[2:])
		return
	}
	if len(rest) == 0 {
		m.unknown(text)
		return
	}
	switch rest[0] {
	case 'A':
		m.executeFlagJump(rest[1:], m.sceneFlags[:], false)
		return
	case 'C':
		m.executeFlagJump(rest[1:], m.systemFlags[:], false)
		return
	case 'O':
		m.executeFlagJump(rest[1:], m.sceneFlags[:], true)
		return
	case 'Y':
		m.executeSingleFlagJump(rest[1:])
		return
	default:
		m.unknown(text)
		return
	}
}

func (m *BdtMachine) executeFlagJump(body string, table []int32, any bool) {
	parts := strings.Split(body, ",")
	if len(parts) < 3 {
		core.Logf("bdt: bad condition: %s", body)
		m.pc++
		return
	}
	count := int(bdtAtoi(parts[0]))
	if count < 0 {
		count = 0
	}
	if len(parts) < count+2 {
		core.Logf("bdt: bad condition: %s", body)
		m.pc++
		return
	}
	taken := !any
	for i := 0; i < count; i++ {
		flag := bdtAtoi(parts[1+i])
		set := flag >= 0 && flag < BdtFlagCap && table[flag] != 0
		if any {
			if set {
				taken = true
				break
			}
		} else if !set {
			taken = false
			break
		}
	}
	if taken {
		if !m.jumpTo(parts[1+count]) {
			m.pc++
		}
	} else {
		m.pc++
	}
}

func (m *BdtMachine) executeSingleFlagJump(body string) {
	parts := strings.Split(body, ",")
	if len(parts) < 2 {
		core.Logf("bdt: bad condition: %s", body)
		m.pc++
		return
	}
	flag := bdtAtoi(parts[0])
	set := flag >= 0 && flag < BdtFlagCap && m.sceneFlags[flag] != 0
	if set {
		if !m.jumpTo(parts[1]) {
			m.pc++
		}
	} else {
		m.pc++
	}
}

func (m *BdtMachine) executeYesNo(body string) {
	first := strings.IndexByte(body, ',')
	second := -1
	if first >= 0 {
		if next := strings.IndexByte(body[first+1:], ','); next >= 0 {
			second = first + 1 + next
		}
	}
	if first < 0 || second < 0 {
		core.Logf("bdt: bad yesno: %s", body)
		m.pc++
		return
	}
	yes := strings.TrimSpace(body[:first])
	no := strings.TrimSpace(body[first+1 : second])
	prompt := body[second+1:]
	m.callHost(func() { m.host.ShowMessage("", prompt, "") }, "message")
	m.choices = m.choices[:0]
	m.pending = m.pending[:0]
	m.choices = append(m.choices, BdtChoiceOption{Index: 0, Label: yes, Text: "はい"})
	m.choices = append(m.choices, BdtChoiceOption{Index: 1, Label: no, Text: "いいえ"})
	m.pc++
	m.resumePc = m.pc
	m.stop = BdtStopChoice
}

func (m *BdtMachine) executeConditional(text string) {
	extended := strings.HasPrefix(text, "IFE")
	body := text[2:]
	if extended {
		body = text[3:]
	}
	comma := strings.IndexByte(body, ',')
	if comma < 0 {
		core.Logf("bdt: bad conditional: %s", body)
		m.pc++
		return
	}
	if m.consequentDepth >= bdtConsequentDepthLimit {
		core.Logf("bdt: conditional depth exceeded: %s", body)
		m.pc++
		return
	}
	expression := body[:comma]
	consequent := strings.TrimLeftFunc(body[comma+1:], unicode.IsSpace)
	if len(consequent) == 0 {
		core.Logf("bdt: empty consequent: %s", body)
		m.pc++
		return
	}
	var result int32
	if infix, ok := TryParseBdtInfixExpression(expression); !extended && ok {
		result = infix.Evaluate(m.resolveExpressionAtom)
	} else {
		result = EvaluateBdtExpression(expression, m.resolveExpressionAtom)
	}
	if result > 0 {
		m.consequentDepth++
		m.executeCommand(consequent)
		m.consequentDepth--
	} else {
		m.pc++
	}
}

func (m *BdtMachine) executeAssign(body string) {
	parts := strings.Split(body, ",")
	if len(parts) < 3 || len(parts[2]) == 0 {
		core.Logf("bdt: bad assign: %s", body)
		m.pc++
		return
	}
	number := bdtAtoi(parts[0])
	basis := 0
	if len(parts[1]) > 0 {
		basis = bdtBankBasis(parts[1][0])
	}
	mode := parts[2][0]
	delta := bdtAtoi(parts[2])
	value := delta
	if mode == '+' || mode == '-' {
		value = m.getVariable(basis, number) + delta
	}
	m.setVariable(basis, number, value)
	m.pc++
}

func (m *BdtMachine) executeSystemFlag(body string) {
	flag := bdtAtoi(body)
	if flag >= 0 && flag < BdtFlagCap {
		m.systemFlags[flag] = 1
	}
	m.pc++
}

func (m *BdtMachine) executeSceneFlag(body string, value int32) {
	flag := bdtAtoi(body)
	if flag >= 0 && flag < BdtFlagCap {
		m.sceneFlags[flag] = value
	}
	m.pc++
}

func (m *BdtMachine) executeExtended(text string) {
	if strings.HasPrefix(text, "XF") {
		sceneId := ""
		if len(text) > 2 {
			sceneId = strings.TrimSpace(text[2:])
		}
		m.pc++
		m.callHost(func() { m.host.SetSceneId(sceneId) }, "scene")
		return
	}
	if strings.HasPrefix(text, "XI") {
		mode := 0
		if len(text) > 2 {
			mode = int(bdtAtoi(text[2:]))
		}
		m.pc++
		m.callHost(func() { m.host.SetScreenMode(mode) }, "screen")
		return
	}
	if strings.HasPrefix(text, "XX") {
		body := ""
		if len(text) > 2 {
			body = strings.TrimSpace(text[2:])
		}
		if len(body) >= 2 {
			number := bdtAtoi(body[1:])
			switch body[0] {
			case '=':
				m.textSpeed = int(number)
			case '+':
				m.textSpeed += int(number)
			case '-':
				m.textSpeed -= int(number)
			default:
				core.Logf("bdt: bad setting: %s", text)
				m.pc++
				return
			}
			if m.textSpeed < 0 {
				m.textSpeed = 0
			}
			if m.textSpeed > BdtTextSpeedMax {
				m.textSpeed = BdtTextSpeedMax
			}
			m.pc++
			m.callHost(func() { m.host.Redraw() }, "redraw")
			return
		}
		core.Logf("bdt: bad setting: %s", text)
		m.pc++
		return
	}
	if m.dialect == BdtDialectManin {
		if strings.HasPrefix(text, "XT") {
			m.maninTimeLimit = int(bdtAtoi(text[2:]))
			m.pc++
			return
		}
		if strings.HasPrefix(text, "XB") {
			args := ParseBdtCommandArgs(text[2:])
			n := int(args.NextInt())
			label := strings.TrimSpace(args.Remainder())
			if n >= 1 && n <= 6 {
				m.maninTargets[n-1] = label
			}
			m.pc++
			return
		}
		if strings.HasPrefix(text, "XA") {
			m.pc++
			target := int(m.x[29])
			if target < 1 {
				target = 1
			}
			if target > 6 {
				target = 6
			}
			if h, ok := m.host.(ManinHost); ok {
				limit := m.maninTimeLimit
				m.callHost(func() { h.StartManinAttack(target, limit) }, "manin-attack")
			}
			m.stop = BdtStopAttack
			return
		}
		if strings.HasPrefix(text, "XR") {
			m.pc++
			if h, ok := m.host.(ManinHost); ok {
				m.callHost(func() { h.StartManinOrganizer() }, "manin-organizer")
			}
			m.stop = BdtStopOrganize
			return
		}
		if strings.HasPrefix(text, "XM") || strings.HasPrefix(text, "XE") {
			m.pc++
			return
		}
	}
	if strings.HasPrefix(text, "XA") ||
		strings.HasPrefix(text, "XB") ||
		strings.HasPrefix(text, "XE") ||
		strings.HasPrefix(text, "XM") ||
		strings.HasPrefix(text, "XT") ||
		strings.HasPrefix(text, "XR") {
		m.pc++
		return
	}
	m.unknown(text)
}

func (m *BdtMachine) executeWait(text string) {
	if strings.HasPrefix(text, "WJ") {
		body := ""
		if len(text) > 2 {
			body = text[2:]
		}
		parts := strings.Split(body, ",")
		if len(parts) < 2 {
			core.Logf("bdt: bad wait jump: %s", body)
			m.pc++
			return
		}
		m.wjFile = strings.TrimSpace(parts[0])
		m.wjLabel = strings.TrimSpace(parts[1])
		m.pc++
		return
	}
	if strings.HasPrefix(text, "WT") {
		wait := int(bdtAtoi(text[2:]))
		if wait < 0 {
			wait = 0
		}
		m.waitMilliseconds = wait
		m.pc++
		m.stop = BdtStopWait
		return
	}
	if strings.HasPrefix(text, "WK") {
		wait := bdtAtoi(text[2:])
		if wait < 0 {
			wait = 0
		}
		m.waitMilliseconds = int(wait * 1000)
		m.pc++
		m.stop = BdtStopWait
		return
	}
	if strings.HasPrefix(text, "WS") || strings.HasPrefix(text, "WM") {
		m.pc++
		return
	}
	m.unknown(text)
}

func (m *BdtMachine) executeMusic(text string) {
	if strings.HasPrefix(text, "MAP") {
		m.pc++
		m.stop = BdtStopMap
		return
	}
	if strings.HasPrefix(text, "MS") {
		m.pc++
		m.callHost(func() { m.host.StopBgm() }, "bgm")
		return
	}
	if strings.HasPrefix(text, "MP") {
		track := int(bdtAtoi(text[2:]))
		m.pc++
		m.callHost(func() { m.host.SetBgm(track, 3) }, "bgm")
		return
	}
	if strings.HasPrefix(text, "MN") {
		track := int(bdtAtoi(text[2:]))
		m.pc++
		m.callHost(func() { m.host.SetBgm(track, 1) }, "bgm")
		return
	}
	if strings.HasPrefix(text, "MV") {
		m.pc++
		return
	}
	m.unknown(text)
}
