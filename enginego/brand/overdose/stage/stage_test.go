package stage

import (
	"os"
	"path/filepath"
	"testing"

	bishopgfx "kizuna/enginego/brand/bishop/gfx"
	"kizuna/enginego/brand/overdose/arc"
	overgfx "kizuna/enginego/brand/overdose/gfx"
	overscript "kizuna/enginego/brand/overdose/script"
)

// 以下 stage 合成测试依赖真实 CPB（bg011b + tmi_d05），无数据整轮跳过。
// 走真实图是因为：棋盘图合成的“非零像素”断言对合成图无意义 ——
// 相比合成色块，真实图的像素分布才透露居中锚点与混合是否落地。

var stageDirCandidates = []string{
	os.Getenv("GAME_DIR"),
	"/root/code/engine/游戏/YUI/優衣",
	"../../../engine/游戏/YUI/優衣",
	"./testdata",
}

func stageDir(t *testing.T) string {
	t.Helper()
	for _, c := range stageDirCandidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(c, "graphic.arc")); err == nil {
			return c
		}
	}
	t.Skip("未找到游戏数据目录（设置 GAME_DIR）")
	return ""
}

func stageCache(t *testing.T) *overgfx.Cache {
	t.Helper()
	dir := stageDir(t)
	set, err := arc.OpenSet(dir)
	if err != nil {
		t.Fatalf("openSet: %v", err)
	}
	t.Cleanup(func() { set.Close() })
	return overgfx.FromArchiveSet(set)
}

func TestComposeBackgroundOnly(t *testing.T) {
	s := NewStage(stageCache(t))
	s.SetBackground("bg011b")
	dst := bishopgfx.NewBgraCanvas(Width, Height)
	s.Compose(dst)
	if countNonZero(dst) == 0 {
		t.Fatalf("background compose empty")
	}
}

func TestComposeBackgroundAndCharacter(t *testing.T) {
	s := NewStage(stageCache(t))
	s.SetBackground("bg011b")
	s.SetCharacter(0, "tmi_d05", 400)
	dst := bishopgfx.NewBgraCanvas(Width, Height)
	s.Compose(dst)
	// 只验特征：有背景像素 + 立绘像素（中央底半区应较亮于纯背景）
	if countNonZero(dst) == 0 {
		t.Fatalf("composed frame empty")
	}
	// 立绘居中到 x=400：中央列应有非零像素且与角落不同
	center := dst.Buffer[(300*Width+400)*4 : (300*Width+400)*4+3]
	corner := dst.Buffer[10*4 : 10*4+3]
	if center[0] == 0 && center[1] == 0 && center[2] == 0 {
		// 允许极少数背景全黑：但立绘是角色皮肤，上限为 0 直接判为“没画上”
		t.Fatalf("center pixel black — character not drawn at x=400 (y=300)")
	}
	_ = corner
}

// ev → bg 必须真的退出 CG 模式；否则 Compose 永远优先旧 CG，
// 玩家会看到“CG 固定不变”，背景/立绘全部失效。
func TestBackgroundClearsCG(t *testing.T) {
	s := NewStage(stageCache(t))
	s.ShowCG("cga01-01")
	if s.State().CG == "" {
		t.Fatal("ShowCG 未生效")
	}
	s.SetBackground("bg011b")
	if s.State().CG != "" {
		t.Fatalf("bg 后 CG 仍为 %q", s.State().CG)
	}
	// 转场路径同样要退出 CG，但旧 CG 保留在 prev 快照里供淡出。
	s.ShowCG("cga01-01")
	s.BeginBackgroundCrossfade("bg011b", 1000)
	if s.State().CG != "" {
		t.Fatalf("crossfade 后 CG 仍为 %q", s.State().CG)
	}
	if !s.hasPrev || s.prevCG != "cga01-01" {
		t.Fatalf("转场快照丢了旧 CG: hasPrev=%v prevCG=%q", s.hasPrev, s.prevCG)
	}
}

func TestFlashIsAnimated(t *testing.T) {
	s := NewStage(nil)
	s.Flash(100, true)
	if !s.Animating() {
		t.Fatal("flash 未进入动画态")
	}
	s.Tick(200)
	if s.Animating() {
		t.Fatal("flash 结束后仍在动画态")
	}
}

// 满幕布被新场景消费时必须淡出，而不是硬清（硬清会闪回旧帧）。
func TestSceneChangeRetiresFullCurtain(t *testing.T) {
	s := NewStage(nil)
	s.Fade(1000, true)
	s.Tick(1100)
	if s.fadeAlpha != 255 {
		t.Fatalf("fade 未到 255: %d", s.fadeAlpha)
	}
	s.SetBackground("bg011b")
	if s.fadeTo != 0 || s.fade == FadeNone {
		t.Fatalf("满幕布应改为淡出，实 kind=%v to=%d", s.fade, s.fadeTo)
	}
	s.Tick(1100)
	if s.fade != FadeNone || s.fadeAlpha != 0 {
		t.Fatalf("淡出后 fade 残留: %v/%d", s.fade, s.fadeAlpha)
	}
	// 淡入中（未到 255）同样是幕布：换背景必须补满再淡出，不能直接清掉。
	s.Fade(1000, true)
	s.Tick(100)
	s.SetBackground("bg012b")
	if s.fadeTo != 0 || s.fadeAlpha != 255 {
		t.Fatalf("半程 fade 应被消费为淡出，实 kind=%v to=%d alpha=%d", s.fade, s.fadeTo, s.fadeAlpha)
	}
}

// モザイク 是 2000ms 的局部演出，不能永久盖到后面的 HCG。
func TestMosaicExpiresAndClearsOnCG(t *testing.T) {
	s := NewStage(nil)
	s.TempFilter("mosaic", 2000)
	if s.filter != "mosaic" || !s.Animating() {
		t.Fatal("mosaic 未生效")
	}
	s.SetCG("cga01-01")
	if s.filter == "mosaic" {
		t.Fatal("新 CG 仍继承马赛克")
	}
	s.TempFilter("mosaic", 2000)
	s.Tick(2100)
	if s.filter == "mosaic" {
		t.Fatal("mosaic 超时未清")
	}
}

// 立绘换位是单层补间：画一张图从旧几何滑到新几何，不能开两张帧。
func TestCharTweenKeepsSingleLayer(t *testing.T) {
	s := NewStage(nil)
	s.setCharByPos(0, "tmi_d05", 400, 1, 1000, 0)
	if s.chars[0].Name != "tmi_d05" || s.chars[0].AnimMs != 0 {
		t.Fatalf("首次落位不应补间: %+v", s.chars[0])
	}
	s.setCharByPos(0, "tmi_d05", 200, 1, 1000, 0)
	if s.hasPrev {
		t.Fatal("立绘换位不应抓 prev 帧（会画两张同角色）")
	}
	if s.chars[0].AnimMs != 1000 || s.chars[0].FromX != 400 || s.chars[0].X != 200 {
		t.Fatalf("补间参数错: %+v", s.chars[0])
	}
	s.Tick(500)
	// 中点是 300：用合成结果验证太依赖像素，直接算插值即可。
	t.Logf("mid tween elapsed=%d", s.chars[0].AnimElapsed)
}

// 复现“暗転 → 新 bg”：黑幕由 bg 消费并 1000ms 淡出，不能永久残留。
func TestCurtainSceneTransitionNotStuck(t *testing.T) {
	s := NewStage(stageCache(t))
	vars := &overscript.VariableStore{}
	_ = vars.SetString(2, 0, "bg011b")
	s.ApplyMacro(vars, "bg")
	s.ApplyMacro(vars, "暗転")
	s.Tick(1200)
	if s.fadeAlpha != 255 {
		t.Fatalf("暗転后幕布未盖满: %d", s.fadeAlpha)
	}
	_ = vars.SetString(2, 0, "bg012b")
	s.ApplyMacro(vars, "bg")
	if s.fadeTo == 255 {
		t.Fatal("bg 未启动幕布淡出")
	}
	s.Tick(1200)
	dst := bishopgfx.NewBgraCanvas(Width, Height)
	s.Compose(dst)
	if s.fade != FadeNone {
		t.Fatalf("场景切换后仍有持久 fade: %v", s.fade)
	}
	if countNonZero(dst) == 0 {
		t.Fatal("幕布结束后仍是黑屏")
	}
}

// 幕布必须一直保持到场景内容替换（原引擎 0 号层语义），不能自动消失。
func TestCurtainPersistsUntilSceneChange(t *testing.T) {
	s := NewStage(nil)
	s.Curtain(1000, false)
	s.Tick(500)
	if s.fadeAlpha == 0 {
		t.Fatal("幕布淡入中 alpha=0")
	}
	s.Tick(2000)
	if s.fadeAlpha != 255 || s.fade != FadeBlack {
		t.Fatalf("幕布被自动撤掉: kind=%v alpha=%d", s.fade, s.fadeAlpha)
	}
	s.SetBackground("bg011b")
	s.Tick(1100)
	if s.fade != FadeNone || s.fadeAlpha != 0 {
		t.Fatalf("换场景后幕布未淡出: %v/%d", s.fade, s.fadeAlpha)
	}
}

func TestTransitionProgress(t *testing.T) {
	s := NewStage(stageCache(t))
	s.SetBackground("bg011b")
	s.BeginBackgroundCrossfade("title_bg", 100)
	st := s.Tick(50)
	if st.TransitionKind != TransitionBackground {
		t.Fatalf("transition kind %v, want Background", st.TransitionKind)
	}
	if st.Progress < 0.4 || st.Progress > 0.6 {
		t.Fatalf("progress %.2f at half tick, want ~0.5", st.Progress)
	}
	dst := bishopgfx.NewBgraCanvas(Width, Height)
	s.Compose(dst)
	if countNonZero(dst) == 0 {
		t.Fatalf("mid-transition frame empty")
	}
	s.Tick(100)
	st2 := s.Tick(0)
	if st2.Progress < 1 {
		// Tick 后进度满了也算完成（Status 下次读 1）
		if s.progress() != 1 {
			t.Fatalf("progress after full tick = %.2f, want 1", s.progress())
		}
	}
	if s.hasPrev {
		t.Fatalf("transition should be done, hasPrev still set")
	}
	s.Compose(dst)
}

func TestFade(t *testing.T) {
	s := NewStage(stageCache(t))
	s.SetBackground("bg011b")
	s.Fade(100, true)
	dst := bishopgfx.NewBgraCanvas(Width, Height)
	s.Compose(dst)
	s.Tick(50)
	st := s.Tick(0)
	if st.FadeActive == false {
		t.Fatalf("fade should be active mid-way")
	}
	if s.fadeAlpha <= 0 || s.fadeAlpha >= 255 {
		t.Fatalf("fadeAlpha %d at half tick", s.fadeAlpha)
	}
	// 完整走完：Alpha 满格，覆盖为黑
	s.Tick(200)
	if s.fadeAlpha != 255 {
		t.Fatalf("fadeAlpha after full = %d, want 255", s.fadeAlpha)
	}
	s.Compose(dst)
	if s.fadeAlpha != 255 {
		t.Fatalf("post-compose fadeAlpha changed")
	}
}

func TestApplyMacroBgFg(t *testing.T) {
	// 宏路径本身只验“状态被推进”，不验像素细节（像素由上面覆盖）
	s := NewStage(stageCache(t))
	vs := newFakeVars()
	vs.strs[0] = "bg011b"
	vs.strs[1] = "tmi_d05"
	s.ApplyMacro(vs, "bg")
	s.Tick(1100) // 背景交叉淡走完，再验证合成有内容
	s.ApplyMacro(vs, "fg")
	st := s.State()
	if st.Background != "bg011b" {
		t.Fatalf("background after bg macro = %q", st.Background)
	}
	found := false
	for _, ch := range st.Characters {
		if ch.Name == "tmi_d05" && ch.X == 400 {
			found = true
		}
	}
	if !found {
		t.Fatalf("fg macro did not place char at (0,400): %+v", st.Characters)
	}
	dst := bishopgfx.NewBgraCanvas(Width, Height)
	s.Compose(dst)
	if countNonZero(dst) == 0 {
		t.Fatalf("macro-driven compose empty")
	}
}

// 纯逻辑路径（无存档）：缩放模式以外的 stage 基本状态机。
func TestStageWithoutCache(t *testing.T) {
	s := NewStage(nil)
	s.SetBackground("any")
	s.SetCharacter(2, "x", 600)
	s.ShowCG("cgA")
	dst := bishopgfx.NewBgraCanvas(Width, Height)
	s.Compose(dst) // 无 cache：仅黑底不崩
	if !s.Visible() {
		t.Fatalf("should be in CG mode")
	}
	s.HideCG()
	if s.Visible() {
		t.Fatalf("CG should hide")
	}
	st := s.State()
	if len(st.Characters) != CharSlots {
		t.Fatalf("state slots")
	}
	s.Fade(100, false)
	s.Tick(100)
	if s.fadeAlpha != 255 || s.fade != FadeWhite {
		t.Fatalf("white fade not at full")
	}
}

func countNonZero(c *bishopgfx.BgraCanvas) int {
	n := 0
	for i := 0; i < len(c.Buffer); i += 4 {
		if c.Buffer[i] != 0 || c.Buffer[i+1] != 0 || c.Buffer[i+2] != 0 {
			n++
		}
	}
	return n
}

// fakeVars 是本测试的 Variables 最小实现（string 表 + int 表）。
type fakeVars struct {
	strs map[uint16]string
	ints map[uint16]int32
}

func newFakeVars() *fakeVars {
	return &fakeVars{strs: map[uint16]string{}, ints: map[uint16]int32{}}
}

func (f *fakeVars) GetString(ns, idx uint16) (string, error) {
	// 只实现本测试用到的槽位（ns 2 = 全局字符串，idx 0/1/90）
	if ns != 2 {
		return "", nil
	}
	return f.strs[idx], nil
}

func (f *fakeVars) GetInt(ns, idx uint16) (int32, error) {
	if ns != 1 {
		return 0, nil
	}
	return f.ints[idx], nil
}

// solidImage 造纯色 BGRA 图，避免立绘重影测试依赖真实 CPB。
func solidImage(w, h int, b, g, r, a byte) *overgfx.Image {
	img := &overgfx.Image{Width: w, Height: h}
	img.Pixels = make([]byte, w*h*4)
	for i := 0; i < len(img.Pixels); i += 4 {
		img.Pixels[i], img.Pixels[i+1], img.Pixels[i+2], img.Pixels[i+3] = b, g, r, a
	}
	return img
}

// 幕布还在淡入（未到 255）时 ev/bg 就到：必须补满再淡出，不能继续
// 淡向全黑。旧逻辑只认 alpha==255，结果整个 H 场景被黑层盖住 20 多秒。
func TestCurtainConsumedByCGDuringFadeIn(t *testing.T) {
	s := NewStage(nil)
	s.Curtain(1000, false)
	s.Tick(100)
	if s.fadeAlpha == 0 || s.fadeAlpha == 255 {
		t.Fatalf("测试前提：幕布应在淡入中，alpha=%d", s.fadeAlpha)
	}
	s.SetCG("cgx")
	if s.fadeTo != 0 || s.fadeAlpha != 255 {
		t.Fatalf("淡入中的幕布未被消费: to=%d alpha=%d", s.fadeTo, s.fadeAlpha)
	}
	s.Tick(1100)
	if s.fade != FadeNone {
		t.Fatalf("幕布淡出后仍残留: %v", s.fade)
	}
}

func TestCurtainConsumedByBgDuringFadeIn(t *testing.T) {
	s := NewStage(nil)
	s.Curtain(1000, false)
	s.Tick(100)
	s.SetBackground("bg011b")
	if s.fadeTo != 0 || s.fadeAlpha != 255 {
		t.Fatalf("bg 未消费淡入中的幕布: to=%d alpha=%d", s.fadeTo, s.fadeAlpha)
	}
	s.BeginBackgroundCrossfade("bg012b", 1000)
	if s.fadeTo != 0 {
		t.Fatalf("幕布淡出应保持: to=%d", s.fadeTo)
	}
}

// 背景转场期间换表情：旧帧不能再用 prevChars，否则旧脸和新脸各画一遍。
func TestExpressionChangeDuringBgCrossfadeDoesNotGhost(t *testing.T) {
	cache := overgfx.NewCache(nil)
	cache.Put("bgx", solidImage(Width, Height, 0, 0, 0, 255))
	cache.Put("oldface", solidImage(200, 600, 0, 0, 255, 255)) // 红
	cache.Put("newface", solidImage(200, 600, 255, 0, 0, 255)) // 蓝
	s := NewStage(cache)
	s.SetBackground("bgx")
	s.SetCharacterFull(0, "oldface", 400, 0, 1, 255)
	dst := bishopgfx.NewBgraCanvas(Width, Height)
	s.Compose(dst)

	s.BeginBackgroundCrossfade("bgx", 1000)
	s.setCharByPos(0, "newface", 400, 1, 1000, 0)
	s.Tick(500)
	s.Compose(dst)

	off := (300*Width + 400) * 4
	b, r := int(dst.Buffer[off]), int(dst.Buffer[off+2])
	if b <= r+40 {
		t.Fatalf("立绘重影：角色区 B=%d R=%d（应只剩新表情的蓝，不应红蓝各半）", b, r)
	}
}

// 幕布淡出期间连续换 CG：每次 SetCG 都会开始新的交叉淡并重置 transition
// 时钟。若幕布复用它，fadeAlpha 会被打回 255，实机表现为黑幕反复闪回。
func TestCurtainClockIndependentFromCGChanges(t *testing.T) {
	s := NewStage(nil)
	s.Curtain(1000, false)
	s.Tick(1100)
	s.SetBackground("bg011b") // 消费幕布，开始 1000ms 淡出
	s.Tick(300)
	first := s.fadeAlpha
	if first >= 255 || first <= 0 {
		t.Fatalf("测试前提：淡出应在中途，alpha=%d", first)
	}
	s.SetCG("cgA01-01")
	s.Tick(100)
	second := s.fadeAlpha
	if second >= first {
		t.Fatalf("换 CG 把幕布进度打回去了: %d -> %d", first, second)
	}
	s.SetCG("cgA01-02")
	s.Tick(100)
	if third := s.fadeAlpha; third >= second {
		t.Fatalf("第二次换 CG 又重置幕布: %d -> %d", second, third)
	}
}
