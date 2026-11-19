package vm

import (
	"strconv"
	"strings"

	"kizuna/enginego/brand/emon/script"
)

// 既読/未読与 SkipModeFlag 的位定义，对应原引擎脚本里的判断：
//
//	(SkipModeFlag&0x01)!=0  全文本跳过（未読スキップ=ON）
//	(SkipModeFlag&0x02)!=0  仅既読跳过（未読スキップ=OFF）
//	(SkipModeFlag&0x04)!=0  AUTO
//	(SkipModeFlag&0x10)!=0  当前正文已读（BOKUONE 的 既読アイコン）
//
// 0x08 在原版里对应“Ctrl 快进”一类快速跳过，移动端没有等效按键，保留位定义。
const (
	skipModeAll      = 0x01
	skipModeReadOnly = 0x02
	skipModeAuto     = 0x04
	skipModeFast     = 0x08
	skipModeRead     = 0x10
)

// noteTextLine 在正文被追加到消息框时调用。key 用“脚本名:源行号”而不是
// 正文内容：译文补丁会改正文，但行号稳定；同一行被脚本循环重放时也仍然命中。
// 已读状态必须在“显示这一行之前”读取，所以这里立刻算出 currentTextRead，
// 然后才把该行写进已读库——首次遇到的行对宿主仍然是 unread（未読スキップ
// 需要在它上面停一次），但下一次就会显示既読图标。
func (m *Machine) noteTextLine(program string, line int32) {
	if m == nil || m.context == nil {
		return
	}
	if program == "" {
		program = "?"
	}
	key := program + ":" + strconv.Itoa(int(line))
	if key == m.readKey {
		// 同一脚本行里的后半句（Inline 正文）再次追加时不能重新判定：
		// 第一次追加已经把该行标成已读，再查会得到 true，首轮就误显示既読图标。
		return
	}
	m.readKey = key
	read := false
	if m.context.Reads != nil {
		read = m.context.Reads.IsRead(key)
	}
	m.currentTextRead = read
	if !read && m.context.Reads != nil {
		m.context.Reads.MarkRead(key)
	}
	m.refreshSkipModeFlag()
}

// MessageUnread 返回“当前这行在显示前是否未读”，供宿主实现
// 「未読文章もスキップする」的停点。
func (m *Machine) MessageUnread() bool {
	if m == nil {
		return false
	}
	return m.readKey != "" && !m.currentTextRead
}

// SetHostSkip / SetHostAuto 接收宿主工具栏的 SKIP/AUTO 开关。
// 原引擎把这两种状态写进 SkipModeFlag，界面上的 AUTO/SKIP/既読 图标
// 全部由脚本的 NUMTRUE 触发器读取该变量后自行显示，宿主不需要管图标。
func (m *Machine) SetHostSkip(value bool) {
	if m == nil || m.hostSkip == value {
		return
	}
	if value && m.readKey != "" && !m.currentTextRead && m.context != nil && m.context.Reads != nil && m.context.Reads.IsRead(m.readKey) {
		// 玩家在一条未读句上被“未読スキップ”停住后再次打开跳过：
		// 这一行已经显示过（noteTextLine 已入库），不应该再次停在同一行。
		m.currentTextRead = true
	}
	m.hostSkip = value
	m.refreshSkipModeFlag()
}

func (m *Machine) SetHostAuto(value bool) {
	if m == nil || m.hostAuto == value {
		return
	}
	m.hostAuto = value
	m.refreshSkipModeFlag()
}

// refreshSkipModeFlag 重新计算并写入 SkipModeFlag。只在该值变化时写变量，
// 避免每帧无意义的写操作唤醒脚本触发器。
func (m *Machine) refreshSkipModeFlag() {
	if m == nil || m.context == nil {
		return
	}
	bits := 0
	if m.hostAuto || m.auto {
		bits |= skipModeAuto
	}
	if m.hostSkip || m.skip {
		if m.SettingInt("未読スキップ", 0) == 1 {
			bits |= skipModeAll
		} else {
			bits |= skipModeReadOnly
		}
	}
	if m.currentTextRead {
		bits |= skipModeRead
	}
	current := m.Lookup("SkipModeFlag").AsInt()
	if current == int32(bits) {
		return
	}
	m.SetSystemVariable("SkipModeFlag", script.EmonValueFromInt(int32(bits)))
	// 图标由 PumpFrame 的每帧触发器轮询刷新（宿主停在 AwaitInput 时
	// 不会持续调 Run，只靠 Run 里的轮询会让图标等到下一次点击才更新）。
}

// textKeyOf 仅用于测试/诊断：把 readKey 拆成脚本名与行号。
func textKeyOf(key string) (string, int, bool) {
	idx := strings.LastIndex(key, ":")
	if idx <= 0 {
		return "", 0, false
	}
	line, err := strconv.Atoi(key[idx+1:])
	if err != nil {
		return "", 0, false
	}
	return key[:idx], line, true
}
