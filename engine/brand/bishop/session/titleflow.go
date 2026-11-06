package session

import (
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
	"kizuna/engine/core"
)

// 主界面（标题菜单）相关的键名，与 bsxx.cfg [title phase] 里的一致。
const (
	TitleKeySpecial     = "special"
	TitleKeyOpening     = "op"
	TitleKeyStart       = "start"
	TitleKeyLoad        = "load"
	TitleKeyMuseum      = "museum"
	TitleKeyConfig      = "config"
	TitleKeyQuit        = "quit"
	TitleKeyMuseumMusic = "museum_music"
	TitleKeyMuseumCG    = "museum_cg"
	TitleKeyMuseumScene = "museum_scene"
)

// titleButtonBase 是标题菜单按钮在 GameState 里的按钮下标偏移。
//
// GameState 的按钮表同时被地图/选择窗口复用，标题模式是独立状态，正常不会和它们
// 并存；但"从标题进入剧情后按钮表要清空"这件事必须由我们自己做（见 leaveTitle），
// 所以这里用 0 起始的下标即可，不需要额外偏移。
const titleButtonBase = 0

// enterTitle 把引擎切到主界面：铺底图、注册菜单按钮、起标题 BGM。
//
// 为什么由引擎而不是 Flutter 画标题：菜单项的位置来自 bsxx.cfg，按钮图来自游戏自己的
// BSA 包，命中判定要用图片真实尺寸（按钮图带透明边距）。这些信息只有引擎侧有，
// 让宿主重画一遍既会重复实现又容易和原版错位。
func (s *Session) enterTitle() state.SessionStep {
	if s.ui != nil {
		s.closeSystemUI()
	}
	s.menuVisible = false
	s.replayReturn = uiNone
	// 回标题会 reset 剧情状态：先把攒着的已读/CG 解锁落盘，
	// 否则刚读过的最后几十句在退出前就丢了。
	s.commitGlobal()
	title := s.config.Title
	if title.Empty() {
		// 这部作品没有配主界面：保持老行为直接进剧情，不要卡在一个空标题页上。
		return state.SessionContinue
	}
	s.clearMapStage()
	s.gameState.Reset()
	s.gameState.EndSelect()
	s.titleMode = true
	// 标题画面不显示常驻工具条（学園2 的 Q.SAVE…QUIT 只在剧情/地图上出现）。
	s.gameState.SetMenuBarVisible(false)

	if len(title.Base) > 0 {
		// 底图允许带原点偏移：ヒメドレイ 的 title_base 高 1012，
		// 原作从 y=-412 开始画，菜单只覆盖图像下半部分。其它作品的
		// BaseX/BaseY 为 0，行为与旧实现完全一致。
		artwork := state.NewLayerArtwork(title.Base, title.BaseX, title.BaseY)
		// BSXScript 1.0 的 title_base 是"标题画 + 菜单按钮"的精灵图：
		// base size 指定只露标题画那 800x600，不裁会把按钮画进屏幕外。
		if title.BaseClip[2] > 0 && title.BaseClip[3] > 0 {
			artwork.ClipX = title.BaseClip[0]
			artwork.ClipY = title.BaseClip[1]
			artwork.ClipWidth = title.BaseClip[2]
			artwork.ClipHeight = title.BaseClip[3]
		}
		s.gameState.SetLayerAt(0, title.Base, state.StageLayerScreen, []state.LayerArtwork{artwork})
	}
	s.gameState.SetMessageWindowVisible(false)
	s.gameState.SetText("")

	entries := s.titleVisibleEntries()
	for index, entry := range entries {
		s.gameState.AddButton(state.StageButton{
			Image:  entry.Image,
			X:      entry.X,
			Y:      entry.Y,
			Value:  titleButtonBase + index,
			Frames: entry.Frames,
			Layer:  -1,
			// 旧式标题按钮图的图头自带屏幕坐标（KD 的 title_menu_* 就是
			// 416,254…），配置里的 X/Y 只是同一位置的文字描述；渲染层
			// 若再叠加图头偏移，按钮会被画到 (832,508) 这类屏幕外位置。
			Frame:             entry.Frame,
			HoverOnly:         entry.HoverOnly,
			IgnoreImageOffset: true,
			Clip:              entry.Clip,
			Atlas:             entry.Atlas,
			AtlasAxis:         entry.AtlasAxis,
		})
	}
	if len(title.BgmName) > 0 {
		s.gameState.SetBgm(title.BgmName)
	}
	core.Logf("title: %d menu item(s), base='%s'", len(entries), title.Base)
	return state.SessionAwaitTitle
}

// leaveTitle 退出主界面，把标题期的舞台状态清干净再交给剧情。
//
// 必须清按钮表：剧情里的地图选择、选择窗口都会往同一张表里加按钮，
// 残留的标题按钮会被命中判定当成地图目标，玩家点菜单之外的地方就会跳剧情。
func (s *Session) leaveTitle() {
	if s.ui != nil {
		s.closeSystemUI()
	}
	s.menuVisible = false
	if !s.titleMode {
		return
	}
	s.titleMode = false
	s.gameState.EndSelect()
	s.gameState.ClearLayer(0)
	s.gameState.SetMenuBarVisible(true)
	core.Logf("title: left the title screen")
}

// 标题菜单两个"开始"入口的脚本标签。
//
// 原引擎的语义（见游戏手册）：
//   - オープニングから始める = 从序章/OP 剧情开始；
//   - 最初から始める = 跳过 OP，从本编开始。
//
// 旧实现把两者都映射到 opening，并且把 op 当成播 op.mpg 影片，导致
// "オープニングから始める"播不出片、"最初から始める"又重播一遍序章。
// 标签名在不同作品里会变，所以每个入口给一组候选，取第一个存在的。
var (
	titleOpeningCandidates = []string{"opening", "prologue", "start", "main"}
	// "最初から始める"从 OP 之后的第一个本编场景开始：本作的 prologue 是
	// 第一天早晨醒来、决定行动目标的那一段；main 已经是大学/地图循环，
	// 直接进 main 会跳过开场独白。
	titleStartCandidates = []string{"prologue", "main", "start", "newgame"}
	// 隐藏菜单（通关后的 special）。标签名在不同作品里会变，取第一个存在的。
	titleSpecialCandidates = []string{"special_mode", "special", "omake", "extra"}
)

// TitleMenu 暴露当前主界面菜单项给宿主。
func (s *Session) TitleMenu() []core.TitleMenuItem {
	// 系统界面（读档/鉴赏）盖在标题上时必须返回空：宿主会用这些矩形铺热区，
	// 不返回空的话玩家点存档画面的位置会同时命中标题菜单。
	if !s.titleMode || s.ui != nil {
		return nil
	}
	entries := s.titleVisibleEntries()
	items := make([]core.TitleMenuItem, 0, len(entries))
	buttons := s.gameState.Buttons()
	for index, entry := range entries {
		width, height := 0, 0
		// 尺寸取按钮图的真实像素：按钮图通常带透明边距，用 cfg 里的坐标当尺寸
		// 会让可点区域和视觉错位。精灵图按钮直接使用 Clip 的单帧尺寸。
		if entry.Atlas && entry.Clip[2] > 0 && entry.Clip[3] > 0 {
			width, height = entry.Clip[2], entry.Clip[3]
		} else if image := s.composer.Images.Get(entry.Image); image != nil {
			width = image.Width
			height = image.Height / max(1, entry.Frames)
		}
		value := titleButtonBase + index
		if value < len(buttons) {
			if width <= 0 {
				width = buttons[value].Width
			}
			if height <= 0 {
				height = buttons[value].Height
			}
		}
		items = append(items, core.TitleMenuItem{
			Key:     entry.Key,
			Label:   entry.Key,
			X:       int32(entry.X),
			Y:       int32(entry.Y),
			Width:   uint32(max(0, width)),
			Height:  uint32(max(0, height)),
			Enabled: true,
		})
	}
	return items
}

// SelectTitleItem 处理主界面点击。
//
// 返回值语义：只有"进入剧情"和"播片"是引擎自己的事，其余（读档/鉴赏/设置/退出）
// 是宿主 UI，引擎保持停在标题（返回 SessionAwaitTitle），由宿主根据自己传进来的
// key 决定开哪个界面。这样引擎不需要知道宿主有哪些界面。
func (s *Session) SelectTitleItem(key string) (core.Step, error) {
	if s.machine == nil {
		return core.StepFaulted, errNotOpen
	}
	if !s.titleMode {
		return s.lastStep, nil
	}
	switch key {
	case TitleKeySpecial:
		// 隐藏菜单只有在 use special mode / use conditions 都满足时才可见；
		// 宿主可能缓存了旧的标题热区，这里再判一次，避免条件不满足时误进。
		if !s.titleKeyVisible(TitleKeySpecial) {
			return s.lastStep, nil
		}
		// MDDVD 的 special 是「卑語モード」画廊，不是隐藏剧情标签。
		// 脚本里没有 special/omake 标签，按标签跳转会落回 boot/title，
		// 所以这里直接打开 higo 角色选择页。
		if s.config != nil && s.config.Mddvd {
			if s.openMddvdHigo() {
				return core.StepAwaitTitle, nil
			}
			return s.lastStep, nil
		}
		return s.startStoryFromTitlePreserve(titleSpecialCandidates, true)
	case TitleKeyOpening:
		// 「オープニングから始める」必须跑脚本的 opening 标签，不能直接在
		// 标题层播 movie/op.mpg 再回标题：
		//   - opening 标签自己会在正确的时点 CALL 影片子程序，子程序用
		//     SELECT_TAIL "op" 请求影片（见 label 9 / SELECT_TAIL 的反汇编）；
		//   - 影片播完后脚本继续 opening 剩下的演出，最后 GOTO main 进本编。
		// 之前为了"让 mpg 能被请求"在标题层短路成直接播片，结果玩家点 OP
		// 只看完片头就回到标题，永远进不了剧情；影片路径现在已由
		// collectMovies 正确索引，不需要这个绕路。
		return s.startStoryFromTitle(titleOpeningCandidates)
	case TitleKeyStart:
		// 跳过序章，直接从本编开始。
		return s.startStoryFromTitle(titleStartCandidates)
	case TitleKeyLoad:
		if !s.openDataScreen(false) {
			// 没有作品自带读档画面时交给宿主；引擎仍停在标题，
			// 读档成功后宿主会收到新的 step/text。
			s.setHostRequest("load")
		}
		return core.StepAwaitTitle, nil
	case TitleKeyMuseum:
		s.openMuseum()
		return core.StepAwaitTitle, nil
	case TitleKeyMuseumMusic, TitleKeyMuseumCG, TitleKeyMuseumScene:
		s.openTitleMuseumMode(key)
		return core.StepAwaitTitle, nil
	case TitleKeyConfig:
		// 优先打开原版的扩展设置画面（system_config_ex.layout）；作品没有这个
		// 资源时才退回宿主的设置页。这样音量/文字速度等选项和原作一致。
		if !s.openConfigScreen() {
			s.setHostRequest("settings")
		}
		return core.StepAwaitTitle, nil
	default:
		// quit 由宿主退出播放页；引擎留在标题。
		return core.StepAwaitTitle, nil
	}
}

// tapTitle 把主界面上的坐标命中成菜单项；返回空串表示点在菜单之外。
func (s *Session) tapTitle(x, y int) string {
	index := s.hitButton(x, y)
	if index < 0 {
		return ""
	}
	entries := s.titleVisibleEntries()
	if index >= len(entries) {
		return ""
	}
	return entries[index].Key
}

// startStoryFromTitle 离开标题并从指定入口进入剧情。
//
// candidates 的顺序就是优先级：不同作品的标签命名不统一（opening/main/start…），
// 取第一个实际存在的标签，避免为了一个不存在的名字把整部作品卡在标题。
func (s *Session) startStoryFromTitle(candidates []string) (core.Step, error) {
	return s.startStoryFromTitlePreserve(candidates, false)
}

// startStoryFromTitlePreserve 是 startStoryFromTitle 的完整版本。
//
// preserveVariables 用于隐藏菜单（special）：原作的隐藏路线依赖跨周目变量
// @harem，如果像"新游戏"一样把变量表清零，special_mode 进去后条件又不成立。
// 普通 START/OP 仍然清零（新游戏就该从初始状态开始）。
func (s *Session) startStoryFromTitlePreserve(candidates []string, preserveVariables bool) (core.Step, error) {
	if s.ui != nil {
		s.closeSystemUI()
	}
	var savedVariables []int32
	if preserveVariables && s.machine != nil {
		savedVariables = append([]int32(nil), s.machine.Variables()...)
	}
	// 旧式 BSScript 的 boot 已经把新周目初始属性算好；进剧情时恢复这份
	// 快照再 Seek 到 opening/prologue，而不是用 EntryState 重建全零状态。
	bootState := s.titleBootState
	s.titleBootState = nil
	s.leaveTitle()
	s.gameState.Reset()
	// 新游戏（非隐藏路线的跨周目保留）必须清空责め言葉记忆：
	// 手机临时/保存记忆是会话状态，不能把上周目的词带进新周目。
	if !preserveVariables {
		s.resetWordMemoryForNewGame()
	}
	s.clearMapStage()
	s.replay = false
	s.storyIn = false
	s.entryUsed = true // 已经明确进剧情了，不要再触发 tryStoryEntry 的兜底跳转
	s.sawMessage = false
	s.resetAudioCache()

	// 有些 Rev 作品的标题调用后面紧接序章初始化（学園/GK）：START 的语义是
	// "从 system call title 的下一条继续"，不能按标签名跳转。恢复快照后直接
	// 运行即可；这里不 Seek boot，否则会再次停在同一个 title 调用上。
	if s.config != nil && s.config.Title.ResumeAfterTitleCall && bootState != nil {
		s.machine.RestoreState(*bootState)
		s.restoreTitleVariables(savedVariables)
		s.machine.RestartBoot = !s.script.IsLegacy()
		core.Logf("title: resumed after the title system call")
		s.lastStep = core.StepContinue
		s.pollAudio()
		return s.finishRun(s.run(commandBudget)), nil
	}
	offset := -1
	entryName := ""
	for _, name := range candidates {
		if found := s.script.FindLabel(name); found >= 0 {
			offset = found
			entryName = name
			break
		}
	}
	if offset < 0 {
		core.Logf("title: no story entry label, falling back to the script boot")
		if bootState != nil {
			// 恢复 boot 快照后必须从 boot 开头重跑；直接运行会再次停在
			// title 的 system call 上，形成"点 START 又回标题"的死循环。
			// Ancient 也有 boot 标签，不能只对 Rev 生效。
			s.machine.RestoreState(*bootState)
			if boot := s.script.FindLabel("boot"); boot >= 0 {
				s.machine.SeekBs(boot)
			}
		} else {
			s.machine.RestoreState(vm.EntryState(0))
		}
		s.restoreTitleVariables(savedVariables)
		s.machine.RestartBoot = !s.script.IsLegacy()
		return s.finishRun(s.run(commandBudget)), nil
	}
	core.Logf("title: start -> '%s' @ %d", entryName, offset)
	if bootState != nil {
		s.machine.RestoreState(*bootState)
		if !s.machine.SeekBs(offset) {
			core.Logf("title: rev seek to %d failed, rebuilding entry state", offset)
			s.machine.RestoreState(vm.EntryState(offset))
		}
	} else {
		s.machine.RestoreState(vm.EntryState(offset))
	}
	s.restoreTitleVariables(savedVariables)
	s.machine.RestartBoot = !s.script.IsLegacy()
	s.lastStep = core.StepContinue
	s.pollAudio()
	return s.finishRun(s.run(commandBudget)), nil
}

// restoreTitleVariables 把标题时看到的 @ 变量写回刚进入的脚本状态。
//
// EntryState 会分配全零的变量表；隐藏菜单的 @harem 必须保留，否则
// special_mode 看到的还是"没通关"的状态。只恢复变量（@），属性（#）仍按
// 新游戏/读档语义由脚本自己初始化。
func (s *Session) restoreTitleVariables(saved []int32) {
	if len(saved) == 0 || s.machine == nil {
		return
	}
	variables := s.machine.Variables()
	copy(variables, saved)
}

// pointerTitle 处理主界面上的鼠标/触摸事件。
//
// 和地图选择不同，主界面不需要"选中态"参与渲染（菜单图自带悬停帧，由 compose 画），
// 所以 move 只更新 hover，press/tap 命中菜单项才真正切换流程。
func (s *Session) pointerTitle(op uint32, x int32, y int32) (core.Step, error) {
	switch op {
	case 0: // move
		s.gameState.SetHoveredButton(s.hitButton(int(x), int(y)))
		return core.StepAwaitTitle, nil
	case 1, 3: // press / tap
		key := s.tapTitle(int(x), int(y))
		if len(key) == 0 {
			return core.StepAwaitTitle, nil
		}
		return s.SelectTitleItem(key)
	default:
		return core.StepAwaitTitle, nil
	}
}
