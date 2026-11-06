package config

import (
	"strconv"
	"strings"
)

// LogScreen 是 [log screen]（回想/バックログ）的布局。
//
// 原引擎的回想不是宿主列表，而是用底图 + 滚动条 + 每句可点击的语音图标
// 组成的一整屏；文字位置和宽度来自配置，字体沿用 [default font]。
//
// 注意 Base 在近年的 Bishop 作品里是注释掉的：发行版只给 `back color`
// 作为半透明底色。旧实现把"没有底图"当成"没有回想界面"，于是回想按钮
// 永远退回宿主的 Flutter 列表——那不是原版界面。Empty 因此必须同时接受
// bar / back color / voice icon 中的任意一项。
type LogScreen struct {
	Base string
	// BackColor 是 0xAARRGGBB 的整屏底色；HasBackColor 区分"没配"和"配了纯黑"。
	BackColor    uint32
	HasBackColor bool
	Lines        int
	LeftPos      int
	Width        int
	Bar          UIButton
	// Up/Down 是回想右侧的上下翻页箭头（[log screen] up button / down button）。
	// 原版把它们和滚动条一起摆成一条竖列；缺了它们，玩家只能滚轮/拖条，
	// 触屏上看起来就像"回想不能翻页"。
	Up         UIButton
	Down       UIButton
	VoiceIcon  string
	VoiceFrame string
	// FontSize 是回想正文的点数；0 表示沿用 [default font] 的 Size。
	// 学園（GK）的对话框是 24 号字，但回想列表是 16 号小字；共用同一份
	// 字体会让每行占两到三行，六行回想互相重叠。
	FontSize int
}

func (l LogScreen) Empty() bool {
	return len(l.Base) == 0 && !l.HasBackColor && l.Bar.Empty() &&
		len(l.VoiceIcon) == 0 && len(l.VoiceFrame) == 0
}

func parseLogScreen(document *IniDocument) LogScreen {
	screen := LogScreen{
		Base:       document.Get(LogScreenSection, "base", ""),
		Lines:      document.GetInt(LogScreenSection, "lines", 6),
		Width:      document.GetInt(LogScreenSection, "width", 700),
		VoiceIcon:  document.Get(LogScreenSection, "voice icon", ""),
		VoiceFrame: document.Get(LogScreenSection, "voice frame", ""),
	}
	if color, ok := ParseColor(document.Get(LogScreenSection, "back color", "")); ok {
		screen.BackColor = color
		screen.HasBackColor = true
	}
	if screen.Lines <= 0 {
		screen.Lines = 6
	}
	if screen.Width <= 0 {
		screen.Width = 700
	}
	if parts := ParsePositions(document.Get(LogScreenSection, "left pos", "")); len(parts) > 0 {
		screen.LeftPos = parts[0]
	}
	screen.Bar = parseUIButton(document.Get(LogScreenSection, "bar", ""))
	screen.Up = parseUIButton(document.Get(LogScreenSection, "up button", ""))
	screen.Down = parseUIButton(document.Get(LogScreenSection, "down button", ""))
	return screen
}

// ParseColor 解析配置里的 0xAARRGGBB / 0xRRGGBB 颜色。
// 第二个返回值区分"没配"与"配了纯黑"。
func ParseColor(text string) (uint32, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0, false
	}
	digits := trimmed
	if len(digits) >= 2 && (digits[:2] == "0x" || digits[:2] == "0X") {
		digits = digits[2:]
	}
	value, err := strconv.ParseUint(digits, 16, 32)
	if err != nil {
		return 0, false
	}
	return uint32(value), true
}
