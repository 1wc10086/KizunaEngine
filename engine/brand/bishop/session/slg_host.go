package session

import (
	"math/rand"
	"strconv"
	"strings"
	"time"

	"kizuna/engine/brand/bishop/data/scriptdb"
	"kizuna/engine/brand/bishop/runtime"
	"kizuna/engine/brand/bishop/slg"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
	"kizuna/engine/core"
)

// slg_host.go 把 SLG 数据层接到剧本运行时上。
//
// BSXScript 3.3 的 SELECT / SELECT_TAIL / TRANSLATE 参数不只是分支标签：
// 特別授業3SLG 把宿主命令写成字符串放在 frame[1]，例如
// `$#ID_BP[0,'BT値'] = 0`、`実績解除 ID17`、`state training`、
// `InitBeginTimeParameters()`。原引擎把这些当 Lua/宿主调用执行，旧实现
// 只当地图/影片名，所以数值、成就、训练流程全部停在原地。
//
// 这里只做"桥"：解析与数据语义在 brand/bishop/slg，变量读写走 vm，
// 界面/影片/标题走 Session。任何不认识的字符串都返回 handled=false，
// 让路由继续按选择肢/地图处理，普通 AVG 的行为保持不变。

// slgHost 实现 slg.Host。
type slgHost struct {
	session *Session
}

func (h *slgHost) DB() *scriptdb.DB { return h.session.ensureScriptDB() }

func (h *slgHost) Property(name string) int32 {
	s := h.session
	if s.script == nil || s.machine == nil {
		return 0
	}
	slot := s.script.GetPropertyIndex(name)
	if slot < 0 {
		return 0
	}
	return s.machine.Property(slot)
}

func (h *slgHost) SetProperty(name string, value int32) {
	s := h.session
	if s.script == nil || s.machine == nil {
		return
	}
	slot := s.script.GetPropertyIndex(name)
	if slot < 0 {
		return
	}
	s.machine.SetProperty(slot, value)
}

func (h *slgHost) PrivateData() map[string]any {
	if h.session.privateData == nil {
		h.session.privateData = make(map[string]any)
	}
	return h.session.privateData
}

func (h *slgHost) CommonData() map[string]any {
	if h.session.commonData == nil {
		h.session.commonData = make(map[string]any)
	}
	return h.session.commonData
}

func (h *slgHost) UnlockAchievement(ids ...string) {
	for _, id := range ids {
		if h.session.unlockAchievement(id) {
			h.session.setToolNotice("実績解除 " + id)
		}
	}
}

// EnterState 记录当前 SLG 子界面。
//
// Lua 界面本身由 engine/brand/bishop/lua 负责绘制与输入；这里不阻塞脚本：
// 状态名只是"下一帧该由哪个 Lua 对象接管画面"的标记。脚本在 SET_STATE
// 之后会继续跑到它自己的等待点，与 Lua 的 initialize/draw 并行。
func (h *slgHost) EnterState(name string) {
	s := h.session
	name = strings.TrimSpace(name)
	if s.activateSLGState(name) {
		return
	}
	// 没有 Lua 运行时（旧作品/embed 缺失）时不要设置 slgState：
	// 那会让 dispatchHostAction 以为界面已接管而暂停脚本，变成死锁。
	// 保持旧行为，继续执行剧本。
	core.Logf("slg: enter state '%s' (no lua runtime), continuing", name)
}

func (h *slgHost) CustomMap() int32 {
	// 自定义地图：剧本已经用 IMAGE/IMAGE_EX type=160 画好底图与按钮，
	// 引擎只需要进入选择态，等宿主点击后把 value/序号写回
	// [custom map phase] 的 #select_value / #custom_map_type。
	h.session.beginCustomMap()
	return 0
}

// AutoSave 写原版的自动存档槽。
//
// 命令出现在 map/choice 标签的开头，此时 VM 的 PC 已经越过 SELECT_TAIL；
// 读档会从下一条指令继续，重新执行 map_main/选择肢，所以存档可以完整恢复。
func (h *slgHost) AutoSave() {
	h.session.autoSave()
}

func (h *slgHost) Movie(name string) bool {
	s := h.session
	if s.gameState == nil {
		return false
	}
	if _, ok := s.gameState.MoviePath(name); !ok {
		return false
	}
	s.gameState.SetMovie(name)
	s.gameState.SetMovieSerial(s.gameState.MovieSerial() + 1)
	return true
}

func (h *slgHost) Title() bool {
	h.session.enterTitle()
	return true
}

func (h *slgHost) SetGalleryRate(kind string) int32 {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "cg", "strip":
		return int32(h.session.cgUnlockPercent())
	case "scene":
		return int32(h.session.sceneUnlockPercent())
	}
	return 0
}

// WaitAnimating 处理 `is animeted #result , 1`。
//
// 这条命令的原义是"等 ActionType 动画播完再继续"；我们暂时没有对应的
// 动画播放器，但不能立即返回 1——脚本会以为动画已经结束，在淡入/翻滚
// 中途继续剧情。给一个 350ms 的时间门，跨帧返回 0，超时后把变量置为
// 目标值并返回 1，既保留演出节奏也不会死循环。
func (h *slgHost) WaitAnimating(variable string, value int32) int32 {
	s := h.session
	name := strings.TrimSpace(variable)
	if name == "" {
		return 1
	}
	if h.Property(name) == value {
		s.animWaitUntil = time.Time{}
		return 1
	}
	now := time.Now()
	if s.animWaitUntil.IsZero() {
		s.animWaitUntil = now.Add(350 * time.Millisecond)
		return 0
	}
	if now.Before(s.animWaitUntil) {
		return 0
	}
	s.animWaitUntil = time.Time{}
	h.SetProperty(name, value)
	return 1
}

func (h *slgHost) ActionType(name string) int32 {
	return 0
}

// GetObjectTest 实现 territory.lua 的随机物件抽取。
//
// 候选表来自 define.bsa:ter_objs（只读 CSV），按 LL/L/M/S 的稀有度先缩小
// 范围再随机；这个结果会写进 #item，随后领地界面用它摆家具。数据缺失时
// 返回 -1，和原实现的"没有候选"一致。
func (h *slgHost) GetObjectTest(variable string) int32 {
	s := h.session
	content, ok := s.resourceReader()("ter_objs")
	if !ok {
		return -1
	}
	rows := scriptdb.ParseCSV(content)
	type item struct {
		size    string
		name    string
		referNo int32
	}
	var all []item
	for index, row := range rows {
		if index == 0 || len(row) < 17 {
			continue
		}
		refer, err := strconv.ParseInt(strings.TrimSpace(row[16]), 10, 32)
		if err != nil {
			continue
		}
		all = append(all, item{
			size:    strings.TrimSpace(row[2]),
			name:    strings.TrimSpace(row[15]),
			referNo: int32(refer),
		})
	}
	if len(all) == 0 {
		return -1
	}
	random := rand.Intn(256) + 1
	pick := func(want func(item) bool) []int32 {
		var out []int32
		for _, candidate := range all {
			if want(candidate) && h.ownedObjects(candidate.name) < 9 {
				out = append(out, candidate.referNo)
			}
		}
		return out
	}
	candidates := pick(func(candidate item) bool { return candidate.name == "等身大フィギュア" && random == 1 })
	if len(candidates) == 0 && random < 12 {
		candidates = pick(func(candidate item) bool { return candidate.size == "LL" })
	}
	if len(candidates) == 0 && random < 32 {
		candidates = pick(func(candidate item) bool { return candidate.size == "L" })
	}
	if len(candidates) == 0 && random < 64 {
		candidates = pick(func(candidate item) bool {
			return candidate.size == "M" && candidate.name != "等身大フィギュア"
		})
	}
	if len(candidates) == 0 {
		candidates = pick(func(candidate item) bool { return candidate.size == "S" })
	}
	if len(candidates) == 0 {
		return -1
	}
	return candidates[rand.Intn(len(candidates))]
}

func (h *slgHost) ownedObjects(name string) int32 {
	db := h.session.ensureScriptDB()
	if db == nil {
		return 0
	}
	// var_hasobjs.csv 只有一列有效数据（列名"共通"），行名就是物件名。
	return db.GetNamed("@ID_HASOBJS", name, "共通")
}

func (h *slgHost) CallLabel(label string) bool {
	s := h.session
	if s.machine == nil || label == "" {
		return false
	}
	if !s.machine.SeekLabel(label) {
		core.Logf("slg: label '%s' not found", label)
		return false
	}
	core.Logf("slg: jumped to label '%s'", label)
	return true
}

// installSLGHost 把回调挂到路由上；在 Open 构造 router 之后调用一次。
func (s *Session) installSLGHost() {
	if s.router == nil {
		return
	}
	host := &slgHost{session: s}
	// 只有现代 BSXScript 路由支持字符串参数；BSScript（Rev/Ancient）
	// 的 SELECT 参数是标签/数值，没有这种命令。
	modern, ok := s.router.(*runtime.ScriptRouter)
	if !ok {
		return
	}
	modern.SetHostAction(func(command vm.ScriptCommandId) (state.SessionStep, bool) {
		return s.dispatchHostAction(host, command)
	})
}

// runLuaTranslate 执行 TRANSLATE / SELECT 里的 Lua 调用。
//
// 有 VM 时优先执行脚本里的原函数，保证 UI 与剧情共用同一份 PrivateData；
// 没有 VM（或函数未定义）时退回 Go 的等价实现。
func (s *Session) runLuaTranslate(host *slgHost, name string) state.SessionStep {
	if s.slgVM != nil && s.slgVM.HasFunction(name) {
		s.slgVM.Call(name)
		if label := s.slgVM.PendingLabel(); label != "" {
			if !s.machine.SeekLabel(label) {
				core.Logf("slg: lua label '%s' is missing", label)
			}
		}
		return state.SessionContinue
	}
	slg.Translate(host, name)
	return state.SessionContinue
}

// isCustomMapStatement 判断 SELECT_TAIL 的参数是否要求"进入自定义地图选择态"。
func isCustomMapStatement(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "custom map")
}

func (s *Session) dispatchHostAction(host *slgHost, command vm.ScriptCommandId) (state.SessionStep, bool) {
	if s.machine == nil || s.script == nil {
		return state.SessionContinue, false
	}
	// IMAGE type=13 是 BSXScript 1.0 的片尾影片请求：name 是 "staffroll"，
	// 素材在 movie/staffroll.mpg。beginStaffroll 会优先找字幕图，没有就
	// 交给宿主播片并返回 AwaitMovie；不能落到 StageDirector 当成普通图层。
	if (command == vm.Image || command == vm.ImageExtended) && s.machine.Frame(2) == 13 &&
		strings.EqualFold(s.script.GetName(int(s.machine.Frame(1))), "staffroll") {
		if step := s.beginStaffroll(); step != state.SessionContinue {
			return step, true
		}
	}
	// IMAGE_EX type=176 是"自定义窗口"：名字指向 define.bsa 里的 .txt，
	// 由引擎画角色面板/单选按钮。只有定义文件真的存在时才拦截；否则
	// 说明这部作品把 176 当普通图层用，继续走 StageDirector 的贴图路径。
	if command == vm.ImageExtended && s.machine.Frame(2) == 176 {
		name := s.script.GetName(int(s.machine.Frame(1)))
		if s.customDefinition(name) != nil {
			s.showCustomUI(name, int(s.machine.Frame(3)), int(s.machine.Frame(4)))
			return state.SessionContinue, true
		}
	}
	// 换背景（IMAGE/IMAGE_EX type=1/2）说明已经离开上一屏：清掉工作面板，
	// 否则新场景上会残留上一屏的单选按钮。真实背景仍交给 StageDirector 画。
	if command == vm.Image || command == vm.ImageExtended {
		imageType := s.machine.Frame(2)
		if imageType == 1 || imageType == 2 {
			s.clearCustomUI()
		}
	}
	name := s.script.GetName(int(s.machine.Frame(1)))
	if name == "" {
		return state.SessionContinue, false
	}
	switch command {
	case vm.Translate:
		return s.runLuaTranslate(host, name), true
	}
	// 満淫電車2 的「言葉責めシステムNEO」。这些名字只在 SELECT_TAIL 里
	// 出现，剧本用它们把数值/界面交给引擎；必须先于 SLG 命令判断。
	if command == vm.SelectTail {
		switch name {
		case "言葉責め開始":
			s.beginWordAttack()
			return state.SessionContinue, true
		case "言葉責め":
			// openWordTurn 返回 true 也可能是"没有词可用"：这时它写
			// #result=2 让脚本走 lackofmaterial，不能停在一张空手机上等点击。
			if s.openWordTurn() && s.wordUI != nil {
				return state.SessionAwaitInput, true
			}
			return state.SessionContinue, true
		case "言葉責めLVUP":
			if s.composer != nil {
				s.composer.Invalidate()
			}
			return state.SessionContinue, true
		case "言葉責め終了":
			s.endWordAttack()
			return state.SessionContinue, true
		case "言葉整理":
			if s.openWordOrganize() {
				return state.SessionAwaitInput, true
			}
			return state.SessionContinue, true
		case "次の責め言葉":
			// 反应台词标签末尾都会 MOVE f1=14951 再 SELECT_TAIL。玩家是在
			// 上一句反应台词停住时点的推进，所以到这里必须直接结算/跳下一句；
			// 再等一次点击会多出一个没有任何台词的空白停顿。
			if s.wordPlay != nil {
				if step, handled := s.advanceWord(); handled {
					return step, true
				}
			}
			return state.SessionContinue, true
		case "責め言葉の反応処理":
			// 每个词标签 MSG 后 MOVE f1=106 + SELECT_TAIL。到这时 wordPlay
			// 的 current 就是玩家在手机里选的第 index 句。把数值写回、跳
			// 到 response_* 台词；找不到标签就直接下一句。
			if s.wordPlay != nil {
				if step, handled := s.wordReaction(); handled {
					return step, true
				}
			}
			return state.SessionContinue, true
		case "title":
			// 剧本主动回标题（结局/失败后）。原引擎会重建标题菜单，
			// 而不是把 SELECT_TAIL 当空选择继续跑 opening 标签。
			return s.enterTitle(), true
		case "sync":
			// 剧本在 TRANSITION/LAYER_CURVE 之后发 sync，原引擎会等演出结束。
			// 引擎的转场由 Compose 按墙钟推进，所以这里返回 Wait，宿主下一帧
			// 再来；没有转场时直接继续，避免无意义的空转。
			if s.composer != nil && s.composer.TransitionRunning() {
				s.gameState.SetWaitMilliseconds(16)
				return state.SessionWait, true
			}
			return state.SessionContinue, true
		}
	}
	switch command {
	// SELECT / SELECT_TAIL 的参数是"名字表下标"，才能当字符串命令解析。
	// SELECT_ENTRY（0x36）的 4 字节参数是"标签号"而不是名字下标：すくーるヘブン
	// 每个可回放场景结尾都是 `SELECT_ENTRY 2`，而名字表第 2 项正好是 "title"，
	// 按名字解析会把场景收尾当成"回标题"，剧情演到一半就被切回标题画面
	// （表现为"整个游戏流程走不下去"）。它只做鉴赏登记，绝不能进宿主命令表。
	case vm.Select, vm.SelectTail:
		// SELECT 的参数有两种：带 FETCH 选项的选择肢，和
		// `InitBeginTimeParameters()` 这类 Lua 调用。后者没有选项，
		// 若当选择肢处理会被静默吞掉，当天参数就不会快照。
		if slg.IsTranslate(name) {
			return s.runLuaTranslate(host, name), true
		}
		// UK/UKIE（裏教師）的地图不是 SystemCall("map")，而是
		// SELECT_TAIL "map"：phase 处理器 move frame[1]=名字表里的 "map"
		// 后直接发 0x37。原引擎看到名字 "map" 就打开 map.dat 驱动的
		// 引擎地图，玩家选完地点把表序号写进 #result 再继续分支。
		// 这里必须赶在 IsCommand 之前拦截：'map' 不是 SLG 命令，落到
		// 下面的通用路径会被 ApplySelect（按钮表为空 → Continue）跳过，
		// 表现就是地图整屏被跳过、日期一路狂飙。
		if name == "map" && command == vm.SelectTail {
			// 満淫電車2 这一代的 map 段是精灵图：脚本自己铺好底图，
			// 引擎只注册站点热区并画头像/箭头。必须在 bsmap 之前判，
			// 否则目录为空会落到 ApplySelect（空按钮表）把地图跳过。
			if s.beginMD2MapSelect() {
				return state.SessionAwaitSelect, true
			}
			if s.openBsMap() {
				return state.SessionAwaitSelect, true
			}
		}
		// 教育指導（KS）的放学目标卡：剧本只写 #result 位掩码，卡片布局在
		// [target select] 里。不拦截就会被 ApplySelect 当成空选择跳过，
		// #result 保持 0，每天放学都走"谁也不找"的分支。
		if name == "target" && command == vm.SelectTail && s.beginTargetCardSelect() {
			return state.SessionAwaitInput, true
		}
		if !slg.IsCommand(name) {
			return state.SessionContinue, false
		}
		result := slg.Tail(host, name)
		s.machine.SetFrame(0, result)
		if s.machine.Faulted() {
			return state.SessionFaulted, true
		}
		if s.slgState != nil {
			// state training/territory/day_result：Lua 界面接管画面，
			// 脚本必须停在这个 SELECT_TAIL 上。
			return state.SessionAwaitInput, true
		}
		if s.gameState != nil && s.gameState.Selecting() && isCustomMapStatement(name) {
			// 只有 "custom map" 才是"等待宿主点击剧本铺好的按钮"。
			// "restore flags"/"store flags"/"auto save" 这些 SLG 系统命令
			// 也可能发生在按钮表仍在选择态的时候（午后命令画面先 store，
			// 选完命令后 restore），如果这里也返回 AwaitSelect，画面会原地
			// 重开，命令永远执行不到。
			return state.SessionAwaitSelect, true
		}
		if s.titleMode {
			return state.SessionAwaitTitle, true
		}
		if _, ok := s.gameState.Movie(); ok {
			return state.SessionAwaitMovie, true
		}
		return state.SessionContinue, true
	}
	return state.SessionContinue, false
}
