package bdt

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"kizuna/engine/core"
	"kizuna/engine/core/textcodec"
)

// bdt_manin_session.go 把 maninRuntime 接到 Session 的通用生命周期上：
// 打开/重置、界面进出、标题菜单、存档旁车、消息染色与解锁记录。
//
// 具体的界面绘制和点击命中分在 bdt_manin_ui_*.go，詞語責め分在
// bdt_manin_attack.go，地图分在 bdt_manin_map.go。这样每个文件只关心一层，
// 也避免把 AFDVD 的 bdt_system_ui.go 越改越复杂。

const maninTitleBackground = "s_title1"

// maninScreenActive 判断当前是否真有 manin 模态界面盖在剧情/标题上。
func (s *Session) maninScreenActive() bool {
	return s.manin != nil && s.manin.screen != maninScreenNone
}

func (s *Session) maninSaveDir() string {
	if s.saves == nil {
		return ""
	}
	return s.saves.DirectoryPath()
}

// maninAutoSave 对应原作「选择肢/地图画面直前自动保存」：只在剧情中调用，
// 标题/界面里不写；失败只记日志，不能因为存档盘满把流程挡住。
func (s *Session) maninAutoSave() {
	if s.manin == nil || s.machine == nil || s.titleMode {
		return
	}
	if err := s.Save(0, "オートセーブ"); err != nil {
		core.Logf("bdt manin: autosave failed: %v", err)
	}
}

// buildManinRuntime 解析 atktbl 并恢复全局设置。catalog 为空也要返回结构体：
// 缺词表时言葉責め会直接判失败，但标题/鉴赏/存读档仍应可用。
func buildManinRuntime(s *Session, saveDir, name string) *maninRuntime {
	runtime := &maninRuntime{
		catalog: &maninWordCatalog{byID: map[int]int{}},
		global:  loadManinGlobal(saveDir),
	}
	if s != nil && s.resources != nil {
		location, ok := s.resources.TryGet(maninWordTableName)
		if !ok {
			location, ok = s.resources.TryGet(maninWordTableName + ".bdt")
		}
		if ok && location != nil {
			runtime.catalog = loadManinWordCatalog(location.Read())
		}
	}
	runtime.sys = runtime.global.Settings
	if s.saves != nil {
		for _, info := range s.saves.ListCount(bdtSlotCount) {
			if info.Exists {
				runtime.hasSave = true
				break
			}
		}
	}
	if title := s.maninTitleFromSystem(); title != "" {
		// 光盘解包目录名是缓存哈希；用 system.bdt 的 TL= 作为作品名，
		// 标题画面的 Info/存档摘要才不会显示一串 md5。
		s.name = title
	}
	core.Logf("bdt manin: open title='%s' words=%d name='%s' save='%s'", name, len(runtime.catalog.Words), runtime.global.Name, saveDir)
	return runtime
}

// maninTitleFromSystem 从 system.bdt 的 `TL=` 行取作品名（文件同样 0xFF
// 异或、CP932）。找不到时返回空，调用方保留目录名兜底。
func (s *Session) maninTitleFromSystem() string {
	if s.resources == nil {
		return ""
	}
	location, ok := s.resources.TryGet("system")
	if !ok {
		location, ok = s.resources.TryGet("system.bdt")
	}
	if !ok || location == nil {
		return ""
	}
	data := location.Read()
	if len(data) == 0 {
		return ""
	}
	decrypted := make([]byte, len(data))
	for i, value := range data {
		decrypted[i] = ^value
	}
	text := textcodec.DecodeCP932(decrypted)
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "TL=") {
			title := strings.TrimSpace(strings.TrimPrefix(line, "TL="))
			if index := strings.Index(title, "(C)"); index > 0 {
				title = strings.TrimSpace(title[:index])
			}
			return title
		}
	}
	return ""
}

// resetManinRun 清空「本周目」数据；全局设置/解锁/主角名不在这里重置。
func (s *Session) resetManinRun() {
	if s.manin == nil {
		return
	}
	u := s.manin
	u.slot = maninSlotData{}
	u.words = nil
	u.memory = nil
	u.caption = ""
	u.station = 0
	u.face = ""
	u.messageHidden = false
	u.screen = maninScreenNone
	u.prevScreen = maninScreenNone
	u.hover = 0
	u.attack = maninAttackState{}
	u.org = maninOrganizerState{}
	u.save = maninSaveState{}
	u.gallery = maninGalleryState{}
	u.mapState = maninMapState{}
	u.log = maninLogState{}
	u.staff = maninStaffRollState{}
	u.nameOpen = false
	u.vulgarPlayback = false
	u.logLines = nil
	u.sys = u.global.Settings
}

// maninEnterScreen 打开一个模态界面。prevScreen 只保存一层：这些界面不会
// 互相嵌套（设定里不会再开鉴赏室），一跳足够。
func (s *Session) maninEnterScreen(screen maninScreen) {
	if s.manin == nil {
		return
	}
	u := s.manin
	if u.screen == screen {
		return
	}
	u.prevScreen = u.screen
	u.screen = screen
	u.hover = 0
	s.gameState.SetAwaitingInput(false)
	s.gameState.ClearChoices()
	switch screen {
	case maninScreenSystem:
		u.sys = u.global.Settings
	case maninScreenSave:
		u.save = maninSaveState{Mode: true, Page: 0, Row: -1, Slots: s.Slots(), ReturnTo: u.prevScreen}
	case maninScreenLoad:
		u.save = maninSaveState{Mode: false, Page: 0, Row: -1, Slots: s.Slots(), ReturnTo: u.prevScreen}
	case maninScreenMuseum:
		u.gallery = maninGalleryState{Kind: maninScreenMuseum}
	case maninScreenMusic, maninScreenCG, maninScreenScene, maninScreenHch, maninScreenVulgar:
		u.gallery = maninGalleryState{Kind: screen, Thumb: -1}
	case maninScreenMap:
		u.mapState = maninMapState{ReturnTo: u.prevScreen}
	case maninScreenLog:
		u.log = maninLogState{}
	case maninScreenStaffRoll:
		u.staff = maninStaffRollState{}
	case maninScreenName:
		s.maninInitNameScreen()
	}
	s.composerInvalidate()
}

// maninExitScreen 关闭当前界面并回到 prevScreen（地图/标题/剧情）。
func (s *Session) maninExitScreen() {
	if s.manin == nil {
		return
	}
	u := s.manin
	previous := u.prevScreen
	u.screen = maninScreenNone
	u.prevScreen = maninScreenNone
	switch previous {
	case maninScreenMap:
		u.screen = maninScreenMap
	case maninScreenMuseum, maninScreenMusic, maninScreenCG,
		maninScreenScene, maninScreenHch, maninScreenVulgar:
		// 鉴赏室子页返回入口菜单；入口菜单自己返回标题/游戏。
		u.screen = maninScreenMuseum
		u.gallery = maninGalleryState{Kind: maninScreenMuseum}
	case maninScreenSystem, maninScreenSave, maninScreenLoad:
		u.screen = maninScreenNone
	}
	s.composerInvalidate()
}

func (s *Session) maninCloseAll() {
	if s.manin == nil {
		return
	}
	s.manin.screen = maninScreenNone
	s.manin.prevScreen = maninScreenNone
	s.manin.attack.Active = false
	s.manin.org.Active = false
}

// maninStep 把界面状态翻译成宿主可识别的 Step。所有模态界面都不写 pc，
// 只有明确选择/关闭后才由具体处理函数恢复脚本或关闭界面。
func (s *Session) maninStep() core.Step {
	if s.manin == nil {
		return core.StepContinue
	}
	switch s.manin.screen {
	case maninScreenAttack:
		if s.manin.attack.Phase == maninAttackExecute {
			s.gameState.SetWaitMilliseconds(650)
			return core.StepWait
		}
		return core.StepAwaitSelect
	case maninScreenMap:
		return core.StepAwaitSelect
	case maninScreenStaffRoll:
		s.gameState.SetWaitMilliseconds(1800)
		return core.StepWait
	case maninScreenLog:
		return core.StepAwaitTitle
	case maninScreenNone:
		return core.StepContinue
	default:
		return core.StepAwaitTitle
	}
}

// maninRunPending 在每次 Run/Advance 前推进「言葉責め」的逐句播放和
// 片尾滚动。返回 handled=true 时调用方应直接返回它的 Step，不再执行脚本。
func (s *Session) maninRunPending() (core.Step, bool) {
	if s.manin == nil {
		return core.StepContinue, false
	}
	switch s.manin.screen {
	case maninScreenAttack:
		if s.manin.attack.Phase == maninAttackExecute {
			return s.maninAdvanceExecution(), true
		}
	case maninScreenStaffRoll:
		return s.maninAdvanceStaffRoll(), true
	}
	return core.StepContinue, false
}

// maninAdvanceExecution 每次调用只出一句词，StepWait 让玩家看清文字；三句
// 播完再结算一轮。这样既保留原作的逐句节奏，也不用在 Go 里起计时协程。
func (s *Session) maninAdvanceExecution() core.Step {
	u := s.manin
	attack := &u.attack
	if attack.QueueIndex < len(attack.Queue) {
		id := attack.Queue[attack.QueueIndex]
		attack.QueueIndex++
		arousal, stress := u.applyReaction(id, attack.Target)
		attack.Arousal += arousal
		attack.Stress += stress
		u.consumeWord(id)
		if attack.Arousal > maninGaugeMax {
			attack.Arousal = maninGaugeMax
		}
		if attack.Stress > maninGaugeMax {
			attack.Stress = maninGaugeMax
		}
		if attack.Arousal/12 > attack.Level {
			// 升级瞬间压力为 0 才点燃烧条；这是「おねだり H」的唯一条件。
			if attack.Stress == 0 {
				attack.Burning += attack.Arousal/12 - attack.Level
				if attack.Burning > 4 {
					attack.Burning = 4
				}
			}
			attack.Level = attack.Arousal / 12
		}
		word, _ := u.catalog.word(id)
		attack.Message = word.Text
		s.gameState.SetSpeaker("")
		s.gameState.SetText(word.Text)
		s.gameState.SetMessageWindowVisible(true)
		s.stage.ShowMessage("", word.Text)
		s.stage.SetMessageColor(0xffffb0c8)
		if attack.Arousal >= maninGaugeMax {
			s.finishManinAttack(1)
			if attack.Burning >= 4 {
				s.finishManinAttack(2)
			}
			return s.runSteps(defaultRunBudget)
		}
		if attack.Stress >= maninGaugeMax {
			s.finishManinAttack(3)
			return s.runSteps(defaultRunBudget)
		}
		s.gameState.SetWaitMilliseconds(650)
		return core.StepWait
	}
	// 一轮三句结束：还有时间就回到选择，否则按原作判时间切れ。
	if attack.TurnsLeft > 0 {
		attack.TurnsLeft--
	}
	if attack.TurnsLeft <= 0 {
		s.finishManinAttack(4)
		return s.runSteps(defaultRunBudget)
	}
	if len(u.attackPool()) == 0 {
		s.finishManinAttack(5)
		return s.runSteps(defaultRunBudget)
	}
	attack.Phase = maninAttackSelect
	attack.Selected = attack.Selected[:0]
	attack.Queue = attack.Queue[:0]
	attack.QueueIndex = 0
	attack.Message = ""
	s.stage.SetMessageColor(0xffffffff)
	return core.StepAwaitSelect
}

// finishManinAttack 写回 X30 并恢复脚本。X30=1 通常 H，2 おねだり H，
// 3/4/5 分别对应压力满/时间切れ/ネタ切れ（脚本 401-02..04 有对应分支）。
func (s *Session) finishManinAttack(result int) {
	if s.manin == nil || s.machine == nil {
		return
	}
	if result < 1 || result > 5 {
		result = 5
	}
	if result == 1 || result == 2 {
		s.maninMarkVulgar(s.manin.attack.Target - 1)
	}
	s.machine.SetXValue(30, int32(result))
	s.manin.attack.Active = false
	s.manin.attack.Phase = maninAttackDone
	s.manin.screen = maninScreenNone
	s.manin.prevScreen = maninScreenNone
	s.machine.CloseManinStop()
	s.gameState.SetAwaitingInput(false)
	s.stage.SetMessageColor(0xffffffff)
	core.Logf("bdt manin: word attack result=%d arousal=%d stress=%d burning=%d", result, s.manin.attack.Arousal, s.manin.attack.Stress, s.manin.attack.Burning)
	s.composerInvalidate()
}

func (s *Session) maninAdvanceStaffRoll() core.Step {
	u := s.manin
	u.staff.Index++
	u.staff.Frame = 0
	images := []string{"e_logo", "e_cast", "e_ch0", "e_ch1", "e_ch2", "e_ch3", "e_ch4", "e_ch5", "e_staff00", "e_staff01", "e_staff02"}
	if u.staff.Index >= len(images) {
		u.screen = maninScreenNone
		u.prevScreen = maninScreenNone
		if s.machine != nil {
			s.machine.CloseManinStop()
		}
		s.composerInvalidate()
		return s.runSteps(defaultRunBudget)
	}
	s.gameState.SetWaitMilliseconds(1800)
	s.composerInvalidate()
	return core.StepWait
}

// ------------------------------------------------------------------ 标题

// maninTitleItems 的顺序/位置取自 s_title3 的 6 条命中带：
// x=400..620，y=204/246/287/329/370/412，高 32。
func (s *Session) maninTitleItems() []core.TitleMenuItem {
	if s.manin == nil {
		return nil
	}
	labels := []struct {
		key   string
		label string
	}{
		{"start", "始めから"},
		{"load", "続きから"},
		{"name", "名前変更"},
		{"config", "システム"},
		{"museum", "鑑賞室"},
		{"quit", "終了"},
	}
	// 原作 6 条命中带始终可点；没有存档时「続きから」打开空读档页，
	// 由玩家自己看到没有数据，而不是把菜单项灰掉。
	items := make([]core.TitleMenuItem, 0, len(labels)+1)
	for index, item := range labels {
		items = append(items, core.TitleMenuItem{
			Key:     item.key,
			Label:   item.label,
			X:       400,
			Y:       int32(204 + []int{0, 42, 41, 42, 41, 42}[index]),
			Width:   220,
			Height:  32,
			Enabled: true,
		})
	}
	// 原引擎在任意一位角色进入卑語モード后，在 (400,160) 画 hgon 横幅，
	// 命中后进入状态 0x80（卑語モード选择画面）。我们的 7 个解锁位与
	// 原作的 7 字节标志一一对应。
	if s.maninVulgarAny() {
		items = append(items, core.TitleMenuItem{
			Key: "vulgar", Label: "卑語モード",
			X: 400, Y: 160, Width: 220, Height: 32, Enabled: true,
		})
	}
	return items
}

func (s *Session) maninVulgarAny() bool {
	if s.manin == nil {
		return false
	}
	for _, unlocked := range s.manin.global.VulgarGirls {
		if unlocked {
			return true
		}
	}
	return false
}

// maninMarkVulgar 在某个角色的 H 场景成功后点亮她的卑語モード；六人全开时
// 顺带点亮第 7 位（その他），对应原作 7 字节标志的聚合语义。
func (s *Session) maninMarkVulgar(index int) {
	if s.manin == nil || index < 0 || index >= 6 {
		return
	}
	s.manin.global.VulgarGirls[index] = true
	all := true
	for i := 0; i < 6; i++ {
		if !s.manin.global.VulgarGirls[i] {
			all = false
			break
		}
	}
	if all {
		s.manin.global.VulgarGirls[6] = true
	}
	saveManinGlobal(s.maninSaveDir(), s.manin.global)
}

func (s *Session) maninSelectTitleItem(key string) (core.Step, error) {
	if s.machine == nil {
		return core.StepFaulted, errBdtNotOpen
	}
	switch key {
	case "start":
		// 从标题进入新周目：词库/现场状态清空；全局设置和鉴赏解锁保留。
		s.resetManinRun()
		s.leaveTitle()
		s.machine.LeaveTitle()
		return s.runSteps(defaultRunBudget), nil
	case "load":
		s.maninEnterScreen(maninScreenLoad)
		return core.StepAwaitTitle, nil
	case "name":
		s.maninEnterScreen(maninScreenName)
		return core.StepAwaitTitle, nil
	case "config":
		s.maninEnterScreen(maninScreenSystem)
		return core.StepAwaitTitle, nil
	case "museum":
		s.maninEnterScreen(maninScreenMuseum)
		return core.StepAwaitTitle, nil
	case "vulgar":
		if !s.maninVulgarAny() {
			return core.StepAwaitTitle, nil
		}
		s.maninEnterScreen(maninScreenVulgar)
		return core.StepAwaitTitle, nil
	case "quit":
		s.setHostRequest("exit")
		return core.StepAwaitTitle, nil
	}
	core.Logf("bdt manin: unknown title item '%s'", key)
	return core.StepAwaitTitle, nil
}

// ------------------------------------------------------------------ 名字

func (s *Session) setManinNamePrompt(prompt string) {
	if s.manin == nil {
		return
	}
	s.namePrompt = prompt
}

// SubmitName 接收宿主名字对话框的输入。原作姓名都是全角 5 字以内，
// 这里只做 trim 和上限保护，避免把换行/控制字符写进存档标题。
func (s *Session) SubmitName(name string) bool {
	if s.manin == nil || !s.manin.nameOpen {
		return false
	}
	clean := strings.TrimSpace(strings.ReplaceAll(name, "\n", " "))
	if clean == "" {
		return false
	}
	runes := []rune(clean)
	if len(runes) > 10 {
		runes = runes[:10]
	}
	s.manin.global.Name = string(runes)
	s.manin.nameOpen = false
	s.namePrompt = ""
	saveManinGlobal(s.maninSaveDir(), s.manin.global)
	core.Logf("bdt manin: player name set to '%s'", s.manin.global.Name)
	return true
}

// ------------------------------------------------------------------ VM host

func (s *Session) SetCaption(text string) {
	if s.manin == nil {
		return
	}
	s.manin.caption = CleanBdtText(text)
	s.composerInvalidate()
}

func (s *Session) StartManinAttack(target, timeLimit int) {
	if s.manin == nil {
		return
	}
	if target < 1 || target > 6 {
		target = 1
	}
	if timeLimit <= 0 {
		timeLimit = maninDefaultTimeLimit
	}
	turns := timeLimit / 100
	if turns < 1 {
		turns = 1
	}
	if turns > 9 {
		turns = 9
	}
	s.manin.attack = maninAttackState{
		Active:    true,
		Target:    target,
		TimeLimit: timeLimit,
		TurnsLeft: turns,
		Phase:     maninAttackSelect,
		Cursor:    0,
		Page:      0,
	}
	s.manin.screen = maninScreenAttack
	s.manin.prevScreen = maninScreenNone
	s.manin.hover = 0
	s.manin.messageHidden = false
	s.gameState.SetMessageWindowVisible(false)
	s.gameState.SetAwaitingInput(false)
	s.gameState.ClearChoices()
	s.composerInvalidate()
	core.Logf("bdt manin: word attack start target=%d time=%d turns=%d pool=%d", target, timeLimit, turns, len(s.manin.attackPool()))
}

func (s *Session) StartManinOrganizer() {
	if s.manin == nil {
		return
	}
	s.manin.org = maninOrganizerState{Active: true, Mode: 0, Page: 0, Cursor: 0, Overwrite: -1}
	s.manin.screen = maninScreenOrganize
	s.manin.prevScreen = maninScreenNone
	s.manin.hover = 0
	s.gameState.SetAwaitingInput(false)
	s.composerInvalidate()
}

func (s *Session) StartManinStaffRoll() {
	if s.manin == nil {
		return
	}
	s.manin.staff = maninStaffRollState{}
	s.manin.screen = maninScreenStaffRoll
	s.manin.prevScreen = maninScreenNone
	s.gameState.SetAwaitingInput(false)
	s.gameState.SetMessageWindowVisible(false)
	s.composerInvalidate()
}

// ------------------------------------------------------------------ 解锁/消息

// maninNoteBackground 记录看过的 CG/事件图。g/gb/ai 是整屏 CG；gc 是人物
// 立绘 cutout（用于 CA3），不进 CG 鉴赏室。
func (s *Session) maninNoteBackground(name string) {
	if s.manin == nil || name == "" {
		return
	}
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "gc") {
		return
	}
	if !strings.HasPrefix(lower, "g") && !strings.HasPrefix(lower, "ai") {
		return
	}
	changed := s.manin.unlockCG(name)
	s.manin.unlockScene(name)
	if changed {
		saveManinGlobal(s.maninSaveDir(), s.manin.global)
	}
}

func (s *Session) maninLogMessage(speaker, text string) {
	if s.manin == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	s.manin.logLines = append(s.manin.logLines, maninLogLine{Speaker: speaker, Text: text})
	if len(s.manin.logLines) > 200 {
		s.manin.logLines = append(s.manin.logLines[:0], s.manin.logLines[len(s.manin.logLines)-200:]...)
	}
}

// expandManinMessage 是 ShowMessage 的 manin 钩子：把 <nn> 换成詞語全文，
// 收集词条并把整句标成粉色。返回展开后的正文。
func (s *Session) expandManinMessage(text string) (string, bool) {
	if s.manin == nil {
		return text, false
	}
	expanded, ids := expandManinWords(text, s.manin.catalog)
	for _, id := range ids {
		s.manin.collectWord(id)
	}
	return expanded, len(ids) > 0
}

// ------------------------------------------------------------------ 存档旁车

func (s *Session) maninCaptureSlot() maninSlotData {
	if s.manin == nil {
		return maninSlotData{}
	}
	return maninSlotData{
		Words:    append([]int{}, s.manin.words...),
		Memory:   append([]int{}, s.manin.memory...),
		Caption:  s.manin.caption,
		Station:  s.manin.station,
		Day:      s.manin.slot.Day,
		TimeSlot: s.manin.slot.TimeSlot,
	}
}

func (s *Session) maninRestoreSlot(slot int) {
	if s.manin == nil {
		return
	}
	fallback := maninSlotData{Words: append([]int{}, s.manin.words...), Memory: append([]int{}, s.manin.memory...)}
	data := loadManinSlotData(s.maninSaveDir(), slot, fallback)
	s.manin.slot = data
	s.manin.words = sanitizeManinWordIDs(data.Words, s.manin.catalog)
	s.manin.memory = sanitizeManinWordIDs(data.Memory, s.manin.catalog)
	s.manin.caption = data.Caption
	s.manin.station = data.Station
	s.manin.screen = maninScreenNone
	s.manin.prevScreen = maninScreenNone
	s.manin.attack = maninAttackState{}
	s.manin.org = maninOrganizerState{}
	s.manin.sys = s.manin.global.Settings
	s.maninApplySettings()
}

func sanitizeManinWordIDs(ids []int, catalog *maninWordCatalog) []int {
	out := make([]int, 0, len(ids))
	seen := map[int]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		if _, ok := catalog.word(id); !ok {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// ------------------------------------------------------------------ 存档标题

func (s *Session) maninSummary() string {
	if s.manin == nil {
		return s.summarize()
	}
	parts := []string{}
	if strings.TrimSpace(s.manin.caption) != "" {
		parts = append(parts, s.manin.caption)
	}
	if s.manin.station > 0 && s.manin.station <= len(maninStationNames) {
		parts = append(parts, maninStationNames[s.manin.station-1])
	}
	text := strings.Join(parts, " ")
	if text == "" {
		text = s.summarize()
	}
	runes := []rune(text)
	if len(runes) > 42 {
		return string(runes[:42])
	}
	return text
}

var maninStationNames = []string{
	"鎌多", "新重", "樟南", "聖城", "桜木町", "新横山", "一ヶ谷", "町屋",
}

// maninSlotStamp 给存读档行提供时间文本；BDT 已有 ticks 转换，复用即可。
func maninSlotStamp(slot core.Slot) string {
	if slot.SavedAt == "" {
		return ""
	}
	if parsed, err := time.Parse(time.RFC3339, slot.SavedAt); err == nil {
		return parsed.Local().Format("06/01/02 15:04")
	}
	return ""
}

// maninSaveExists 给标题的「続きから」和读档页使用。
func (s *Session) maninSaveExists() bool {
	for _, slot := range s.Slots() {
		if slot.Exists {
			return true
		}
	}
	return false
}

// maninEnsureDirs 只是让首次存档前把目录建好，失败不阻塞流程。
func maninEnsureDirs(saveDir string) {
	if saveDir == "" {
		return
	}
	if err := os.MkdirAll(filepath.Join(saveDir, "manin"), 0o755); err != nil {
		core.Logf("bdt manin: mkdir failed: %v", err)
	}
}
