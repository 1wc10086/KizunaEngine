// 本文件把点击/按键分派到引擎自绘的界面。
//
// 原版的分派顺序是：确认框 → 某个模态界面（暂停菜单/存读档/设置/回想/
// 脚本界面）→ 对话框底部工具条 → 剧情推进。顺序不能改：确认框盖在所有界面
// 之上；界面打开时正文点击必须被吃掉（否则点格子会连带推进一句对白）。
//
// 坐标一律是引擎坐标系（1280x720）。宿主已经把屏幕点换算好（见
// player_chrome 的 engineX/engineY），这里不再做缩放。
package session

import (
	"kizuna/enginego/brand/tmrhiro/config"
	"kizuna/enginego/brand/tmrhiro/vm"
	"kizuna/enginego/core"
)

// pointerOverlay 处理界面层的指针事件。返回 handled=false 表示"没吃到，
// 交给剧情/标题逻辑"。
func (s *Session) pointerOverlay(op uint32, x, y int32) (core.Step, bool, error) {
	px, py := int(x), int(y)
	// 记录位置：设置界面的悬停高亮需要它（跟具体控件无关）。
	s.overlayHoverX, s.overlayHoverY = px, py
	if s.overlay == overlayConfirm {
		return s.pointerConfirm(op, px, py)
	}
	if s.overlay != overlayNone {
		return s.pointerModal(op, px, py)
	}
	return s.step, false, nil
}

// pointerModal 分派"处于某个界面"时的点击。
func (s *Session) pointerModal(op uint32, x, y int) (core.Step, bool, error) {
	switch s.overlay {
	case overlayMenu:
		if step, handled, err := s.pointerMenu(x, y); handled {
			return step, true, err
		}
		// 暂停菜单打开时，指针停在顶栏（0..48）不算"点外面"：
		// 列表本身就在这条带子里，玩家从按钮之间滑过去很常见。
		if y < int(s.ui.Menu.Height) {
			return s.step, true, nil
		}
		// 点其它位置视为"关掉菜单"（原版行为）。
		if op == 1 || op == 3 {
			s.overlay = overlayNone
			s.overlayHover = -1
		}
		return s.step, true, nil
	case overlaySave, overlayLoad:
		return s.pointerSlotBoard(op, x, y)
	case overlayConfig:
		return s.pointerConfig(op, x, y)
	case overlayBacklog:
		return s.pointerBacklog(op, x, y)
	}
	return s.step, false, nil
}

// pointerConfirm 处理确认框：只有 YES/NO 两个热区，其它点击一律忽略。
func (s *Session) pointerConfirm(op uint32, x, y int) (core.Step, bool, error) {
	yes, no := s.confirmButtons()
	switch op {
	case 0:
		switch {
		case yes.Contains(x, y):
			s.overlayHover = 0
		case no.Contains(x, y):
			s.overlayHover = 1
		default:
			s.overlayHover = -1
		}
		return s.step, true, nil
	case 1, 3, 5:
		switch {
		case yes.Contains(x, y):
			return s.applyConfirm(true)
		case no.Contains(x, y):
			return s.applyConfirm(false)
		}
		if op == 5 {
			// 右键/取消 = NO。
			return s.applyConfirm(false)
		}
		return s.step, true, nil
	}
	return s.step, true, nil
}

// applyConfirm 执行确认结果。
//
// 取消只是关框；确认要按 action 走真实动作（覆盖存档、删档、回标题、退出、
// 改达成率），所以这里必须调用与按钮完全相同的实现，而不是各自复制一份。
func (s *Session) applyConfirm(yes bool) (core.Step, bool, error) {
	confirm := s.confirm
	s.confirm = nil
	s.overlay = overlayNone
	s.overlayHover = -1
	if !yes || confirm == nil {
		return s.step, true, nil
	}
	switch confirm.action {
	case "save":
		_ = s.Save(confirm.slot, "")
		s.openSlotBoard(true)
	case "load":
		if step, err := s.Load(confirm.slot); err != nil {
			return s.step, true, err
		} else {
			return step, true, nil
		}
	case "delete":
		if s.store != nil {
			_ = s.store.Delete(confirm.slot)
			_ = s.store.DeleteThumb(confirm.slot)
		}
		return s.step, true, nil
	case "title":
		return s.backToTitle()
	case "exit":
		s.requestHost("exit")
		return s.step, true, nil
	case "percent100":
		s.applyPercent(100)
		return s.step, true, nil
	case "percent0":
		s.applyPercent(0)
		return s.step, true, nil
	}
	return s.step, true, nil
}

// applyPercent 把鉴赏达成率改成 100%/0%（原版 MEMORY COMPLETE 的调试按钮）。
//
// 原版是把全局存档里的 CG/场景解锁位全置 1 或全清 0。我们把解锁集合
// （s.unlockedCG / s.unlockedScene）同样处理，于是"データ閲覧"格子立刻反映。
func (s *Session) applyPercent(percent int) {
	if percent >= 100 {
		for _, name := range s.cgList {
			s.unlockedCG[name] = true
		}
		for i := range s.sceneList {
			s.unlockedScene[i] = true
		}
		return
	}
	s.unlockedCG = make(map[string]bool)
	s.unlockedScene = make(map[int]bool)
}

// pointerMenu 处理暂停菜单的点击。
func (s *Session) pointerMenu(x, y int) (core.Step, bool, error) {
	for index, button := range s.ui.Menu.Buttons {
		if !button.Enabled {
			continue
		}
		rect := config.Rect{X: button.X, Y: button.Y, W: button.W, H: button.H}
		if !rect.Contains(x, y) {
			continue
		}
		switch toolbarAction(button.Key) {
		case ActionSave:
			s.openSlotBoard(true)
		case ActionLoad:
			s.openSlotBoard(false)
		case ActionSystem:
			s.openConfig()
		case ActionTitle:
			s.overlay = overlayConfirm
			s.confirm = &confirmState{prompt: "タイトル画面に戻りますか？", action: "title"}
		case ActionExit:
			s.overlay = overlayConfirm
			s.confirm = &confirmState{prompt: "ゲームを終了しますか？", action: "exit"}
		case ActionBacklog:
			s.openBacklog()
		case ActionAuto:
			s.toggleAuto()
		case ActionSkip:
			s.toggleSkip()
		case ActionQSave:
			_ = s.Save(0, "")
			s.notice = "クイックセーブしました"
		case ActionQLoad:
			if step, err := s.Load(0); err == nil {
				s.overlay = overlayNone
				return step, true, nil
			}
			s.notice = "クイックセーブがありません"
		case ActionMenu, ActionClose:
			s.overlay = overlayNone
		}
		s.overlayHover = index
		return s.step, true, nil
	}
	return s.step, false, nil
}

// pointerSlotBoard 处理存/读档界面的点击。
func (s *Session) pointerSlotBoard(op uint32, x, y int) (core.Step, bool, error) {
	board := s.ui.Save
	if s.overlay == overlayLoad {
		board = s.ui.Load
	}
	// 页签。
	for page := 0; page < board.PageMax; page++ {
		rect := config.Rect{X: board.PageX + page*board.PageStepX, Y: board.PageY, W: board.PageW, H: board.PageH}
		if rect.Contains(x, y) && (op == 1 || op == 3) {
			s.overlayPage = page
			return s.step, true, nil
		}
	}
	// 自动存档页签 / 删档 / 返回（三个都在底部那排按钮里）。
	autoRect := config.Rect{X: board.AutoX, Y: board.AutoY, W: board.AutoW, H: board.AutoH}
	if autoRect.Contains(x, y) && (op == 1 || op == 3) {
		s.overlayPage = board.PageMax
		return s.step, true, nil
	}
	returnRect := config.Rect{X: board.ReturnX, Y: board.ReturnY, W: board.ReturnW, H: board.ReturnH}
	if returnRect.Contains(x, y) && (op == 1 || op == 3) {
		s.overlay = overlayNone
		s.overlayHover = -1
		return s.step, true, nil
	}
	// 格子。
	perPage := slotCount(board)
	for i := 0; i < perPage; i++ {
		rect := s.slotRect(board, i)
		if !rect.Contains(x, y) {
			continue
		}
		if op == 0 {
			s.overlayHover = i
			return s.step, true, nil
		}
		if op != 1 && op != 3 {
			return s.step, true, nil
		}
		slot := uint32(s.overlayPage*perPage + i + 1)
		if s.overlay == overlaySave {
			// 已有存档时先确认覆盖（原版行为）。
			if _, has := s.slotEntry(slot); has {
				s.overlay = overlayConfirm
				s.confirm = &confirmState{prompt: "上書きしますか？", action: "save", slot: slot}
				return s.step, true, nil
			}
			if err := s.Save(slot, ""); err != nil {
				s.notice = err.Error()
			} else {
				// 存完立刻回到剧情：原版存一次就退出存档界面，玩家不必再点
				// "戻る"（手册流程是"开菜单 → セーブ → 选格 → 回到画面"）。
				s.overlay = overlayNone
				s.overlayHover = -1
				s.notice = "セーブしました (No." + padSlot(slot) + ")"
			}
			return s.step, true, nil
		}
		if _, has := s.slotEntry(slot); !has {
			return s.step, true, nil
		}
		step, err := s.Load(slot)
		s.overlay = overlayNone
		s.overlayHover = -1
		if err != nil {
			return s.step, true, err
		}
		return step, true, nil
	}
	return s.step, true, nil
}

// pointerConfig 处理设置界面。
//
// 命中顺序：拖动（op=6，只对滑杆有效）→ 左侧页签 → 底部三个按钮 → 当前页
// 的控件。控件一律按"单元的真实几何"命中（见 systemPartAt）：
//
//	按钮组：第 i 格在 (X + col*ColStep, Y + row*RowStepY)，每格 W×H；
//	滑杆：一组是 Total 条纵向排列，每条轨道宽 SlideX，命中给出"第几条"，
//	      具体值由点击的 x 换算（slidePercent）。
//
// 早期实现按"整行宽带"算命中、再按整行均分算值，结果是滑杆与同一行的
// 开关组重叠（点滑杆先命中开关组），并且第 2 条以后的滑杆永远命中不到。
func (s *Session) pointerConfig(op uint32, x, y int) (core.Step, bool, error) {
	px, py := int(x), int(y)
	// 悬停位置：控件高亮与按钮高亮都用它。
	s.overlayHoverX, s.overlayHoverY = px, py
	// 滑杆拖动：按住不放时连续改值，原版就是这么操作的（没有"确定"按钮）。
	if op == 6 {
		if part, rect, row, ok := s.systemPartAt(px, py); ok && part.Kind == "SW_SLIDE" {
			s.applySystemSlide(part, rect, row, s.slidePercent(part, px))
			return s.step, true, nil
		}
	}
	// 左侧页签。
	for index, rect := range s.ui.System.PageButtons {
		if rect.Contains(px, py) && (op == 1 || op == 3) {
			s.overlayPage = index
			s.overlayHover = index
			return s.step, true, nil
		}
	}
	// 左下角三个动作按钮（顺序与坐标都来自 _system，不能按字段名猜）：
	//
	//	S_GAME  (70,597) → 戻る（关掉设置页）
	//	S_END   (70,627) → ゲーム終了
	//	S_TITLE (70,657) → タイトル画面
	if s.ui.System.GameButton.Contains(px, py) && (op == 1 || op == 3) {
		s.overlay = overlayNone
		s.saveSettings()
		return s.step, true, nil
	}
	if s.ui.System.EndButton.Contains(px, py) && (op == 1 || op == 3) {
		s.overlay = overlayConfirm
		s.confirm = &confirmState{prompt: "ゲームを終了しますか？", action: "exit"}
		return s.step, true, nil
	}
	if s.ui.System.TitleButton.Contains(px, py) && (op == 1 || op == 3) {
		s.overlay = overlayConfirm
		s.confirm = &confirmState{prompt: "タイトル画面に戻りますか？", action: "title"}
		return s.step, true, nil
	}
	// 控件：命中即改设置（原版点一下立刻生效）。
	if part, rect, index, ok := s.systemPartAt(px, py); ok {
		if op != 1 && op != 3 {
			return s.step, true, nil
		}
		if part.Kind == "SW_SLIDE" {
			s.applySystemSlide(part, rect, index, s.slidePercent(part, px))
		} else {
			s.applySystemPart(part, rect, index)
		}
		return s.step, true, nil
	}
	return s.step, true, nil
}

// applySystemPart 处理按钮组被点击：按命中的单元下标改成对应设置。
//
// 原版每个控件都直接改全局设置（音量、速度、开关），没有"确定"按钮，
// 所以这里点一下立刻生效并落盘。index 由 systemPartAt 按真实几何算出来，
// 这里不再做第二次换算——早期实现用"整行宽度均分"重算，ON/OFF 会反过来、
// 多行控件永远只会命中第一行。
func (s *Session) applySystemPart(part config.SystemPart, rect config.Rect, index int) {
	switch part.Key {
	case "S_TEXTSPEED":
		s.textSpeed = clampIntValue(index+1, 10)
	case "S_AUTOSPEED":
		s.autoSpeed = clampIntValue(index+1, 10)
		s.autoWait = autoDelay(s.autoSpeed)
	case "S_SKIP":
		s.skipUnread = index == 1
	case "S_SKIP_H_STOP":
		s.ceaseSkip = index == 1
	case "S_GRAPHIC":
		s.effects = index == 1
	case "S_AUTO_VOICE_CUT":
		s.autoVoiceCut = index == 1
	case "S_READ_TEXT_COLOR":
		s.readColorOn = index == 1
	case "S_VULGAR_WORD":
		s.vulgarWord = index == 1
	case "S_WINDOW_BASE_DRAW":
		// 「テキストウィンドウのベースを表示する」：关掉时对话框只留文字。
		s.windowBaseDraw = index == 1
	case "S_MES_CONFIRM":
		// 確認メッセージ：4 行 × 2 列，每一行是独立的开关
		// （Save / Load / QuickSave / QuickLoad），保存时按"键名#行号"记列号，
		// 绘制时 systemRowChoice 用同一套键回读，所以点第 3 行只改第 3 行。
		if part.PerRow > 0 && part.Total > part.PerRow {
			row, col := index/part.PerRow, index%part.PerRow
			s.storeSystemChoice(part.Key+"#"+itoa(row), col)
			break
		}
		s.storeSystemChoice(part.Key, index)
	case "S_SOUND_HSE", "S_SOUND_HSE03", "S_VP_SOUND_DOWN",
		"S_SOUND_CHECK", "S_FONT_SELECT", "S_RCLICK", "S_BWHEEL",
		"S_NS_MODE", "S_WS_MODE", "S_SCREEN", "S_CLOTHES",
		"S_WINDOW_STAT_DEFAULT", "S_SKIP_S_STOP", "S_SKIP_V_STOP",
		"S_SKIP_E_STOP", "S_FINISH_EFFECT", "S_ECSTACY_NAVI",
		"S_ECSTACY_NAVI_COUNT", "S_VOICE_CUT", "S_TEST_GRAPHIC",
		"S_TEST_ONOFF", "S_PAGE", "S_E_SPOT_I", "S_E_SPOT_F",
		"S_E_SPOT_P", "S_E_SPOT_E":
		// 设备/交互类选项（右键行为、滚轮、窗口模式、确认框、换装开关…）。
		// 这些在触屏移植里没有对应的可观察行为，但选择必须被记住并回显，
		// 否则玩家点完看不到任何反馈，会以为界面坏了。
		if part.PerRow > 0 && part.Total > part.PerRow {
			// 多行控件（サウンド的 8×2 两行）：每一行各自一个开关。
			row, col := index/part.PerRow, index%part.PerRow
			s.storeSystemChoice(part.Key+"#"+itoa(row), col)
			break
		}
		s.storeSystemChoice(part.Key, index)
	case "S_CHARASET":
		// 「キャラ設定」入口：原版跳到一个角色自定义画面；本移植没有该画面，
		// 点了保持在设置页（至少不吞掉点击）。
	}
	s.saveSettings()
}

// systemPartAt 返回当前页上命中的控件、它的第几个单元与该单元的矩形。
//
// 命中必须按**单元**算，而不是整行：ON/OFF 组占两个单元（各 W 宽），
// 滑杆是"多条轨道纵向排列"。按整行宽度算会让滑杆与同一行的开关组重叠，
// 点滑杆先命中开关组，于是滑块拖不动、开关也点不中。
func (s *Session) systemPartAt(x, y int) (config.SystemPart, config.Rect, int, bool) {
	for _, part := range s.ui.System.Parts {
		if part.Page != s.overlayPage+1 || part.W <= 0 || part.H <= 0 {
			continue
		}
		if part.Kind == "SW_SLIDE" {
			// 一组滑杆占 Total 条、每条行距 RowStepY 的整块区域。
			height := part.H
			if part.Total > 1 && part.RowStepY > 0 {
				height = part.RowStepY*(part.Total-1) + part.H
			}
			width := part.SlideX
			if width <= 0 {
				width = part.W
			}
			rect := config.Rect{X: part.X, Y: part.Y, W: width, H: height}
			if rect.Contains(x, y) {
				// 命中的是"第几条滑杆"（行号）；具体值由 x 换算。
				row := 0
				if step := part.RowStepY; step > 0 && part.Total > 1 {
					row = (y - rect.Y) / step
				}
				if row < 0 {
					row = 0
				}
				if row >= part.Total {
					row = part.Total - 1
				}
				return part, rect, row, true
			}
			continue
		}
		for i := 0; i < part.Total; i++ {
			cx, cy := s.systemPartCell(part, i)
			rect := config.Rect{X: cx, Y: cy, W: part.W, H: part.H}
			if rect.Contains(x, y) {
				return part, rect, i, true
			}
		}
	}
	return config.SystemPart{}, config.Rect{}, 0, false
}

// slidePercent 按点击（或拖动）的 x 换算成 0..100 的百分比。
func (s *Session) slidePercent(part config.SystemPart, x int) int {
	width := part.SlideX
	if width <= 0 {
		width = part.W
	}
	within := x - part.X
	if within < 0 {
		within = 0
	}
	if within > width {
		within = width
	}
	return within * 100 / maxInt(1, width)
}

// applySystemSlide 处理滑杆的一条被拖动/点击。
//
// row 是这一组里的第几条，percent 是 0..100：
//   - サウンド：0 BGM / 1 ボイス / 2 通常SE / 3 H系SE；
//   - 一般：不透明度与 R/G/B（没有可观察行为，只记档位）。
//
// 同时把档位存进 systemChoices（键名带行号），滑块位置下次进入时按它回显。
func (s *Session) applySystemSlide(part config.SystemPart, rect config.Rect, row int, percent int) {
	percent = clampIntValue(percent, 100)
	switch part.Key {
	case "S_SOUND":
		switch row {
		case 0:
			s.bgmVolume = float64(percent) / 100
		case 1:
			s.voiceVolume = float64(percent) / 100
		default:
			s.seVolume = float64(percent) / 100
		}
	case "S_SOUND_VOLUME":
		switch row {
		case 0:
			s.bgmVolume = float64(percent) / 100
		case 1:
			s.voiceVolume = float64(percent) / 100
		default:
			s.seVolume = float64(percent) / 100
		}
	}
	s.storeSystemChoice(part.Key+"#"+itoa(row), percent)
	s.saveSettings()
}

// pointerBacklog 处理回想：滚轮/上下拖动滚动，右键关闭。
func (s *Session) pointerBacklog(op uint32, x, y int) (core.Step, bool, error) {
	switch op {
	case 4, 5:
		s.overlay = overlayNone
		return s.step, true, nil
	case 6:
		// 拖动：按 y 增量滚动（宿主每帧送绝对坐标，这里用差分）。
		if s.backlogDragY != 0 {
			delta := y - s.backlogDragY
			if delta > 6 {
				s.scrollBacklog(1)
				s.backlogDragY = y
			} else if delta < -6 {
				s.scrollBacklog(-1)
				s.backlogDragY = y
			}
		} else {
			s.backlogDragY = y
		}
		return s.step, true, nil
	}
	return s.step, true, nil
}

// scrollBacklog 按行滚动回想。
func (s *Session) scrollBacklog(delta int) {
	maxScroll := len(s.backlog) - s.backlogLayout().maxLines
	if maxScroll < 0 {
		maxScroll = 0
	}
	s.overlayScroll += delta
	if s.overlayScroll > maxScroll {
		s.overlayScroll = maxScroll
	}
	if s.overlayScroll < 0 {
		s.overlayScroll = 0
	}
}

// wheelOverlay 处理滚轮：界面在时由界面决定，否则交给剧情。
func (s *Session) wheelOverlay(delta int) bool {
	switch s.overlay {
	case overlayBacklog:
		s.scrollBacklog(delta)
		return true
	case overlayConfig:
		return true
	}
	return s.overlay != overlayNone
}

// pointerScreenMenu 处理脚本定义界面（鉴赏/立绘/选择/确认）的点击。
//
// 点中的格子/按钮把它的标签交回脚本：与原版完全一样，跳转由脚本的
// 0x10 记录决定，引擎不解释"这个标签是什么意思"。
func (s *Session) pointerScreenMenu(op uint32, x, y int) (core.Step, bool, error) {
	screen := s.screen
	if screen == nil {
		return s.step, false, nil
	}
	switch op {
	case 0:
		s.screenHover = s.hitScreenButton(x, y)
		return s.step, true, nil
	case 1, 3:
		index := s.hitScreenButton(x, y)
		if index < 0 {
			// 点击空白处 = 默认项（原版就是"取消/返回"）。
			if screen.Default != "" {
				return s.jumpScreenLabel(screen.Default)
			}
			return s.step, true, nil
		}
		s.screenHover = index
		label := s.screenLabel(index)
		if label == "" {
			return s.step, true, nil
		}
		return s.jumpScreenLabel(label)
	case 4, 5:
		if screen.Default != "" {
			return s.jumpScreenLabel(screen.Default)
		}
	}
	return s.step, true, nil
}

// jumpScreenLabel 让脚本跳到界面上的标签。
//
// 与标题按钮走同一条路径（VM.TitleSelect）：标签可能是同脚本的页面，
// 也可能是另一个脚本（view06_04_02 这类"回放片段"）。
func (s *Session) jumpScreenLabel(label string) (core.Step, bool, error) {
	// SelectLabel 是容错版本（不要求现行停点必须是 AwaitTitle）：
	// 鉴赏/确认框的点击一律走它，避免"停点不同 → 点了没反应"。
	step := s.vm.SelectLabel(label)
	s.absorb()
	s.step = s.mapStep(step)
	// 跳转后必须重新判定标题态：跳回 title3 那一页时 atTitle 要恢复为 true，
	// 否则标题层不再画、点击落到剧情分支（"回到主界面却推进了剧情"）。
	s.settleTitleState()
	return s.step, true, nil
}

// openSlotBoard 打开存/读档界面。
func (s *Session) openSlotBoard(saveMode bool) {
	if saveMode {
		s.overlay = overlaySave
	} else {
		s.overlay = overlayLoad
	}
	s.overlayPage = 0
	s.overlayHover = -1
}

// openConfig 打开设置界面。
func (s *Session) openConfig() {
	s.overlay = overlayConfig
	s.overlayPage = 0
	s.overlayHover = -1
}

// openBacklog 打开回想。
func (s *Session) openBacklog() {
	s.overlay = overlayBacklog
	maxScroll := len(s.backlog) - s.backlogLayout().maxLines
	if maxScroll < 0 {
		maxScroll = 0
	}
	s.overlayScroll = maxScroll
	s.backlogDragY = 0
}

// openMenu 打开暂停菜单（ESC / 右键 / 顶栏热区）。
func (s *Session) openMenu() {
	s.overlay = overlayMenu
	s.overlayHover = -1
}

// backToTitle 回标题：先回脚本顶部，再跑 start 的 TITLE 段。
//
// 原版的「タイトル画面」不是"重开进程"，而是让 start 脚本跳回 TITLE 标签；
// 这样标题的 BGM、底图、菜单都按原顺序重新初始化。
func (s *Session) backToTitle() (core.Step, bool, error) {
	s.overlay = overlayNone
	s.confirm = nil
	s.screen = nil
	s.atTitle = true
	s.messageVisible = false
	s.text = ""
	s.speaker = ""
	if err := s.vm.StartAt(entryStart, "TITLE"); err == nil {
		s.vm.Run(commandBudget)
		s.absorb()
	}
	s.buildTitleMenu()
	s.step = core.StepAwaitTitle
	return s.step, true, nil
}

// toggleAuto / toggleSkip 是暂停菜单里的 AUTO/SKIP 开关。
//
// 这两个状态由宿主持有（自动推进/跳过推进都在宿主侧计时），引擎只把
// "现在是不是开着"回给宿主；所以这里改的是引擎侧镜像并让宿主同步。
func (s *Session) toggleAuto() {
	s.autoActive = !s.autoActive
	if s.autoActive {
		s.requestHost("auto")
	}
}

func (s *Session) toggleSkip() {
	s.skipUnread = !s.skipUnread
	s.forceHost("skip")
}

// handleToolbarClick 处理对话框底部工具条的点击。
func (s *Session) handleToolbarClick(index int) bool {
	if s.ui == nil || index < 0 || index >= len(s.ui.Toolbar.Buttons) {
		return false
	}
	button := s.ui.Toolbar.Buttons[index]
	switch toolbarAction(button.Key) {
	case ActionQSave:
		_ = s.Save(0, "")
		s.notice = "クイックセーブしました"
	case ActionQLoad:
		if step, err := s.Load(0); err == nil {
			_ = step
		}
	case ActionBacklog:
		s.openBacklog()
	case ActionAuto:
		s.toggleAuto()
	case ActionSkip:
		s.toggleSkip()
	case ActionVoice:
		// 语音重播：重播当前这句的语音（脚本 0x30 的第三字段）。
		s.replayVoice()
	case ActionSystem:
		s.openConfig()
	case ActionClose:
		// CLEAR = 清空消息窗（原版把正文与名字一起清掉，等下一句）。
		s.messageVisible = false
		s.text = ""
		s.revealedText = ""
		return true
	}
	return true
}

// replayVoice 重播当前句的语音。
func (s *Session) replayVoice() {
	if s.voice == "" {
		return
	}
	if s.audio == nil || s.set == nil {
		return
	}
	raw := s.set.EntryOrEmpty(s.voice)
	if len(raw) == 0 {
		return
	}
	path, err := s.audio.Write(s.voice, raw)
	if err != nil {
		return
	}
	s.pending = append(s.pending, core.AudioEvent{Kind: 2, Path: path, Volume: s.voiceVolume, Channel: 2})
}

// isScreenStep 报告 VM 当前是否停在脚本定义的界面。
func (s *Session) isScreenStep() bool {
	return s.vm != nil && s.vm.ScreenState() != nil && len(s.vm.ScreenState().Items) > 0
}

// screenChoiceItems 把脚本界面的格子转成宿主的 Choice（无自绘界面的兜底）。
//
// 只有当脚本没有给格子坐标（旧世代写法）时才用到：那种情况下引擎画不出
// 原版布局，退回宿主的列表渲染至少能玩通。
func (s *Session) screenChoiceItems() []core.Choice {
	if s.screen == nil {
		return nil
	}
	out := make([]core.Choice, 0, len(s.screen.Items))
	for _, item := range s.screen.Items {
		label := item.Label
		if label == "" && len(item.Names) > 0 {
			label = item.Names[0]
		}
		if label == "" {
			continue
		}
		out = append(out, core.Choice{Name: label, Value: int32(len(out))})
	}
	return out
}

// maxInt 是界面布局用的局部辅助（stage 包没有导出）。
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var _ = vm.ScreenNone

// Wheel 处理滚轮：界面开着时滚界面（回想），否则忽略。
//
// 原版的滚轮只作用于回想/列表类界面；剧情里滚轮不推进对白（那是点击/回车
// 的事），这一点和其它品牌一致。
func (s *Session) Wheel(delta int32) (core.Step, error) {
	if s.overlay == overlayBacklog {
		if delta > 0 {
			s.scrollBacklog(1)
		} else if delta < 0 {
			s.scrollBacklog(-1)
		}
	}
	return s.step, nil
}

// PressKey 处理键盘：ESC = 暂停菜单，Enter/Space = 推进，S/L = 快存快读。
func (s *Session) PressKey(code uint32) (core.Step, error) {
	if s.overlay != overlayNone {
		switch code {
		case 27:
			if s.overlay == overlayConfirm {
				return s.applyConfirmKey(false)
			}
			s.overlay = overlayNone
			s.overlayHover = -1
			return s.step, nil
		case 13, 32:
			if s.overlay == overlayConfirm {
				return s.applyConfirmKey(true)
			}
		}
		return s.step, nil
	}
	switch code {
	case 27: // ESC
		return s.rightClick()
	case 'S':
		_ = s.Save(0, "")
		return s.step, nil
	case 'L':
		if step, err := s.Load(0); err == nil {
			return step, nil
		}
		return s.step, nil
	case 13, 32: // Enter / Space
		return s.Advance()
	}
	return s.step, nil
}

// applyConfirmKey 是键盘版的确认（Enter = YES，ESC = NO）。
func (s *Session) applyConfirmKey(yes bool) (core.Step, error) {
	step, _, err := s.applyConfirm(yes)
	return step, err
}

// absInt 取绝对值（界面步进允许为负，表示选项在图集里向左排）。
func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
