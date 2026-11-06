package bdt

import (
	"strings"

	"kizuna/engine/core"
)

func (m *BdtMachine) executeWindow(text string) {
	if !strings.HasPrefix(text, "BW") {
		m.unknown(text)
		return
	}
	parts := strings.Split(text[2:], ",")
	if len(parts) < 2 {
		core.Logf("bdt: bad window: %s", text)
		m.pc++
		return
	}
	part := int(bdtAtoi(parts[0]))
	if part < 0 || part > 2 {
		core.Logf("bdt: bad window: %s", text)
		m.pc++
		return
	}
	visible := bdtAtoi(parts[1]) != 0
	m.pc++
	m.callHost(func() { m.host.SetWindow(part, visible) }, "window")
	if len(parts) < 3 || len(parts[2]) == 0 || (parts[2][0] != 'f' && parts[2][0] != 'F') {
		m.callHost(func() { m.host.Redraw() }, "redraw")
	}
}

func (m *BdtMachine) executeFigure(text string) {
	if len(text) < 2 || text[0] != 'C' {
		m.unknown(text)
		return
	}
	body := ""
	if len(text) > 2 {
		body = text[2:]
	}
	switch text[1] {
	case 'A':
		if m.dialect == BdtDialectManin {
			m.executeManinSpriteAdd(body)
			return
		}
		m.executeSpriteAdd(body)
		return
	case 'B':
		m.executeClearBlack()
		return
	case 'W':
		m.executeClearWhite()
		return
	case 'F':
		m.executeSetBackground(body)
		return
	case 'S':
		m.executeSetBackgroundNow(body)
		return
	case 'D':
		m.executeEffect(body)
		return
	case 'K':
		if m.dialect == BdtDialectManin {
			m.pc++
			m.callHost(func() { m.host.ClearSprite(0) }, "sprites")
			m.clearSpriteTracking()
			return
		}
		m.executeSpriteRemove(body)
		return
	case 'M':
		m.executeSpritePlaced(body, 1)
		return
	case 'N':
		m.executeSpritePlaced(body, 0)
		return
	case 'O', 'P', 'Q':
		m.executeSimpleEffect(body)
		return
	case 'R':
		m.pc++
		m.callHost(func() { m.host.SetEffect(32) }, "effect")
		return
	case 'Z':
		m.executeStall(body)
		return
	case 'U':
		m.executeBlend(body)
		return
	case 'T':
		m.pc++
		m.callHost(func() { m.host.Redraw() }, "redraw")
		return
	default:
		m.unknown(text)
		return
	}
}

func (m *BdtMachine) executeClearBlack() {
	m.clearSpriteTracking()
	m.pc++
	m.callHost(func() { m.host.ClearSprites() }, "sprites")
	m.callHost(func() { m.host.SetBackground("", 0) }, "background")
}

func (m *BdtMachine) executeClearWhite() {
	m.clearSpriteTracking()
	m.pc++
	m.callHost(func() { m.host.ClearSprites() }, "sprites")
}

func (m *BdtMachine) executeSetBackground(body string) {
	name := strings.TrimSpace(body)
	m.clearSpriteTracking()
	m.pc++
	m.callHost(func() { m.host.ClearSprites() }, "sprites")
	m.callHost(func() { m.host.SetBackground(name, 0) }, "background")
}

func (m *BdtMachine) executeSetBackgroundNow(body string) {
	name := strings.TrimSpace(body)
	m.pc++
	m.callHost(func() { m.host.SetBackground(name, 0) }, "background")
	m.callHost(func() { m.host.Redraw() }, "redraw")
}

func (m *BdtMachine) executeSimpleEffect(body string) {
	effect := int(ParseBdtCommandArgs(body).NextInt())
	m.pc++
	m.callHost(func() { m.host.SetEffect(effect) }, "effect")
}

func (m *BdtMachine) executeStall(body string) {
	args := ParseBdtCommandArgs(body)
	m.czFirst = args.NextInt()
	if args.AtEnd() {
		m.czSecond = 0
	} else {
		m.czSecond = args.NextInt()
	}
	m.pc++
}

func (m *BdtMachine) executeBlend(body string) {
	effect := 101
	if ParseBdtCommandArgs(body).NextInt() == 31 {
		effect = 131
	}
	m.pc++
	m.callHost(func() { m.host.SetEffect(effect) }, "effect")
}

func (m *BdtMachine) executeManinSpriteAdd(body string) {
	args := ParseBdtCommandArgs(body)
	_ = args.NextInt()
	name := strings.TrimSpace(args.Remainder())
	m.spriteUsed[0] = true
	m.spriteSlots[0] = bdtSpritePlacement{name: name}
	m.pc++
	m.callHost(func() { m.host.SetSprite(0, name, 160, 0, 0) }, "sprite")
}

func (m *BdtMachine) executeSpriteAdd(body string) {
	var x int32
	var name string
	if strings.HasPrefix(body, ",") {
		name = strings.TrimSpace(ParseBdtCommandArgs(body).Remainder())
	} else {
		comma := strings.IndexByte(body, ',')
		if comma < 0 {
			name = strings.TrimSpace(body)
		} else {
			args := ParseBdtCommandArgs(body)
			x = args.NextInt()
			name = strings.TrimSpace(args.Remainder())
		}
	}
	slot := m.firstFreeSprite()
	if slot >= 0 && slot < BdtSpriteSlotSize {
		m.spriteUsed[slot] = true
		m.spriteSlots[slot] = bdtSpritePlacement{name: name, x: x}
	}
	m.pc++
	m.callHost(func() { m.host.SetSprite(slot, name, int(x), 0, 0) }, "sprite")
}

func (m *BdtMachine) executeSpriteRemove(body string) {
	slot := int(ParseBdtCommandArgs(body).NextInt())
	if slot < 1 || slot > BdtSpriteSlotSize {
		m.clearSpriteTracking()
		m.pc++
		m.callHost(func() { m.host.ClearSprites() }, "sprites")
		return
	}
	index := slot - 1
	if !m.spriteUsed[index] {
		m.pc++
		return
	}
	m.spriteUsed[index] = false
	m.spriteSlots[index] = bdtSpritePlacement{}
	m.pc++
	target := index
	for source := index + 1; source < BdtSpriteSlotSize; source++ {
		if !m.spriteUsed[source] {
			continue
		}
		placement := m.spriteSlots[source]
		m.spriteSlots[target] = placement
		m.spriteUsed[target] = true
		m.spriteUsed[source] = false
		m.spriteSlots[source] = bdtSpritePlacement{}
		m.callHost(func() {
			m.host.SetSprite(target, placement.name, int(placement.x), int(placement.y), int(placement.mode))
		}, "sprite")
		m.callHost(func() { m.host.ClearSprite(source) }, "sprites")
		target++
	}
}

func (m *BdtMachine) executeSpritePlaced(body string, mode int32) {
	first := strings.IndexByte(body, ',')
	second := -1
	if first >= 0 {
		if next := strings.IndexByte(body[first+1:], ','); next >= 0 {
			second = first + 1 + next
		}
	}
	if first < 0 || second < 0 {
		core.Logf("bdt: bad sprite: %s", body)
		m.pc++
		return
	}
	args := ParseBdtCommandArgs(body)
	x := args.NextInt()
	y := args.NextInt()
	name := strings.TrimSpace(args.Remainder())
	slot := m.firstFreeSprite()
	if slot >= 0 && slot < BdtSpriteSlotSize {
		m.spriteUsed[slot] = true
		m.spriteSlots[slot] = bdtSpritePlacement{name: name, x: x, y: y, mode: mode}
	}
	m.pc++
	m.callHost(func() {
		m.host.SetSprite(slot, name, int(x), int(y), int(mode))
	}, "sprite")
}

func (m *BdtMachine) clearSpriteTracking() {
	for i := range m.spriteUsed {
		m.spriteUsed[i] = false
	}
	for i := range m.spriteSlots {
		m.spriteSlots[i] = bdtSpritePlacement{}
	}
}

func (m *BdtMachine) firstFreeSprite() int {
	for i := 0; i < BdtSpriteSlotSize; i++ {
		if !m.spriteUsed[i] {
			return i
		}
	}
	return BdtSpriteSlotSize - 1
}

func (m *BdtMachine) executeEffect(body string) {
	args := ParseBdtCommandArgs(body)
	effect := int(args.NextInt())
	name := strings.TrimSpace(args.Remainder())
	m.pc++
	if len(name) > 0 {
		m.callHost(func() { m.host.SetBackground(name, effect) }, "background")
	}
	m.callHost(func() { m.host.SetEffect(effect) }, "effect")
}
