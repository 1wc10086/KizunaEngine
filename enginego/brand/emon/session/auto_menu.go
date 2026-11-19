package session

// auto_menu.go：ApearMenu(AUTOSETTING)（オートモード設定）的引擎自绘面板。
//
// 原版依据：asenure.exe 的菜单分发 sub_427AB0 把 AUTOSETTING 视为引擎菜单
// （返回 2 = 内置对话框），对话框里只有三项：オートＷＡＩＴ / オートＭＩＮ /
// オートＭＡＸ，分别写系统变量 AUTOWAIT / AUTOMIN / AUTOMAX（sub_410800/
// sub_410840/sub_410880）。原版用三个滑杆；这里用 ◀/▶ 步进，操作结果相同，
// 且不依赖宿主 UI，触屏可直接点。数值随 emesys.dat 持久化。

import (
	"strconv"
	"strings"

	emontext "kizuna/enginego/brand/emon/text"
	"kizuna/enginego/brand/emon/script"
	"kizuna/enginego/core"
)

const (
	autoMenuWidth  = 620
	autoMenuHeight = 340
	autoMenuRowH   = 64
)

type autoMenuButton struct {
	id    string // wait-, wait+, min-, min+, max-, max+, close
	x, y  int
	w, h  int
	label string
}

func (s *Session) pollAutoMenu() {
	if s.machine == nil || s.autoMenu {
		return
	}
	if !s.machine.AutoMenuRequested() {
		return
	}
	s.machine.ClearAutoMenuRequest()
	s.ensureAutoMenuDefaults()
	s.autoMenu = true
}

func (s *Session) ensureAutoMenuDefaults() {
	defaults := []struct {
		name  string
		value int32
	}{
		{"AUTOWAIT", 200},
		{"AUTOMIN", 2},
		{"AUTOMAX", 15},
	}
	for _, item := range defaults {
		if s.machine.Context().SystemVariables.Contains(item.name) {
			continue
		}
		s.machine.SetSystemVariable(item.name, script.EmonValueFromInt(item.value))
	}
}

func (s *Session) autoMenuValue(name string, fallback int) int {
	return s.machine.SettingInt(name, fallback)
}

func (s *Session) setAutoMenuValue(name string, value int) {
	s.machine.SetSystemVariable(name, script.EmonValueFromInt(int32(value)))
	// 设置要立即持久化：原引擎改动混音/设置时也是即时生效并写 emesys.dat。
	s.machine.FlushSystemState()
}

func (s *Session) autoMenuLayout() (int, int, []autoMenuButton) {
	w := autoMenuWidth
	h := autoMenuHeight
	if s.width > 0 && s.width < w+40 {
		w = s.width - 40
	}
	left := (s.width - w) / 2
	top := (s.height - h) / 2
	buttons := []autoMenuButton{}
	valueX := left + 260
	// 每行左侧数值、右侧两个步进按钮。
	for i, id := range []string{"wait", "min", "max"} {
		y := top + 40 + i*autoMenuRowH
		buttons = append(buttons,
			autoMenuButton{id: id + "-", x: valueX + 190, y: y, w: 44, h: 44, label: "－"},
			autoMenuButton{id: id + "+", x: valueX + 246, y: y, w: 44, h: 44, label: "＋"},
		)
	}
	buttons = append(buttons, autoMenuButton{
		id: "close", x: left + (w-200)/2, y: top + h - 76, w: 200, h: 48, label: "閉じる",
	})
	return left, top, buttons
}

func (s *Session) drawAutoMenu() {
	if !s.autoMenu || s.frame == nil {
		return
	}
	canvas := s.frame
	canvas.FillBlend(0, 0, s.width, s.height, 8, 10, 18, 170)
	left, top, buttons := s.autoMenuLayout()
	w := autoMenuWidth
	if s.width > 0 && s.width < w+40 {
		w = s.width - 40
	}
	canvas.Fill(left, top, w, autoMenuHeight, 16, 20, 28, 255)
	canvas.Fill(left, top, w, 3, 220, 230, 240, 255)
	canvas.Fill(left, top+autoMenuHeight-3, w, 3, 90, 100, 114, 255)
	canvas.Fill(left, top, 3, autoMenuHeight, 220, 230, 240, 255)
	canvas.Fill(left+w-3, top, 3, autoMenuHeight, 90, 100, 114, 255)

	glyphs := emontext.CurrentGlyphSource()
	if glyphs == nil {
		glyphs = emontext.ResolveGlyphSource()
	}
	DrawPromptLine(canvas, "オートモード設定", left+40, top+8, 24, glyphs, 235, 240, 248)
	wait := s.autoMenuValue("AUTOWAIT", 200)
	minV := s.autoMenuValue("AUTOMIN", 2)
	maxV := s.autoMenuValue("AUTOMAX", 15)
	rows := []struct {
		label string
		value string
	}{
		{"オートＷＡＩＴ", strconv.Itoa(wait)},
		{"オートＭＩＮ", strconv.Itoa(minV) + " 秒"},
		{"オートＭＡＸ", strconv.Itoa(maxV) + " 秒"},
	}
	for i, row := range rows {
		y := top + 40 + i*autoMenuRowH
		DrawPromptLine(canvas, row.label, left+40, y+10, 24, glyphs, 220, 226, 236)
		DrawPromptLine(canvas, row.value, left+300, y+10, 24, glyphs, 250, 252, 255)
	}
	for _, b := range buttons {
		canvas.Fill(b.x, b.y, b.w, b.h, 52, 60, 76, 255)
		canvas.Fill(b.x, b.y, b.w, 2, 120, 134, 158, 255)
		DrawPromptLineCenter(canvas, b.label, b.x, b.y, b.w, b.h, 24, glyphs, 240, 244, 250)
	}
}

func (s *Session) hitAutoMenu(x, y int) (string, bool) {
	_, _, buttons := s.autoMenuLayout()
	for _, b := range buttons {
		if x >= b.x && y >= b.y && x < b.x+b.w && y < b.y+b.h {
			return b.id, true
		}
	}
	return "", false
}

func (s *Session) handleAutoMenuClick(x, y int) bool {
	if !s.autoMenu {
		return false
	}
	id, ok := s.hitAutoMenu(x, y)
	if !ok {
		return true // 面板是模态的：点空白处也吞掉，避免误触游戏
	}
	switch id {
	case "close":
		s.autoMenu = false
		return true
	case "wait-", "wait+":
		value := core.Clamp(s.autoMenuValue("AUTOWAIT", 200), 0, 1000)
		if strings.HasSuffix(id, "+") {
			value += 20
		} else {
			value -= 20
		}
		s.setAutoMenuValue("AUTOWAIT", core.Clamp(value, 0, 1000))
	case "min-", "min+":
		value := core.Clamp(s.autoMenuValue("AUTOMIN", 2), 0, 60)
		if strings.HasSuffix(id, "+") {
			value++
		} else {
			value--
		}
		value = core.Clamp(value, 0, 60)
		s.setAutoMenuValue("AUTOMIN", value)
		if s.autoMenuValue("AUTOMAX", 15) < value {
			s.setAutoMenuValue("AUTOMAX", value)
		}
	case "max-", "max+":
		value := core.Clamp(s.autoMenuValue("AUTOMAX", 15), 1, 120)
		if strings.HasSuffix(id, "+") {
			value++
		} else {
			value--
		}
		value = core.Clamp(value, 1, 120)
		if minV := s.autoMenuValue("AUTOMIN", 2); value < minV {
			value = minV
		}
		s.setAutoMenuValue("AUTOMAX", value)
	}
	return true
}
