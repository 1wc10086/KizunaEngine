// 本文件是 NAME（主人公姓名入力）界面与它的弹簧输入框。
//
// 原版不是脚本自绘，而是引擎自己在装载 `sys_name_change` 这张整屏图时弹出的
// Windows 对话框（手册 NAME.JPG 就是"引擎底图 + 系统对话框"的组合）：
//
//	┌ 名前の入力 ────────────────┐
//	│ 主人公の名前を設定してください。（全角文 │
//	│ [アーロン                ] │
//	│ [ 決定 ]      [ デフォルト ] │
//	└───────────────────────────┘
//
// 默认值来自 _system 的 [GAMESYSTEM] PLAYER_NAME01（本作是 アーロン）。
// 決定后把名字写进脚本变量 ＄α（正文里的名字就是它），
// 所以这里必须与 VM 的变量表联动，而不是只改宿主的一个字符串。
//
// 输入法：安卓上没有硬件键盘，宿主会叠一层透明的 TextField 只负责 IME，
// 通过 core.PromptDialogExposer / PromptTextSetter 把文字同步进引擎，
// 由引擎把文字画进输入框（与 emon 的 prompt 对话框同一套协议）。
package session

import (
	"strings"

	"kizuna/enginego/core"
)

// nameEntry 是姓名输入的模态状态。
type nameEntry struct {
	active bool
	// text 是当前编辑中的文字（宿主 IME 通过 SetPromptText 写入）。
	text string
	// defaultName 是 _system 的 PLAYER_NAME01，デフォルト 按钮回到它。
	defaultName string
	// hover 是悬停的按钮下标（0=決定，1=デフォルト）。
	hover int
}

// 姓名对话框的屏幕矩形（按 NAME.JPG 的像素换算到 1280x720）。
//
// 截图里对话框占 (198,99)-(344,186)（544x306 基准），乘以 1280/544 = 2.353
// 得到 (466,233)-(809,438)。这些数字只用于绘制与命中，与原版系统对话框
// 的尺寸比例一致。
var (
	nameBox        = rectOf(466, 233, 343, 205)
	nameTitleBar   = rectOf(466, 233, 343, 38)
	nameTitleText  = pointOf(480, 243)
	nameMessage    = pointOf(480, 292)
	nameEditBox    = rectOf(482, 322, 308, 40)
	nameEditText   = pointOf(492, 332)
	nameConfirmBtn = rectOf(476, 376, 150, 42)
	nameDefaultBtn = rectOf(640, 376, 150, 42)
)

type irect struct{ X, Y, W, H int }

func rectOf(x, y, w, h int) irect      { return irect{X: x, Y: y, W: w, H: h} }
func pointOf(x, y int) irect           { return irect{X: x, Y: y} }
func (r irect) contains(x, y int) bool { return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H }

// nameEntryActive 报告姓名输入是否开着。
func (s *Session) nameEntryActive() bool { return s.nameEntry.active }

// beginNameEntry 打开姓名输入（脚本装载 sys_name_change 时调用）。
//
// 默认名优先用 _system 的 PLAYER_NAME01；脚本变量里已经有值时沿用它
// （读档后再次进入名字画面时，原版会回填上次输入的名字）。
func (s *Session) beginNameEntry() {
	if s.nameEntry.active {
		return
	}
	fallback := s.defaultPlayerName()
	current := strings.TrimSpace(string(s.vm.Vars()["＄α"]))
	s.nameEntry = nameEntry{active: true, defaultName: fallback, text: current}
	if s.nameEntry.text == "" {
		s.nameEntry.text = fallback
	}
	s.step = core.StepAwaitName
	s.dirty = true
}

// defaultPlayerName 取 _system 的 PLAYER_NAME01，缺省用 アーロン。
func (s *Session) defaultPlayerName() string {
	if s.system != nil {
		if name := strings.TrimSpace(s.system.Get("GAMESYSTEM", "PLAYER_NAME01")); name != "" {
			return name
		}
	}
	return "アーロン"
}

// confirmNameEntry 把输入写进脚本变量并继续执行。
//
// 全角 5 字以内的限制是原版的（_system 的 NAME_DRAW_WIDTH 与手册说明）；
// 超出部分截断，而不是拒绝输入——拒绝会让玩家在触屏上以为"決定 没反应"。
func (s *Session) confirmNameEntry() (core.Step, error) {
	if !s.nameEntry.active {
		return s.step, nil
	}
	name := strings.TrimSpace(s.nameEntry.text)
	if name == "" {
		name = s.nameEntry.defaultName
	}
	name = truncateRunes(name, 5)
	s.nameEntry.active = false
	// 主角名的脚本变量是 ＄α（剧本里所有主角台词的名字都是它）。
	s.vm.SetVar("＄α", name)
	// 确认后脚本要重新执行"当前停住的那一句"，让正文里的 ＄α 立刻变成新名字。
	// 不重跑的话，玩家确认后看到的第一句名字仍是字面的「＄α」。
	s.vm.Replay()
	s.step = core.StepContinue
	// 名字确认后脚本继续往下跑（start 脚本随后会切到 00_00）。
	return s.pump(commandBudget)
}

// truncateRunes 按字符数截断（日文一个全角字就是一个 rune）。
func truncateRunes(text string, limit int) string {
	if limit <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}

// SetPromptText 由宿主 IME 调用：把当前编辑文字同步进引擎。
func (s *Session) SetPromptText(text string) {
	if !s.nameEntry.active {
		return
	}
	s.nameEntry.text = text
	s.dirty = true
}

// PromptDialog 是 core.PromptDialogExposer 的实现。
//
// 只暴露按钮与文案；输入框的文字由引擎自己画（宿主只留透明 IME 层）。
func (s *Session) PromptDialog() core.PromptDialog {
	if !s.nameEntry.active {
		return core.PromptDialog{}
	}
	return core.PromptDialog{
		Active:    true,
		YesNo:     false,
		Title:     "名前の入力",
		Prompt:    "主人公の名前を設定してください。",
		Edit:      s.nameEntry.text,
		MaxLength: 5,
		Buttons: []core.PromptButton{
			{ID: core.PromptButtonConfirm, X: int32(nameConfirmBtn.X), Y: int32(nameConfirmBtn.Y), W: int32(nameConfirmBtn.W), H: int32(nameConfirmBtn.H)},
			{ID: core.PromptButtonCancel, X: int32(nameDefaultBtn.X), Y: int32(nameDefaultBtn.Y), W: int32(nameDefaultBtn.W), H: int32(nameDefaultBtn.H)},
		},
	}
}

// pointerNameEntry 处理姓名画面的点击。返回 handled=false 表示交给外层。
func (s *Session) pointerNameEntry(op uint32, x, y int) (bool, error) {
	if !s.nameEntry.active {
		return false, nil
	}
	switch op {
	case 0:
		switch {
		case nameConfirmBtn.contains(x, y):
			s.nameEntry.hover = 0
		case nameDefaultBtn.contains(x, y):
			s.nameEntry.hover = 1
		default:
			s.nameEntry.hover = -1
		}
		return true, nil
	case 1, 3:
		switch {
		case nameConfirmBtn.contains(x, y):
			_, err := s.confirmNameEntry()
			return true, err
		case nameDefaultBtn.contains(x, y):
			// デフォルト：把输入框填回默认名（原版就是"重置为默认"，
			// 不是"取消"——手册截图两个按钮的字就是 決定 / デフォルト）。
			s.nameEntry.text = s.nameEntry.defaultName
			s.dirty = true
			return true, nil
		}
		return true, nil
	}
	return true, nil
}

// composeNameEntry 把姓名对话框画进帧。
//
// 底图是脚本刚装载的 sys_name_change（舞台层已经贴好），这里只叠对话框：
// 标题条 + 说明文字 + 输入框 + 两个按钮。
func (s *Session) composeNameEntry() {
	if !s.nameEntry.active {
		return
	}
	canvas := s.stage.Canvas()
	// 外框：系统对话框的浅灰底 + 深色标题条。
	canvas.Fill(nameBox.X, nameBox.Y, nameBox.W, nameBox.H, 0xE6, 0xE6, 0xE6, 0xFF)
	canvas.Fill(nameTitleBar.X, nameTitleBar.Y, nameTitleBar.W, nameTitleBar.H, 0x4A, 0x40, 0x3C, 0xFF)
	// 输入框：白底 + 细边框。
	canvas.Fill(nameEditBox.X, nameEditBox.Y, nameEditBox.W, nameEditBox.H, 0xFF, 0xFF, 0xFF, 0xFF)
	for _, edge := range [][4]int{
		{nameEditBox.X, nameEditBox.Y, nameEditBox.W, 1},
		{nameEditBox.X, nameEditBox.Y + nameEditBox.H - 1, nameEditBox.W, 1},
		{nameEditBox.X, nameEditBox.Y, 1, nameEditBox.H},
		{nameEditBox.X + nameEditBox.W - 1, nameEditBox.Y, 1, nameEditBox.H},
	} {
		canvas.Fill(edge[0], edge[1], edge[2], edge[3], 0x80, 0x80, 0x80, 0xFF)
	}
	// 两个按钮的底与边框。
	for i, btn := range []irect{nameConfirmBtn, nameDefaultBtn} {
		light := 0xF2
		if s.nameEntry.hover == i {
			light = 0xD0
		}
		canvas.Fill(btn.X, btn.Y, btn.W, btn.H, byte(light), byte(light), byte(light), 0xFF)
		for _, edge := range [][4]int{
			{btn.X, btn.Y, btn.W, 1},
			{btn.X, btn.Y + btn.H - 1, btn.W, 1},
			{btn.X, btn.Y, 1, btn.H},
			{btn.X + btn.W - 1, btn.Y, 1, btn.H},
		} {
			canvas.Fill(edge[0], edge[1], edge[2], edge[3], 0x60, 0x60, 0x60, 0xFF)
		}
	}
	// 文字：标题、说明、输入内容、按钮字。原版的系统对话框用黑字。
	const dark = 0xFF101010
	s.drawUIRun("名前の入力", nameTitleText.X, nameTitleText.Y, 20, 0xFFF0F0F0, nameTitleBar.W-24)
	s.drawUIRun("主人公の名前を設定してください。（全角文）", nameMessage.X, nameMessage.Y, 16, dark, nameBox.W-24)
	s.drawUIRun(s.nameEntry.text, nameEditText.X, nameEditText.Y, 20, dark, nameEditBox.W-20)
	s.drawUIRun("決定", nameConfirmBtn.X+nameConfirmBtn.W/2-20, nameConfirmBtn.Y+10, 18, dark, nameConfirmBtn.W)
	s.drawUIRun("デフォルト", nameDefaultBtn.X+nameDefaultBtn.W/2-45, nameDefaultBtn.Y+10, 18, dark, nameDefaultBtn.W)
}

// SubmitName 是 core.NamePrompter 的实现：宿主 NameDialog 路径（非引擎自绘）
// 提交名字时走这里。本作的对话框是引擎自绘，但保留这个入口让旧宿主也能用。
func (s *Session) SubmitName(name string) bool {
	if !s.nameEntry.active {
		return false
	}
	s.nameEntry.text = name
	if _, err := s.confirmNameEntry(); err != nil {
		s.lastErr = err.Error()
		return false
	}
	return true
}
