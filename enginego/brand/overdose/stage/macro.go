package stage

import (
	"math/rand"
	"sync"

	overgfx "kizuna/enginego/brand/overdose/gfx"
	"kizuna/enginego/brand/overdose/script"
	"kizuna/enginego/core"
)

// 宏映射（overdose_spec §6）
// macro.asb 里每个宏（中英/日文名）先走 0x0a "macro" 进 VM，
// VM 的 MacroHandler 把 (宏名, 参数) 递给 session，session 再落到本包。
// 这里 ApplyMacro 是纯映射，**不读脚本、不跑定时器**：
// 定时器/等待全部在 Tick 里推进，session 侧按 Status.FadeDone 自行补续。
//
// 全局变量约定（§5.2）：
//   global string var 0 = 背景/CG 图名（bg/ev 宏读它）
//   global string var 1 = 立绘图名（fg/face 系读它）
//   global int var 90   = fg_filter 风格（0=中心,1=左,2=右,3=远左,4=远右）
//   global int var 90 的字符串形态存 filter 名（night/evening/sepia…）
// 这一映射是 0x33 的直接展开：op 0x33 (idx, image-ref, x, y, …, alpha, …, time)
// 实测宏全部用 x∈{400,200,600,160,640}、y=Ai=0、alpha=100%、time=1000/1200/900。

// Vars 是 stage 读全局变量的最小视口。直接给 script.VariableStore 或包装均可。
type Vars interface {
	GetString(ns, idx uint16) (string, error)
	GetInt(ns, idx uint16) (int32, error)
}

const (
	varBgYuki     = 0  // global str(2,0)
	varCharYuki   = 1  // global str(2,1)
	varFilterSty  = 90 // global int(1,90) → fg 槽位也做此约定
	varFilterName = 90 // global str(2,90) → filter 名（night/evening/sepia…）
)

// ApplyMacro 按宏名把变量值落进舞台。参数 args 是 VM MacroHandler 给的求值结果（供大小写以外的扩展）。
func (s *Stage) ApplyMacro(vars Vars, name string) {
	if vars == nil {
		// 空变量表也得能响应（测试/无脚本的标题画面）：只剩不读变量的宏对手。
		switch name {
		case "fg_delall":
			for i := 0; i < CharSlots; i++ {
				s.ClearCharacter(i)
			}
		}
		return
	}
	bgName, _ := vars.GetString(2, varBgYuki)
	chName, _ := vars.GetString(2, varCharYuki)

	switch name {
	case "bg":
		// 交叉淡（trans_bg 紧跟，一般 1000ms）。BeginBackgroundCrossfade
		// 会先抓旧帧（含旧 CG）再切背景并退出 CG 模式，所以不要先 SetBackground，
		// 否则旧 CG 快照会被提前清掉，转场只剩背景硬切。
		core.Logf("overdose-stage: bg %q", bgName)
		s.BeginBackgroundCrossfade(bgName, 1000)
	case "ev":
		// 全屏 CG 覆盖一切（把字条也藏进 CG 下，脚本里 msgoff 先行）
		core.Logf("overdose-stage: cg %q", bgName)
		s.ShowCG(bgName)
	case "evd":
		core.Logf("overdose-stage: cg %q (evd)", bgName)
		s.ShowCG(bgName)
		s.ClearCGVars(bgName)
	case "trans_bg":
		s.CommitTransition()
	case "trans_fg", "trans_face":
		s.CommitTransition()
	case "fg":
		s.setCharByPos(0, chName, 400, 1, 1000, 0)
	case "fgl":
		// macro.asb: fgl 用 0x33 idx=3 / x=160（远左档）；idx=1/x=200 是 fgl_set。
		s.setCharByPos(3, chName, 160, 1, 1000, 3)
	case "fgr":
		// fgr 用 idx=4 / x=640（远右档）；idx=2/x=600 是 fgr_set。
		s.setCharByPos(4, chName, 640, 1, 1000, 4)
	case "face":
		s.setCharByPos(0, chName, 400, 1, 1000, 0)
	case "facel":
		s.setCharByPos(1, chName, 200, 1, 1000, 1)
	case "facer":
		s.setCharByPos(2, chName, 600, 1, 1000, 2)
	case "facell":
		s.setCharByPos(3, chName, 160, 1, 1000, 3)
	case "facerr":
		s.setCharByPos(4, chName, 640, 1, 1000, 4)
	case "fg_set":
		s.setCharByPos(0, chName, 400, 1, 1000, 0)
	case "fgl_set":
		s.setCharByPos(1, chName, 200, 1, 1000, 1)
	case "fgr_set":
		s.setCharByPos(2, chName, 600, 1, 1000, 2)
	case "fgll_set":
		s.setCharByPos(3, chName, 160, 1, 1000, 3)
	case "fgrr_set":
		s.setCharByPos(4, chName, 640, 1, 1000, 4)
	case "fg_sizeL":
		s.setCharByPos(0, chName, 400, 1.2, 1000, 0)
	case "fgl_sizeL":
		s.setCharByPos(1, chName, 200, 1.2, 1000, 1)
	case "fgr_sizeL":
		s.setCharByPos(2, chName, 600, 1.2, 1000, 2)
	case "fg_sizeL_set":
		s.setCharByPos(0, chName, 400, 1.2, 1000, 0)
	case "fgl_sizeL_set":
		s.setCharByPos(1, chName, 200, 1.2, 1000, 1)
	case "fgr_sizeL_set":
		s.setCharByPos(2, chName, 600, 1.2, 1000, 2)
	case "fg_sizeS":
		s.setCharByPos(0, chName, 400, 0.9, 1000, 0)
	case "fgl_sizeS":
		s.setCharByPos(1, chName, 200, 0.9, 1000, 1)
	case "fgr_sizeS":
		s.setCharByPos(2, chName, 600, 0.9, 1000, 2)
	case "fg_sizeS_set":
		s.setCharByPos(0, chName, 400, 0.9, 1000, 0)
	case "fgl_sizeS_set":
		s.setCharByPos(1, chName, 200, 0.9, 1000, 1)
	case "fgr_sizeS_set":
		s.setCharByPos(2, chName, 600, 0.9, 1000, 2)
	case "fg_del":
		s.ClearCharacter(0)
		s.beginCharFade(250)
	case "fgl_del":
		s.ClearCharacter(1)
		s.beginCharFade(250)
	case "fgr_del":
		s.ClearCharacter(2)
		s.beginCharFade(250)
	case "fgll_del":
		s.ClearCharacter(3)
		s.beginCharFade(250)
	case "fgrr_del":
		s.ClearCharacter(4)
		s.beginCharFade(250)
	case "fg_delall":
		for i := 0; i < CharSlots; i++ {
			s.ClearCharacter(i)
		}
		s.beginCharFade(250)
	case "fg_filter", "fg_filter_":
		if mode, err := vars.GetString(2, varFilterName); err == nil && mode != "" {
			s.SetFilter(mode)
		} else if idx, err := vars.GetInt(1, varFilterSty); err == nil {
			// int var 90 的 0..4 同槽位——某些脚本只写 int 不写 str 名
			// 退化为按位置模式无滤镜，但要留一下口：原引擎在此处按 int var 90 的值走滤镜分支
			s.SetFilter(filterNameForIndex(int(idx)))
		}
	case "暗転":
		// macro.asb 暗転 = 0x28(-1)+0x34(-1) 清两层数组 → 0x27 铺黑层 1000ms
		// → 等 1000ms。黑层要一直留到下一句 bg/ev 用 0 号层替换。
		core.Logf("overdose-stage: curtain black")
		s.ClearSceneContent()
		s.Curtain(1000, false)
	case "白転":
		core.Logf("overdose-stage: curtain white")
		s.ClearSceneContent()
		s.Curtain(1000, true)
	case "initscene":
		// 落到默认：背景与 CG 换回 (2,0) 指向的图，立绘保留
		s.SetBackground(bgName)
	case "map_close", "場面転換閉", "時間経過閉":
		// map_close = msgoff + 0x28(-1)+0x34(-1) → 黑层 1000ms → 等 1000ms。
		core.Logf("overdose-stage: map close curtain")
		s.ClearSceneContent()
		s.Curtain(1000, false)
	case "map_open", "場面転換", "時間経過":
		// map_open = 0x27(0, bg) 1000ms → 等 1000ms；黑幕由 BeginBackgroundCrossfade
		// 消费并淡出，不能先 SetBackground 再 FadeOut（会闪回旧帧）。
		s.BeginBackgroundCrossfade(bgName, 1000)
	case "scenestart", "sceneend":
		// 原版本质是 msgoff + 暗転；暗転 用定时幕布，避免持久黑层。
		if name == "scenestart" {
			core.Logf("overdose-stage: scenestart curtain")
			s.ClearSceneContent()
			s.Curtain(1000, false)
		}
	case "msgon", "msgoff":
		// 窗口显隐由 session 的 EventMacro 处理，舞台不需要动作。
	case "rand_int":
		// 真正的随机化在 HandleMacroEvent 里做（需要写变量，Vars 只有 getter）；
		// 走到这里的是测试/无 VM 的路径，保持无副作用。
	case "フラッシュ短":
		s.Flash(150, true)
	case "フラッシュ", "フラッシュ２", "フラッシュ表示":
		// macro: 白层 50ms → 清层 → 等 150ms（总计 200ms）。
		s.Flash(200, true)
	case "絶頂":
		// 原宏连续三次白闪后再等 2000ms；这里用一次较强白屏近似，
		// 不阻塞脚本（macro body 的等待在本实现里不执行）。
		s.Flash(600, true)
	case "絶頂小":
		s.Flash(350, true)
	case "モザイク":
		// macro.asb 0x1ee8：msgoff → 0x34(-1) 清全部立绘层 → 0x27 把 (2,0)
		// 重新铺回 0 号层 → 条件 0x2c(0,2)/(0,0) → 等 2000ms → msgon。
		// 真正的"马赛克"只出现在 bool(0,99)==false 且 str(2,90)=="sepia"
		// 的回忆滤镜分支；无条件全屏马赛克会把普通 HCG 全打成格子。
		s.ClearCharacters()
		sepia := false
		if v, err := vars.GetString(2, varFilterName); err == nil && v == "sepia" {
			sepia = true
		}
		if sepia {
			core.Logf("overdose-stage: mosaic (sepia comp) 2000ms")
			s.TempFilter("mosaic", 2000)
		} else {
			core.Logf("overdose-stage: comp 2000ms")
		}
	case "delsepia":
		s.SetFilter("")
	case "回想in":
		// 回想入：sepia + 白闪；回想out 关闭 sepia。
		s.SetFilter("sepia")
		s.Flash(400, true)
	case "回想out":
		s.SetFilter("")
	case "終端", "終端短":
		// 終端 = 等音频 → 0x28(-1)+0x34(-1) 清层 → 黑层 1000ms → 再等 2500ms。
		s.ClearSceneContent()
		s.Fade(1000, true)
	case "レイヤー":
		alpha := int32(100)
		if v, err := vars.GetInt(1, 80); err == nil {
			alpha = v
		}
		s.SetOverlayLayer(8, bgName, 400, 0, int(alpha)*255/100, 1, true)
	case "レイヤー消去":
		s.ClearOverlayLayer(8)
	default:
		// 未知宏（フラッシュ/レンタ/回想 等）不是本包职责：session 负责打 log
		// 这里只保留舞台不被误触发：不画、不改、不清。
		logUnknownMacro(name)
	}
}

var unknownMacros sync.Map

func logUnknownMacro(name string) {
	if _, seen := unknownMacros.LoadOrStore(name, struct{}{}); seen {
		return
	}
	core.Logf("overdose-stage: 未处理宏 %q（已记录一次）", name)
}

// setCharByPos 是 fg 系宏的公共落点：先感性地记好前帧（供转场混），再把当前层写好。
// durationMs 与原 0x33 的 INT(7)（第一版 1000、sizeL 1200、sizeS 900）一一对应；
// 0 时不转场（_set 后缀形）。
func (s *Stage) setCharByPos(slot int, name string, x int, scale float64, durationMs, filterIdx int) {
	ch := Slot{Name: name, X: x, Y: charAnchorY, Scale: scale, Alpha: 255}
	// 原 0x33 只改同一条层：图名换成新值、几何动画到新值。绝不能开
	// prev/cur 两帧交叉淡——那会同时画旧位置和新位置两张同角色图。
	if durationMs > 0 && s.chars[slot].Name != "" {
		old := s.chars[slot]
		ch.FromX, ch.FromY = old.X, old.Y
		ch.FromScale, ch.FromAlpha = old.Scale, old.Alpha
		ch.AnimMs = durationMs
		ch.AnimElapsed = 0
	} else {
		ch.FromX, ch.FromY, ch.FromScale, ch.FromAlpha = ch.X, ch.Y, ch.Scale, ch.Alpha
	}
	s.chars[slot] = ch
	s.dirty = true
	_ = filterIdx // filter 在 fg_filter 宏里统一处理，槽位不同不意味着滤镜不同
}

// beginCharFade 原宏 fg_del 是 0x34 直接清层，没有淡出；这里保持"立即消失"。
// 旧实现用 prev/cur 交叉淡，会把被删角色再画 250ms，看起来像多了一个残影。
func (s *Stage) beginCharFade(durationMs int) {
	_ = durationMs
	s.dirty = true
}

// ClearCGVars 表示 evd 的副作用：清 (2,0)（原宏 0x12 清 (2,0)）。
func (s *Stage) ClearCGVars(bgName string) {
	// Stage 不持有变量表，不动这里：留接口给 session 按宏语义去清变量。
	_ = bgName
}

// filterNameForIndex 把 int var 90 的槽位码映射回来（仅为恢复/调试，不改变合成）。
func filterNameForIndex(idx int) string {
	switch idx {
	case 0:
		return "center"
	case 1:
		return "left"
	case 2:
		return "right"
	case 3:
		return "farleft"
	case 4:
		return "farright"
	case 5:
		return "sepia"
	default:
		return ""
	}
}

// HandleMacroEvent 是 script.MacroHandler 的适配：供 VM.SetMacroHook 直接传。
// VM 只给宏名与已求值参数（脚本侧参数此处未用），本适配从 vm 的变量表补图名。
func HandleMacroEvent(s *Stage, vs *script.VariableStore, macroName string, args []script.Value) error {
	// macro.asb 的 rand_int 体是 `0x0e var(1,92) var(1,92) label(113) t18`：
	// 节点 3(label id) 在求值器里是"以内联 opcode 调用"，113=0x71 的
	// handler 0x4343ea 计算 (int)(rand01 * N)，N 取变量当前值。
	// 调用方固定先写 int var 92=N 再调本宏，结果也落回 var 92（0..N-1）。
	// 不实现会让所有随机分支恒取预设值，剧情旗标无法推进（序盤A 死循环）。
	if macroName == "rand_int" {
		randIntVar(vs, 92)
		return nil
	}
	s.ApplyMacro(varsAdapter{vs}, macroName)
	return nil
}

// randIntVar 复刻 opcode 0x71：result = (int)(rand01 * N)，N 为变量当前值。
// 原 handler 先 timeGetTime+seed，再 cvttsd2si（向零截断），所以值是 0..N-1；
// N<=0 时保持 0（原引擎 fild 0 → 结果 0）。
func randIntVar(vs *script.VariableStore, idx uint16) {
	if vs == nil {
		return
	}
	n, err := vs.GetInt(1, idx)
	if err != nil || n <= 0 {
		return
	}
	_ = vs.SetInt(1, idx, int32(rand.Intn(int(n))))
}

type varsAdapter struct{ vs *script.VariableStore }

func (a varsAdapter) GetString(ns, idx uint16) (string, error) { return a.vs.GetString(ns, idx) }
func (a varsAdapter) GetInt(ns, idx uint16) (int32, error)     { return a.vs.GetInt(ns, idx) }

// SlotLetter 与 SortCharsForSave 辅助 session 持久化（状态已里带名，直接返回供直存）。
func SlotLetter(slot int) string {
	if slot < 0 || slot >= CharSlots {
		return ""
	}
	return string(rune('0' + slot))
}

// Ensure imports 对齐（scale.go 在 gfx 包，这里 stage 只直接用过 Cache 与 Image 类型）。
// 这行写在下面是为了让编译器不弄错包名（overgfx 是另一个包别解构）。
var _ = overgfx.ScaleBilinear
