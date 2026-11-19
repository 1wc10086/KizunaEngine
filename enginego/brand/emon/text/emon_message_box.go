package text

import (
	"strings"

	"kizuna/enginego/core"
)

type EmonMessageBox struct {
	speaker       string
	body          string
	visible       bool
	revealCount   int
	revealedBody  string
	revealedShown int
	revealedText  string
	revision      int
	// revealCarry 保存逐字显示不足一个字的余量。每帧 delta*cps 常常小于 1
	// （60fps + 15cps 时只有 0.25），直接取整再加会把“慢速”永久卡在 0 字。
	revealCarry float64
}

func NewEmonMessageBox() *EmonMessageBox {
	return &EmonMessageBox{revealedShown: -1}
}

func (b *EmonMessageBox) Speaker() string {
	return b.speaker
}

func (b *EmonMessageBox) SetSpeaker(value string) {
	if b.speaker == value {
		return
	}

	b.speaker = value
	b.revision++
}

func (b *EmonMessageBox) Body() string {
	return b.body
}

func (b *EmonMessageBox) SetBody(value string) {
	if b.body == value {
		return
	}

	b.body = value
	b.revealCarry = 0
	b.revision++
}

func (b *EmonMessageBox) Visible() bool {
	return b.visible
}

func (b *EmonMessageBox) SetVisible(value bool) {
	if b.visible == value {
		return
	}

	b.visible = value
	b.revision++
}

func (b *EmonMessageBox) RevealCount() int {
	return b.revealCount
}

func (b *EmonMessageBox) setRevealCount(value int) {
	if b.revealCount == value {
		return
	}

	b.revealCount = value
	b.revision++
}

func (b *EmonMessageBox) Revision() int {
	return b.revision
}

func (b *EmonMessageBox) FullyRevealed() bool {
	return b.revealCount >= runeLength(b.body)
}

func (b *EmonMessageBox) LineCount() int {
	if len(b.body) == 0 {
		return 0
	}

	return 1 + strings.Count(b.body, "\n")
}

func (b *EmonMessageBox) RevealedText() string {
	length := runeLength(b.body)
	shown := core.Clamp(b.revealCount, 0, length)
	if b.revealedShown != shown || b.revealedBody != b.body {
		b.revealedBody = b.body
		b.revealedShown = shown
		b.revealedText = strings.TrimRight(prefixRunes(b.body, shown), " \u3000\t")
	}

	return b.revealedText
}

func (b *EmonMessageBox) Append(text string) {
	if len(text) == 0 {
		return
	}

	if len(b.body) == 0 {
		b.revealCarry = 0
		b.SetBody(text)
		return
	}

	// body 已经以换行结尾（#Return 刚做过软换行）时直接续写，
	// 否则每句正文都会多出一个空行。
	if strings.HasSuffix(b.body, "\n") {
		b.SetBody(b.body + text)
		return
	}

	b.SetBody(b.body + "\n" + text)
}

// BreakLine 对应原引擎 #Return 的“推进到下一行、不清空”语义：
// 后续 #Hit 前的下一句正文要另起一行显示在同一个对话框里。
// 已经以换行结尾（或文本框为空）时不再补，避免出现空行。
func (b *EmonMessageBox) BreakLine() {
	if len(b.body) == 0 || strings.HasSuffix(b.body, "\n") {
		return
	}
	b.SetBody(b.body + "\n")
}

// AppendInline 追加"同一脚本行里命令之后的后半句"：直接接在正文末尾，
// 不插换行。原引擎一行脚本只对应一段正文，命令只是段内的副作用。
func (b *EmonMessageBox) AppendInline(text string) {
	if len(text) == 0 {
		return
	}
	if len(b.body) == 0 {
		b.SetBody(text)
		return
	}
	b.SetBody(b.body + text)
}

func (b *EmonMessageBox) Clear() {
	b.SetSpeaker("")
	b.SetBody("")
	b.revealCarry = 0
	b.setRevealCount(0)
}

func (b *EmonMessageBox) Advance(deltaSeconds float64, charactersPerSecond int) {
	if deltaSeconds <= 0 || charactersPerSecond <= 0 {
		return
	}

	b.revealCarry += deltaSeconds * float64(charactersPerSecond)
	whole := int(b.revealCarry)
	if whole <= 0 {
		return
	}
	b.revealCarry -= float64(whole)
	next := b.revealCount + whole
	length := runeLength(b.body)
	if next > length {
		next = length
		b.revealCarry = 0
	}

	b.setRevealCount(next)
}

func (b *EmonMessageBox) RevealAll() {
	b.setRevealCount(runeLength(b.body))
}
