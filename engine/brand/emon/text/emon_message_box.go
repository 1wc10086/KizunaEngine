package text

import (
	"strings"

	"kizuna/engine/core"
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
		b.SetBody(text)
		return
	}

	b.SetBody(b.body + "\n" + text)
}

func (b *EmonMessageBox) Clear() {
	b.SetSpeaker("")
	b.SetBody("")
	b.setRevealCount(0)
}

func (b *EmonMessageBox) Advance(deltaSeconds float64, charactersPerSecond int) {
	if deltaSeconds <= 0 || charactersPerSecond <= 0 {
		return
	}

	next := b.revealCount + int(deltaSeconds*float64(charactersPerSecond))
	length := runeLength(b.body)
	if next > length {
		next = length
	}

	b.setRevealCount(next)
}

func (b *EmonMessageBox) RevealAll() {
	b.setRevealCount(runeLength(b.body))
}
