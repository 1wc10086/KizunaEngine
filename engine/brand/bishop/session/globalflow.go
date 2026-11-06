package session

import (
	"path/filepath"
	"strings"

	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/brand/bishop/save"
	"kizuna/engine/core"
)

// globalflow.go 负责"全局存档"（save/common.dat）的读写，以及由它驱动的鉴赏解锁。
//
// 分工：save.GlobalSave 只做字节编解码（布局、边界检查、原子落盘），
// 这里负责"什么时候读、什么时候写、哪张 CG 该解锁"这些业务判断。
// 分开的原因是编码逻辑要被原引擎的文件格式约束，而业务判断要被剧情流程约束，
// 两者的变化原因完全不同。

// globalSavePath 是 common.dat 的实际路径，和槽位存档共用同一个目录。
func (s *Session) globalSavePath() string {
	return filepath.Join(dataSaveDir(s.dataDir), "common.dat")
}

// globalCommitEvery 是"多少句对白落一次盘"。
//
// 取值权衡：40KB 的文件在手机上写一次约几 ms，每句都写会让长对话明显卡顿；
// 32 句大约是一两分钟的游戏时长，万一进程被杀，最多丢这一小段的"已读"标记。
// CG 与场景解锁不受影响——它们会立刻触发一次提交（见 markCgUnlocked）。
const globalCommitEvery = 32

// loadGlobalSave 在打开游戏时读一次全局存档。
//
// 读失败（文件损坏/格式不认识）时只记日志并降级成"空档"：鉴赏界面显示成全部未解锁，
// 但游戏本身照常能玩。为了一份损坏的 museum 数据让整个游戏打不开是本末倒置。
func (s *Session) loadGlobalSave() {
	want := s.globalExpectations()
	if s.unlockAll {
		// 目录是从资源名推导的，没有和原引擎一致的名字表索引；
		// 读到的解锁位无法安全地映射到我们的图上，干脆不读、按全解锁展示。
		s.globalDone = true
		return
	}
	if want.MessageCount <= 0 {
		// 脚本还没读出来，无法校验表长；等第一次真正需要时再加载。
		return
	}
	path := s.globalSavePath()
	stored, err := save.LoadGlobalSave(path, want)
	if err != nil {
		core.Logf("global save: '%s' is unreadable (%v), starting from an empty one", path, err)
		stored = nil
	}
	if stored == nil {
		if s.ancientLayout {
			stored = save.NewAncientGlobalSave(want)
		} else {
			stored = save.NewGlobalSave(want)
		}
		core.Logf("global save: no '%s', starting from an empty one", path)
	} else {
		// 游戏更新后资源会变长，把表补齐能保住旧解锁状态。
		stored.EnsureSize(want)
		core.Logf("global save: loaded '%s' cg=%d scene=%d messages=%d", path, len(stored.CgUnlocked), len(stored.SceneUnlocked), len(stored.MessageRead))
	}
	s.global = stored
	s.globalDone = true
}

// ensureGlobal 保证 s.global 可用（惰性加载）。
func (s *Session) ensureGlobal() *save.GlobalSave {
	if !s.globalDone {
		s.loadGlobalSave()
	}
	return s.global
}

// flushGlobal 把全局存档落盘。
//
// 只在真的发生了解锁变化时调用（见 markCgUnlocked 等），因为一次落盘要写整个文件；
// 每句对白都写一次会把闪存写穿，也没必要——原引擎也是攒着写的。
func (s *Session) flushGlobal() {
	if s.global == nil {
		return
	}
	path := s.globalSavePath()
	if err := s.global.Save(path); err != nil {
		core.Logf("global save: write '%s' failed: %v", path, err)
		return
	}
	core.Logf("global save: wrote '%s'", path)
}

// globalExpectations 用游戏资源算出三张表应有的长度。
//
// 这三个数字是"这份 common.dat 是不是本作的"的判据：对不上就说明文件来自别的作品
// 或已被损坏，此时不能拿它去覆盖玩家的数据。
func (s *Session) globalExpectations() save.GlobalExpectations {
	want := save.GlobalExpectations{}
	if s.script != nil {
		want.MessageCount = s.script.MessageCount()
	}
	if cg := s.ensureCg(); cg != nil {
		want.CgCount = len(cg.Names)
	}
	if scenes := s.ensureScenes(); scenes != nil {
		want.SceneCount = len(scenes.Scenes)
	}
	return want
}

// MarkCgUnlocked 由剧情在"出现一张 CG"时调用，把这张 CG 记成已鉴赏。
//
// 参数是图片名（例如 ev01_001_1p00）。cg.dat 的名字表就是这些图片名，
// 所以能直接反查下标；查不到说明这张图不在鉴赏目录里（比如背景、立绘），
// 属于正常情况，直接忽略。
func (s *Session) markCgUnlocked(image string) {
	if s.unlockAll {
		return
	}
	global := s.ensureGlobal()
	if global == nil || len(image) == 0 {
		return
	}
	catalog := s.ensureCg()
	if catalog == nil {
		return
	}
	index := catalog.IndexOf(image)
	if index < 0 {
		return
	}
	if global.UnlockCg(index) {
		s.globalDirty = true
		core.Logf("global save: cg unlocked '%s' (#%d)", image, index)
	}
}

// markSceneUnlocked 由剧情在"进入一个场景"时调用。
func (s *Session) markSceneUnlocked(label string) {
	if s.unlockAll {
		return
	}
	global := s.ensureGlobal()
	if global == nil || len(label) == 0 {
		return
	}
	catalog := s.ensureScenes()
	if catalog == nil {
		return
	}
	index := catalog.IndexOfLabel(label)
	if index < 0 {
		return
	}
	if global.UnlockScene(index) {
		s.globalDirty = true
		core.Logf("global save: scene unlocked '%s' (#%d)", label, index)
	}
}

// messageRead 查询一句对白是否已经读过。不会触发全局存档落盘；
// 没有全局存档时按"未读"处理（与原版第一次启动一致）。
func (s *Session) messageRead(messageID int) bool {
	global := s.ensureGlobal()
	return global != nil && messageID >= 0 && global.MessageReadAt(messageID)
}

// markMessageRead 记录"这句对白读过了"。
//
// 只在 step 进入 AwaitInput（玩家真的看到这句）时调用，而不是在脚本解码出文本时：
// 后者会把跳过/自动播放中途略过的句子也算成读过。
func (s *Session) markMessageRead(messageID int) {
	global := s.ensureGlobal()
	if global == nil || messageID < 0 {
		return
	}
	if global.MarkMessageRead(messageID) {
		s.globalDirty = true
	}
}

// commitGlobal 在合适的时机把攒下的解锁写盘。
//
// 只有真正有变化时才写：这份文件是整份重写（40KB），每句对白都写会把闪存写穿。
func (s *Session) commitGlobal() {
	if !s.globalDirty || s.global == nil {
		return
	}
	s.globalDirty = false
	s.flushGlobal()
}

// ensureGlobalSave 保证磁盘上存在一份 common.dat。
//
// 和 commitGlobal 的分工：commitGlobal 只在"解锁状态变了"时写，覆盖"玩过之后的增量"；
// 这里负责"文件本来就不存在"的情况——全新安装、或者玩家删过存档目录。
// 原引擎的行为是打开游戏就有一份总存档；没有它的话鉴赏界面读不到任何状态，
// 而且玩家会以为存档功能坏了。
//
// 只在文件确实缺失时落盘，已有文件不会因为一次打开被无谓改写（保留其时间戳，
// 也避免把原引擎写的摘要头覆盖成空值）。
func (s *Session) ensureGlobalSave() {
	global := s.ensureGlobal()
	if global == nil {
		return
	}
	path := s.globalSavePath()
	if core.PathExists(path) {
		return
	}
	s.flushGlobal()
}

// SceneUnlocked 告诉宿主某个场景是否已解锁。
func (s *Session) SceneUnlocked(index int) bool {
	return s.ensureGlobal().SceneUnlockedAt(index)
}

// scanStageUnlocks 扫描当前舞台上出现的图片，把其中属于鉴赏目录的记成已解锁。
//
// 为什么在"停在对白上"这个时刻扫，而不是在绘制指令执行时逐个上报：
//   - 绘制指令（image / character / change）有很多变体，逐个挂钩容易漏；
//   - 一张 CG 常常由多条指令叠出来（底图 + 立绘 + 差分），
//     只有等这一屏稳定下来再扫，才看得到最终组合；
//   - 这个时刻每句对白只有一次，开销可以接受（图层数量是个位数）。
func (s *Session) scanStageUnlocks() {
	catalog := s.ensureCg()
	if catalog == nil || s.gameState == nil {
		return
	}
	for _, layer := range s.gameState.Layers() {
		if layer == nil {
			continue
		}
		for _, artwork := range layer.Artworks() {
			if len(artwork.Image) == 0 {
				continue
			}
			if catalog.IndexOf(artwork.Image) >= 0 {
				s.markCgUnlocked(artwork.Image)
				continue
			}
			// 差分图常带后缀（_a / _b / :s），名字表里记的是基名。
			// 这里只在后缀是单字符时尝试去尾，避免把完全不同的图误判成同一张。
			if base := cgBaseName(artwork.Image); base != artwork.Image && catalog.IndexOf(base) >= 0 {
				s.markCgUnlocked(base)
			}
		}
	}
}

// cgBaseName 去掉差分后缀（最后一个 '_' 或 ':' 之后的单字符尾巴）。
func cgBaseName(name string) string {
	if len(name) < 2 {
		return name
	}
	last := name[len(name)-1]
	if last < 'a' || last > 'z' {
		return name
	}
	separator := name[len(name)-2]
	if separator != '_' && separator != ':' {
		return name
	}
	return name[:len(name)-2]
}

// markSceneUnlockedAt 在脚本跳到某个标签时判定"这是不是一个可回放的场景"。
//
// 参数是标签在代码里的偏移。场景表里存的是标签名，所以要先按偏移反查名字。
func (s *Session) markSceneUnlockedAt(offset int) {
	catalog := s.ensureScenes()
	if catalog == nil || offset < 0 || s.unlockAll {
		return
	}
	if s.sceneOffsets == nil {
		// 建立"代码偏移 -> 场景下标"的反查表：场景标签在脚本里是固定的，
		// 每次跳转都做一次 FindLabel 会把字符串比较摊到热路径上。
		s.sceneOffsets = make(map[int]int, len(catalog.Scenes))
		for index := range catalog.Scenes {
			// 剧情实际跳的是场景标签（L01_001），不是解锁列 label_01_001。
			// 回放包装（_replay）也映射到同一个场景，二者都算"进入过"。
			for _, label := range catalog.Scenes[index].LookupNames() {
				if found := s.script.FindLabel(label); found >= 0 {
					s.sceneOffsets[found] = index
				}
			}
		}
	}
	index, ok := s.sceneOffsets[offset]
	if !ok {
		return
	}
	// CHART 的"进行中"标记按场景标签找格子。scene.dat 的 SceneId 可能是
	// L01_002_replay，图里的 label 是 L01_002，所以统一去掉回放后缀。
	scene := catalog.Scenes[index]
	s.currentSceneLabel = strings.TrimSuffix(scene.SceneId, replaySuffix)
	if s.currentSceneLabel == "" {
		s.currentSceneLabel = scene.Label
	}
	global := s.ensureGlobal()
	if global == nil {
		return
	}
	if global.UnlockScene(index) {
		s.globalDirty = true
		core.Logf("global save: scene unlocked '%s' (#%d)", catalog.Scenes[index].Label, index)
	}
}

// registerSceneEntry 处理 SELECT_ENTRY(arg>=0)：剧情走到某个可回放场景的
// 结尾时，脚本把该场景的回放标签号交出来，原引擎据此把它记进鉴赏。
//
// 这比"进入标签时按偏移解锁"更准确：有些场景是由地图/共通流程 CALL 的，
// 玩家不一定从场景标签本身进入；而 SELECT_ENTRY 是剧本明确声明的
// "这一段就是鉴赏里的第 N 个场景"。
func (s *Session) registerSceneEntry(labelID int) {
	if s.script == nil || s.unlockAll {
		return
	}
	name := s.script.GetLabelName(labelID)
	if name == "" {
		return
	}
	// CHART 的格子标签不带 _replay 后缀；这里也统一成普通场景标签。
	s.currentSceneLabel = strings.TrimSuffix(name, replaySuffix)
	catalog := s.ensureScenes()
	if catalog == nil {
		return
	}
	index := catalog.IndexOfLabel(name)
	if index < 0 {
		return
	}
	global := s.ensureGlobal()
	if global == nil {
		return
	}
	if global.UnlockScene(index) {
		s.globalDirty = true
		core.Logf("global save: scene registered '%s' (#%d)", name, index)
	}
}

// CgUnlockedByName 按图片名查询鉴赏解锁状态。
func (s *Session) CgUnlockedByName(name string) bool {
	if s.unlockAll {
		return true
	}
	global := s.ensureGlobal()
	if global == nil || len(name) == 0 {
		return false
	}
	catalog := s.ensureCg()
	if catalog == nil {
		return false
	}
	return global.CgUnlockedAt(catalog.IndexOf(name))
}

// informationWindowValues 读出信息窗要显示的日期与时段。
//
// 读哪两个变量由 bsxx.cfg 的 [information window] 决定（`day = #day`、
// `time = #time`），不是写死的属性名——不同作品的变量名不一样。
//
// 返回 -1 表示"不显示"：标题画面、回想场景、以及还没开始计时的开场都不该
// 挂一个"行動開始から 0日目"的牌子，那在原引擎里也是没有的。
func (s *Session) informationWindowValues() rendering.InformationWindowValues {
	values := rendering.InformationWindowValues{Day: -1, Time: -1}
	if s.config == nil || s.machine == nil || s.gameState == nil {
		return values
	}
	layout := s.config.Information
	if layout.Empty() {
		return values
	}
	// 主界面、影片、系统界面和回想期间不显示：这些状态下剧情没有在推进，
	// 显示日期会让玩家误以为进度在走（原引擎在这些画面也不挂信息窗）。
	if s.titleMode || s.ui != nil || s.menuVisible || s.replay || s.wordActive {
		// 言葉責め进行中右上角是兴奋度仪表，再画日期窗会叠在一起。
		return values
	}
	// CS 的 [follow window view] mesg = game：这块小窗跟随对话框。
	// 地图/选择态会调 BeginSelect 把消息框标记成隐藏，此时原版不画它；
	// 只对写了该配置的作品生效，其它作品维持常驻。
	//
	// 用 messageWindowVisible() 而不是只看 MessageWindowVisible 标记：
	// 后者在选择态由 BeginSelect 清零，但在换景/正文为空时仍可能为真，
	// 只看它会让日期窗在字幕空档里闪一下。
	if layout.FollowsMessageWindow && !s.messageWindowVisible() {
		return values
	}
	// 引擎地图铺满整屏且自带周数/时段卡（UK/UKIE 的 [map phase]），
	// 再画对话框左下角的这张卡会和 map_time 叠在一起。原引擎的地图画面
	// 没有这张信息窗，只有消息窗可见时才挂它。其它有引擎地图的作品
	// 没有 [map phase]，保持原来的可见性，避免改变它们的行为。
	if s.mapShown && !s.config.MapPhase.Empty() {
		return values
	}
	if len(layout.DayVariable) > 0 {
		// 多数作品用 0 表示"还没开始"（init_variable 里 `##day = -1`、
		// opening 里置 0）；KD 的 0 是 オープニング，需要显式允许。
		if day := s.readProperty(layout.DayVariable); day > 0 || (layout.DayZero && day == 0) {
			values.Day = day
			// 诊断：只在日期真正变化时记一行，下一次日志就能定位是剧本自己
			// 跳日（时间跳跃）还是引擎重复执行了日循环。正常一天 +1。
			if day != s.loggedDay {
				core.Logf("hud: day %d -> %d at pc=%d", s.loggedDay, day, s.machine.InstructionStart())
				s.loggedDay = day
			}
		}
	}
	if len(layout.TimeVariable) > 0 {
		if clock := s.readProperty(layout.TimeVariable); clock > 0 || (layout.TimeZero && clock == 0) {
			values.Time = clock
		}
	}
	// UK/UKIE 脚本自己维护 #week_no / #week_day，直接采用它们，比从 #day 换算可靠；
	// 其它作品没有这两个属性，readProperty 返回 -1，于是退回按 #day 换算。
	// 第 1 天（#day==1）脚本跳过 eyecatch，两个属性还没写、仍是 0；这种
	// "两值都为 0" 只可能是初始化状态，交给渲染器按 day/7、day%7 换算，
	// 否则第一天会错误地显示日曜日。
	if len(layout.WeekStrip.Image) > 0 {
		if week := s.readProperty("#week_no"); week >= 0 {
			if weekday := s.readProperty("#week_day"); weekday >= 0 && (week != 0 || weekday != 0) {
				values.Week = week
				values.WeekDay = weekday
				values.WeekKnown = true
			}
		}
	}
	return values
}

// messageSubWindowValues 读出对话框左侧日期小窗要显示的 #day / #time。
//
// 学園2 的 #day 从 0（序章日）开始，0 时只显示时刻、不显示周数；#time=0
// 是「朝」，所以 TimeZero 允许 0。标题/系统界面/回放期间返回 -1，避免
// 日期窗在标题或存档画面上露出一角。
func (s *Session) messageSubWindowValues() (int, int) {
	if s.config == nil || s.machine == nil || s.gameState == nil {
		return -1, -1
	}
	sub := s.config.Message.SubWindow
	if !sub.Enabled {
		return -1, -1
	}
	if s.titleMode || s.ui != nil || s.menuVisible || s.replay {
		return -1, -1
	}
	day, clock := -1, -1
	if len(sub.DayVariable) > 0 {
		day = s.readProperty(sub.DayVariable)
	}
	if len(sub.TimeVariable) > 0 {
		clock = s.readProperty(sub.TimeVariable)
		if clock == 0 && !sub.TimeZero {
			clock = -1
		}
	}
	return day, clock
}

// readProperty 按属性名读脚本变量；名字不存在时返回 -1。
func (s *Session) readProperty(name string) int {
	if s.script == nil || s.machine == nil {
		return -1
	}
	index := s.script.GetPropertyIndex(name)
	if index < 0 {
		return -1
	}
	return int(s.machine.Property(index))
}
