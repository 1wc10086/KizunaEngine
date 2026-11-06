package bdt

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"kizuna/engine/core"
)

func (m *BdtMachine) executeQuote(text string) {
	body := ""
	if len(text) > 1 {
		body = text[1:]
	}
	body = stripBdtEdgeQuotes(body)
	m.pc++
	m.callHost(func() { m.host.ShowMessage("", body, "") }, "message")
	m.stop = BdtStopText
}

func (m *BdtMachine) executeBang(text string) {
	body := ""
	if len(text) > 1 {
		body = text[1:]
	}
	speaker := ""
	saying := body
	if tab := strings.IndexByte(body, '\t'); tab >= 0 {
		speaker = body[:tab]
		saying = body[tab+1:]
	}
	saying = stripBdtEdgeQuotes(saying)
	m.pc++
	m.callHost(func() { m.host.ShowMessage(speaker, saying, "") }, "message")
	m.stop = BdtStopText
}

func (m *BdtMachine) executeDialogue(text string) {
	dialogue, ok := TryParseBdtDialogue(text)
	if !ok {
		var faceOk bool
		dialogue, faceOk = TryParseBdtFaceDialogue(text)
		if !faceOk {
			m.unknown(text)
			return
		}
	}
	m.pc++
	face := dialogue.Face
	speaker := dialogue.Speaker
	message := dialogue.Text
	voiceCode := dialogue.VoiceCode
	if len(voiceCode) > 0 {
		voiceCode = "v" + voiceCode
	}
	if len(face) > 0 {
		m.callHost(func() { m.host.ShowFace(face) }, "face")
	}
	if len(voiceCode) > 0 {
		m.callHost(func() { m.host.PlayVoice(voiceCode) }, "voice")
	}
	m.callHost(func() { m.host.ShowMessage(speaker, message, voiceCode) }, "message")
	m.stop = BdtStopText
}

func (m *BdtMachine) executeVoice(text string) {
	if strings.HasPrefix(text, "!") {
		m.executeBang(text)
		return
	}
	tab := strings.IndexByte(text, '\t')
	code := text
	rest := ""
	if tab >= 0 {
		code = text[:tab]
		rest = text[tab+1:]
	}
	if utf8.RuneCountInString(code) >= 2 && !bdtSecondRuneIsDigit(code) {
		m.pc++
		m.callHost(func() { m.host.PlaySe(code) }, "se")
		return
	}
	digits := bdtDropFirstRune(code)
	voiceCode := "v" + digits
	m.pc++
	m.callHost(func() { m.host.PlayVoice(voiceCode) }, "voice")
	quoted, ok := extractBdtQuoted(rest)
	if !ok {
		return
	}
	speaker := ""
	if bang := strings.IndexByte(rest, '!'); bang >= 0 {
		after := rest[bang+1:]
		speakerTab := strings.IndexByte(after, '\t')
		quote := strings.IndexByte(after, '"')
		end := len(after)
		if speakerTab >= 0 && (quote < 0 || speakerTab < quote) {
			end = speakerTab
		} else if quote >= 0 {
			end = quote
		}
		speaker = strings.TrimSpace(after[:end])
	}
	m.callHost(func() { m.host.ShowMessage(speaker, quoted, voiceCode) }, "message")
	m.stop = BdtStopText
}

// T 是 BDT 引擎内置的标题画面调用，不是普通等待：停在 BdtStopTitle，由宿主/Session
// 铺标题并等待玩家选择。选中 START 后 pc 已指向下一条（$100-01），直接继续执行即可。
// TM<n> 是标题音乐（第 n 轨，循环），同样不是等待。其他 T 开头的命令保留原先的等待兜底。
func (m *BdtMachine) executeType(text string) {
	if text == "T" {
		m.pc++
		m.stop = BdtStopTitle
		return
	}
	if strings.HasPrefix(text, "TM") {
		track := int(bdtAtoi(text[2:]))
		m.pc++
		m.callHost(func() { m.host.SetBgm(track, 3) }, "bgm")
		return
	}
	m.waitMilliseconds = 0
	m.pc++
	m.stop = BdtStopWait
}

func (m *BdtMachine) executeSelect(text string) {
	if strings.HasPrefix(text, "SROL") && m.dialect == BdtDialectManin {
		m.pc++
		if h, ok := m.host.(ManinHost); ok {
			m.callHost(func() { h.StartManinStaffRoll() }, "manin-staffroll")
		}
		m.stop = BdtStopStaffRoll
		return
	}
	if strings.HasPrefix(text, "SA") {
		body := ""
		if len(text) > 2 {
			body = text[2:]
		}
		comma := strings.IndexByte(body, ',')
		if comma < 0 {
			core.Logf("bdt: bad choice: %s", body)
			m.pc++
			return
		}
		label := strings.TrimSpace(body[:comma])
		choiceText := body[comma+1:]
		if len(m.pending) >= BdtMaxChoices {
			core.Logf("bdt: choice overflow: %s", label)
		} else {
			m.pending = append(m.pending, bdtPendingChoice{label: label, text: choiceText})
		}
		m.pc++
		return
	}
	if strings.HasPrefix(text, "SU") {
		if len(m.pending) == 0 {
			core.Logf("bdt: empty choice prompt")
			m.pc++
			return
		}
		m.choices = m.choices[:0]
		for i := range m.pending {
			m.choices = append(m.choices, BdtChoiceOption{Index: i, Label: m.pending[i].label, Text: m.pending[i].text})
		}
		m.pending = m.pending[:0]
		m.pc++
		m.resumePc = m.pc
		m.stop = BdtStopChoice
		return
	}
	if strings.HasPrefix(text, "ST") {
		m.pc++
		m.callHost(func() { m.host.Redraw() }, "redraw")
		return
	}
	if strings.HasPrefix(text, "SR") || strings.HasPrefix(text, "SEV") {
		m.pc++
		return
	}
	m.unknown(text)
}

func (m *BdtMachine) executeSeCommand(text string) {
	runes := []rune(text)
	if len(runes) >= 3 && strings.EqualFold(string(runes[1:3]), "se") {
		m.pc++
		m.callHost(func() { m.host.PlaySe(text[1:]) }, "se")
		return
	}
	if len(runes) >= 2 && unicode.IsDigit(runes[1]) {
		end := 1
		for end < len(text) && text[end] != '\t' && text[end] != ' ' {
			end++
		}
		voiceCode := "v" + text[1:end]
		m.pc++
		m.callHost(func() { m.host.PlayVoice(voiceCode) }, "voice")
		return
	}
	m.unknown(text)
}

func bdtSecondRuneIsDigit(s string) bool {
	_, width := utf8.DecodeRuneInString(s)
	if width >= len(s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[width:])
	return unicode.IsDigit(r)
}

func bdtDropFirstRune(s string) string {
	_, width := utf8.DecodeRuneInString(s)
	if width >= len(s) {
		return ""
	}
	return s[width:]
}
