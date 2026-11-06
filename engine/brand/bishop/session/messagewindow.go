package session

import (
	"strings"
	"time"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

// messagewindow.go 是原版对话框底部工具条的命中与动作。
//
// 画面由 rendering.MessageWindowRenderer 画；这里只做两件渲染器不该做的事：
// 判断鼠标落在哪个按钮上，以及把按钮映射成引擎动作。工具条只在
// "引擎绘制对话框"模式（showMessageFrame）下常驻，关闭时仍走原来的
// [game menu] 悬停/ESC 菜单。

// messageWindowVisible 判断这一帧是否应该由引擎绘制对话框。
func (s *Session) messageWindowVisible() bool {
	if s.gameState == nil || s.composer == nil || s.config == nil {
		return false
	}
	// 没有 [message window] 配置的作品不能声称"引擎已画"，否则宿主会隐藏
	// Flutter 的 MessageBox，玩家就看不到文字了。
	if s.config.Message.Empty() {
		return false
	}
	// 只认主消息框。window part=2 在本作里出现得很频繁，但它的语义还没
	// 确认；如果把它也算成"有消息框"，主框会被一个没有正文的空框顶出来，
	// 玩家会看到上一句文本一直挂着。
	if !s.gameState.ShowMessageFrame() || !s.gameState.MessageWindowVisible() {
		return false
	}
	if s.titleMode || s.ui != nil || s.menuVisible {
		return false
	}
	return len(s.gameState.Text()) > 0 || len(s.gameState.MultiText()) > 0
}

// messageButtonAt 返回命中的工具条按钮下标；-1 表示没有命中。
func (s *Session) messageButtonAt(x, y int) int {
	if !s.toolbarVisible || s.config == nil || s.composer == nil {
		return -1
	}
	offset := s.messageWindowOffset()
	for index, button := range s.config.GameMenu.Buttons {
		if offset != 0 && button.Y >= s.config.Message.BaseY {
			button.Y += offset
		}
		width, height := button.Width, button.Height
		if button.Image != "" {
			image := s.composer.Images.Get(button.Image)
			if image == nil {
				continue
			}
			frames := max(1, button.Frames)
			width = image.Width
			height = image.Height / frames
		}
		if width <= 0 || height <= 0 {
			continue
		}
		if x >= button.X && y >= button.Y && x < button.X+width && y < button.Y+height {
			return index
		}
	}
	return -1
}

// insideToolbarStrip 判断点是否落在 Ancient 叠层工具条的外接矩形内。
//
// 图标按钮的尺寸来自图片本身，GameMenu 里 Width/Height 为 0；必须用图片尺寸
// 求包围框，否则框为空，工具条空白处的点击会漏到正文里推进对白。
func (s *Session) insideToolbarStrip(x, y int) bool {
	if s.composer == nil || s.config == nil {
		return false
	}
	left, top, right, bottom := 0, 0, 0, 0
	found := false
	for _, button := range s.config.GameMenu.Buttons {
		width, height := button.Width, button.Height
		if button.Image != "" {
			image := s.composer.Images.Get(button.Image)
			if image == nil {
				continue
			}
			width = image.Width
			height = image.Height / max(1, button.Frames)
		}
		if width <= 0 || height <= 0 {
			continue
		}
		if !found {
			left, top = button.X, button.Y
			right, bottom = button.X+width, button.Y+height
			found = true
			continue
		}
		left = min(left, button.X)
		top = min(top, button.Y)
		right = max(right, button.X+width)
		bottom = max(bottom, button.Y+height)
	}
	if !found {
		return false
	}
	return x >= left && x < right && y >= top && y < bottom
}

// pointerMessageWindow 处理引擎对话框上的指针事件。
//
// 返回 handled=false 时调用方继续按"点击推进对白"处理：点在正文区域仍然要能
// 翻页，只有点在底部工具条上才吞掉这次点击。
func (s *Session) pointerMessageWindow(op uint32, x, y int32) (core.Step, bool) {
	if s.gameState == nil || !s.gameState.ShowMessageFrame() || s.titleMode || s.ui != nil || s.menuVisible {
		return s.lastStep, false
	}
	// 工具条隐藏时点击全部交给"推进对白"，不要吞底部区域。
	if !s.toolbarVisible {
		return s.lastStep, false
	}
	switch op {
	case 0, 1:
		// press 只更新悬停；真正触发放在 release/tap，避免手指按下时误触。
		hover := s.messageButtonAt(int(x), int(y))
		if hover != s.messageHover {
			s.messageHover = hover
			s.composer.Invalidate()
		}
		return s.lastStep, false
	case 2, 3:
		index := s.messageButtonAt(int(x), int(y))
		// 地图阶段引擎不画消息工具条，但按钮坐标仍在：不拦住的话，玩家点
		// 地图底部空白（y≈564）会静默执行 QSAVE/QLOAD。地图上的点击必须
		// 留给地图命中，不能喂给隐藏按钮。
		if index >= 0 && !(s.mapShown && !s.messageWindowVisible()) {
			s.activateMessageButton(index)
			s.composer.Invalidate()
			return s.lastStep, true
		}
		// 对话框没在画的时候（地图/无对白），只有真正的按钮命中才吞点击；
		// 否则底部 34px 会被当成工具条空白，地图上的操场/地面点不动。
		if !s.messageWindowVisible() {
			break
		}
		// 点在工具条范围内但没命中按钮：吞掉，避免点空白处推进对白。
		if s.config == nil {
			break
		}
		if !s.config.GameMenu.Overlay {
			// 旧式底部菜单条：整条按钮行的最大 Y 作为分界。
			// 不能拿 GameMenu.BaseY 当分界，那会把正文翻页也吞掉。
			barTop := 0
			offset := s.messageWindowOffset()
			for _, button := range s.config.GameMenu.Buttons {
				y := button.Y
				if offset != 0 && y >= s.config.Message.BaseY {
					y += offset
				}
				barTop = max(barTop, y)
			}
			if barTop > 0 && int(y) >= barTop {
				return s.lastStep, true
			}
		} else if s.insideToolbarStrip(int(x), int(y)) {
			// Ancient 的工具图标叠在对话框右侧：只吞图标区内部的空白，
			// 正文区域仍然要能点一下翻页。
			return s.lastStep, true
		}
	}
	return s.lastStep, false
}

func (s *Session) activateMessageButton(index int) {
	buttons := s.config.GameMenu.Buttons
	if index < 0 || index >= len(buttons) {
		return
	}
	switch buttons[index].Key {
	case "save":
		if !s.openDataScreen(true) {
			// 作品没有 [data screen] 布局时退回宿主存档面板：至少让玩家
			// 能保存/读取，而不是点了按钮毫无反应。
			s.setHostRequest("save")
		}
	case "load":
		if !s.openDataScreen(false) {
			s.setHostRequest("load")
		}
	case "delete":
		// DELETE 只负责进入删除模式，真正的删除发生在存档画面点击槽位时（见 systemui.go）。
		// 没有存档画面的作品无处可删，只记日志，不静默吞掉按钮。
		if s.openDataScreen(false) && s.ui != nil {
			s.ui.deleteMode = true
			s.composer.Invalidate()
		} else {
			core.Logf("message: delete mode unavailable without data screen")
		}
	case "quick save", "qsave":
		s.quickSave()
	case "quick load", "qload":
		s.quickLoad()
	case "config":
		if !s.openConfigScreen() {
			s.setHostRequest("settings")
		}
	case "dress":
		// ヒメドレイ 的 DRESS 在 bu<角色><服装3位>00 的八套身体图之间循环，
		// 表情后缀保持不变。
		if s.bsStage != nil && s.bsStage.ToggleRev6Dress() {
			if s.composer != nil {
				s.composer.Invalidate()
			}
			break
		}
		// Rev5 的 cs 立绘把服装做进图名（着衣/下着/全裸三套身体图），
		// 点 DRESS 必须立刻换一整套身体图，而不是打开换装界面。
		if s.bsStage != nil && s.bsStage.ToggleLegacyDress() {
			if s.composer != nil {
				s.composer.Invalidate()
			}
			break
		}
		// 学園2 老版没有 [dress screen]：DRESS 按钮只切换 clNNNNN 图层
		// 的着衣/全裸后缀，和原引擎的全局开关一致。
		if s.toggleGakuen2Dress() {
			break
		}
		// 其它作品的立ち絵 / 脱衣设定画面：引擎自绘，打不开时退回宿主设置。
		if !s.openDressScreen() {
			s.setHostRequest("settings")
		}
	case "log":
		// 回想由引擎自绘（[log screen] + 引擎字体）；没有配置时才退回宿主。
		if !s.openLogScreen() {
			s.setHostRequest("backlog")
		}
	case "status":
		// "状態"画面：布局在 define.bsa 的 status_layout，画面/条件全由它驱动。
		// 多数作品的 status 按钮是注释掉的，只有キョウセイ支配 等启用；
		// OH/OHIE 的这颗粒子叫 btn_karte，原版打开 [status2 screen] 的
		// ケダモノカルテ，openStatusScreen 会优先分流到 Karte。
		if !s.openStatusScreen() {
			core.Logf("message: status screen unavailable")
		}
	case "simple view":
		// MKIE 的 VIEW：顔 → 胸 → 股間 循环，见 simple_display.go。
		if !s.cycleSimpleView() {
			core.Logf("message: simple view unavailable (no free-sight/bustup)")
		}
	case "simple strip":
		// MKIE 的 STRIP：当前 bu 立绘循环脱衣；没有立绘时退回 Rev5 的换装。
		if !s.toggleSimpleStrip() {
			if s.bsStage != nil && s.bsStage.ToggleLegacyDress() {
				if s.composer != nil {
					s.composer.Invalidate()
				}
			} else {
				core.Logf("message: simple strip has no bustup to change")
			}
		}
	case "view up":
		// MK 只有一个 VIEW 药丸按钮，点击依次切三档视点；原版也是循环。
		if s.bsStage != nil && s.bsStage.CycleLegacyView() {
			if s.composer != nil {
				s.composer.Invalidate()
			}
			break
		}
		if !s.freeSightToggle(0) {
			s.gameState.SetViewMode(0)
			s.composer.Invalidate()
		}
	case "view middle":
		if s.bsStage != nil && s.bsStage.CycleLegacyView() {
			if s.composer != nil {
				s.composer.Invalidate()
			}
			break
		}
		if !s.freeSightToggle(1) {
			s.gameState.SetViewMode(1)
			s.composer.Invalidate()
		}
	case "view down":
		if s.bsStage != nil && s.bsStage.CycleLegacyView() {
			if s.composer != nil {
				s.composer.Invalidate()
			}
			break
		}
		if !s.freeSightToggle(2) {
			s.gameState.SetViewMode(2)
			s.composer.Invalidate()
		}
	case "cut in":
		s.settings.Cutin = !s.settings.Cutin
		s.saveGameSettings()
		s.setToolNotice("カットイン: " + onOff(s.settings.Cutin))
		s.composer.Invalidate()
	case "hold menu":
		s.settings.Hold = !s.settings.Hold
		s.saveGameSettings()
		s.setToolNotice("HOLD: " + onOff(s.settings.Hold))
		s.composer.Invalidate()
	case "menu display":
		s.toolbarVisible = !s.toolbarVisible
		if !s.toolbarVisible {
			s.messageHover = -1
		}
		s.setToolNotice("メニュー: " + onOff(s.toolbarVisible))
		s.composer.Invalidate()
	case "voice", "v.play", "vplay":
		// 「音声再生」= 重播当前句的语音。原版按钮位于工具条左侧，
		// 不改变脚本位置，只把最后一条 voice 再发一次。
		if s.lastVoice != "" {
			s.audio = append(s.audio, core.AudioEvent{
				Kind:    2,
				Path:    s.materialize(s.lastVoice),
				Volume:  s.volumeGain(kindVoice) * s.characterGain(s.lastVoice, true),
				Channel: noTarget,
				Group:   noTarget,
			})
			s.voicePlaying = true
			s.appendBgmAttenuation(true)
		}
	case "window hide":
		// 右上角的 X：收起对话框（下一条对白仍会自动出现）。
		s.gameState.SetMessageWindowVisible(false)
		s.composer.Invalidate()
	case "window up":
		// 原版对话框有上/中/下三档；▲ 往上，▼ 往下，到头不回绕。
		s.settings.MessageWindowPosition = clampInt(s.settings.MessageWindowPosition-1, 0, 2)
		s.saveGameSettings()
		s.applyDisplaySettings()
		s.composer.Invalidate()
	case "window down":
		s.settings.MessageWindowPosition = clampInt(s.settings.MessageWindowPosition+1, 0, 2)
		s.saveGameSettings()
		s.applyDisplaySettings()
		s.composer.Invalidate()
	case "quit":
		s.setHostRequest("exit")
	case "auto ex", "auto":
		s.setHostRequest("auto")
	case "skip":
		s.setHostRequest("skip")
	default:
		// 视角/HOLD 等按钮属于原作的扩展功能，当前作品没有接入，
		// 保留按钮图但不做动作，避免误触发。
		core.Logf("message: tool button '%s' is not implemented", buttons[index].Key)
	}
}

// messageButtonFrames 返回按钮图的状态帧；有些作品只配了单帧。
func messageButtonFrames(button config.MenuButton) int {
	if button.Frames > 0 {
		return button.Frames
	}
	return 1
}

// setToolNotice 记录一条短暂的工具按钮反馈，由对话框渲染器画在窗口内。
func (s *Session) setToolNotice(text string) {
	s.toolNotice = text
	s.toolNoticeAt = time.Now()
}

func onOff(value bool) string {
	if value {
		return "ON"
	}
	return "OFF"
}

// pointerGameMenuDirect 直接命中顶部/底部菜单按钮，不要求对话框处于
// "显示"状态。选择肢/地图会把对话框收起来，但原版仍然允许从这些画面
// 打开 SAVE/LOAD/CONFIG/LOG；旧的 pointerMessageWindow 在 ShowMessageFrame
// 为假时直接返回，导致菜单在这些画面上点不动。
func (s *Session) pointerGameMenuDirect(op uint32, x, y int32) (core.Step, bool) {
	if s.config == nil || !s.toolbarVisible || s.ui != nil {
		return s.lastStep, false
	}
	index := s.messageButtonAt(int(x), int(y))
	if index < 0 {
		return s.lastStep, false
	}
	// 底部的 SKIP/AUTO/VOICE 只在对话框真正显示时画出来；地图上它们虽然
	// 还有坐标，但不能被点到（否则点地图底部会误触发 skip/auto）。
	if s.config != nil && index < len(s.config.GameMenu.Buttons) {
		button := s.config.GameMenu.Buttons[index]
		if button.Y >= s.config.Message.BaseY && s.config.Message.BaseY > 0 && !s.messageWindowVisible() {
			return s.lastStep, false
		}
	}
	switch op {
	case 0, 1, 2, 3:
		if op == 0 || op == 1 {
			if index != s.messageHover {
				s.messageHover = index
				s.composer.Invalidate()
			}
			return s.lastStep, true
		}
		s.activateMessageButton(index)
		if s.composer != nil {
			s.composer.Invalidate()
		}
		return s.lastStep, true
	}
	return s.lastStep, false
}

// autoSelectChoiceIndex 读取设置里的"射精位置"自动选择，返回要自动点的
// 选项下标；没有匹配时返回 -1。原版在设置成「中出し/外出し」时，H 场景
// 里出现对应选项会直接替玩家选中，不再弹选择肢。
func (s *Session) autoSelectChoiceIndex() int {
	if s.settings.AutoSelect == 0 || s.gameState == nil || s.machine == nil || s.machine.IsBsScript() {
		return -1
	}
	items := s.gameState.SelectItems()
	if len(items) == 0 {
		return -1
	}
	wanted := []string{"中出し"}
	if s.settings.AutoSelect == 2 {
		wanted = []string{"外出し", "外出"}
	}
	for index, item := range items {
		text := item.Text
		for _, keyword := range wanted {
			if strings.Contains(text, keyword) {
				return index
			}
		}
	}
	return -1
}

// applyChoiceInRun 在选择肢循环内部替玩家选中一项，不递归调用 Run。
func (s *Session) applyChoiceInRun(index int) state.SessionStep {
	s.stopVoiceForAdvance()
	option := s.router.Choose(index)
	s.gameState.ClearChoices()
	s.gameState.SetSelectItems(nil)
	s.gameState.SetAwaitingInput(false)
	if option == nil {
		return state.SessionContinue
	}
	s.machine.ChooseOption(option.Index)
	if !s.machine.IsBsScript() {
		s.writeChoiceResult(*option)
	}
	return state.SessionContinue
}
